package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8/internal/client"
	"k8/internal/controller"
	"k8/internal/pki"
	"k8/internal/scheduler"
	"k8/internal/store"
)

func main() {
	listenAddr := flag.String("listen", ":6443", "controller listen address (TLS)")
	tokenFlag := flag.String("token", os.Getenv("K8_TOKEN"), "join secret agents must present (random if empty; also $K8_TOKEN)")
	tlsSANs := flag.String("tls-san", "", "extra comma-separated DNS names or IPs for the serving certificate")
	reconcileInterval := flag.Duration("reconcile-interval", 2*time.Second, "how often to reconcile desired state")
	nodeTimeout := flag.Duration("node-timeout", 10*time.Second, "when a node is considered offline")
	dataDir := flag.String("data-dir", "/var/lib/k8", "where the state database lives (local disk, not NFS)")
	backupDir := flag.String("backup-dir", "", "directory for periodic state snapshots, e.g. a NAS mount (disabled if empty)")
	backupInterval := flag.Duration("backup-interval", 5*time.Minute, "how often to snapshot state to -backup-dir")
	backupKeep := flag.Int("backup-keep", 24, "how many snapshots to keep in -backup-dir")
	flag.Parse()

	dbPath := filepath.Join(*dataDir, "state.db")
	if *backupDir != "" {
		restored, err := store.RestoreIfMissing(dbPath, *backupDir)
		if err != nil {
			log.Fatalf("restore from backup: %v", err)
		}
		if restored != "" {
			log.Printf("no local state found; restored %s from %s", dbPath, restored)
		}
	}

	state, err := store.OpenSQLite(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer state.Close()

	tlsDir := filepath.Join(*dataDir, "tls")
	ca, err := pki.LoadOrCreateCA(tlsDir)
	if err != nil {
		log.Fatal(err)
	}
	token, err := pki.LoadOrCreateToken(filepath.Join(*dataDir, "token"), ca, joinSecret(*tokenFlag))
	if err != nil {
		log.Fatal(err)
	}
	dnsNames, ips := servingNames(*tlsSANs)
	serverCert, err := ca.ServerCert(dnsNames, ips)
	if err != nil {
		log.Fatal(err)
	}
	adminConfigPath := filepath.Join(*dataDir, "admin.conf")
	if err := writeAdminConfig(adminConfigPath, ca, *listenAddr); err != nil {
		log.Fatal(err)
	}

	sched := scheduler.New(*nodeTimeout)
	ctrl := controller.New(state, sched, *nodeTimeout)
	server := controller.NewHTTPServer(ctrl, controller.Auth{CA: ca, JoinSecret: token.Secret})

	go func() {
		ticker := time.NewTicker(*reconcileInterval)
		defer ticker.Stop()

		ctrl.Reconcile(time.Now().UTC())
		for tick := range ticker.C {
			ctrl.Reconcile(tick.UTC())
		}
	}()

	if *backupDir != "" {
		go func() {
			ticker := time.NewTicker(*backupInterval)
			defer ticker.Stop()
			for tick := range ticker.C {
				path, err := state.Backup(*backupDir, *backupKeep, tick)
				if err != nil {
					// A missed backup is not worth taking the control plane
					// down for; the next tick tries again.
					log.Printf("backup failed: %v", err)
					continue
				}
				log.Printf("state backed up to %s", path)
			}
		}()
	}

	httpServer := &http.Server{
		Addr:              *listenAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{serverCert},
			ClientCAs:    ca.Pool(),
			// Optional at the handshake so /cacerts and /nodes/join work
			// before a node has a certificate; every other route requires one.
			ClientAuth: tls.VerifyClientCertIfGiven,
			MinVersion: tls.VersionTLS13,
		},
	}

	log.Printf("controller listening on %s, state in %s", *listenAddr, dbPath)
	log.Printf("join token in %s, admin config in %s", filepath.Join(*dataDir, "token"), adminConfigPath)
	if err := httpServer.ListenAndServeTLS("", ""); err != nil {
		log.Fatal(err)
	}
}

// joinSecret accepts either a bare secret or a full K8... token, like
// K3S_TOKEN does.
func joinSecret(flagValue string) string {
	if token, err := pki.ParseToken(flagValue); err == nil {
		return token.Secret
	}
	return strings.TrimSpace(flagValue)
}

// servingNames lists every name agents might use to reach this controller:
// its hostname, localhost, each interface address, and any -tls-san extras.
func servingNames(extra string) ([]string, []net.IP) {
	dnsNames := []string{"localhost"}
	if host, err := os.Hostname(); err == nil && host != "" {
		dnsNames = append(dnsNames, host)
	}
	ips := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				ips = append(ips, ipNet.IP)
			}
		}
	}
	for _, san := range strings.Split(extra, ",") {
		san = strings.TrimSpace(san)
		switch {
		case san == "":
		case net.ParseIP(san) != nil:
			ips = append(ips, net.ParseIP(san))
		default:
			dnsNames = append(dnsNames, san)
		}
	}
	return dnsNames, ips
}

// writeAdminConfig issues a fresh admin client certificate and writes it with
// the CA and a loopback server URL, readable by root only (k3s.yaml's role).
func writeAdminConfig(path string, ca *pki.CA, listenAddr string) error {
	key, err := pki.NewKey()
	if err != nil {
		return err
	}
	certPEM, err := ca.ClientCert(pki.AdminCN, key.Public())
	if err != nil {
		return err
	}
	keyPEM, err := pki.EncodeKey(key)
	if err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return fmt.Errorf("parse -listen: %w", err)
	}
	data, err := json.MarshalIndent(client.AdminConfig{
		Server: "https://127.0.0.1:" + port,
		CA:     string(ca.PEM),
		Cert:   string(certPEM),
		Key:    string(keyPEM),
	}, "", "  ")
	if err != nil {
		return err
	}
	return pki.WriteFile(path, data, 0o600)
}
