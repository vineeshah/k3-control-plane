package main

import (
	"flag"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"k8/internal/controller"
	"k8/internal/scheduler"
	"k8/internal/store"
)

func main() {
	listenAddr := flag.String("listen", ":8080", "controller listen address")
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

	sched := scheduler.New(*nodeTimeout)
	ctrl := controller.New(state, sched, *nodeTimeout)
	server := controller.NewHTTPServer(ctrl)

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

	log.Printf("controller listening on %s, state in %s", *listenAddr, dbPath)
	if err := http.ListenAndServe(*listenAddr, server.Handler()); err != nil {
		log.Fatal(err)
	}
}
