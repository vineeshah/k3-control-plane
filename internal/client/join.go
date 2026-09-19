package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8/internal/pki"
)

// renewBefore is how close to expiry a node certificate may get before the
// agent rejoins for a new one at startup.
const renewBefore = 30 * 24 * time.Hour

// BootstrapNode returns TLS credentials for nodeID, joining the cluster with
// token if the node has none yet (or its certificate is about to expire).
// Credentials live in dataDir: ca.crt, node.key, node.crt.
//
// Joining is k3s's flow: download the CA, accept it only if its hash matches
// the one pinned in the token, then send a CSR, authenticated by the token's
// secret, over TLS verified against that CA.
func BootstrapNode(server, rawToken, nodeID, dataDir string) (*tls.Config, error) {
	caPath := filepath.Join(dataDir, "ca.crt")
	keyPath := filepath.Join(dataDir, "node.key")
	certPath := filepath.Join(dataDir, "node.crt")

	if tlsConfig, err := loadNodeCredentials(caPath, certPath, keyPath, nodeID); err == nil {
		return tlsConfig, nil
	}
	if rawToken == "" {
		return nil, errors.New("node has no valid credentials and no join token was given")
	}
	token, err := pki.ParseToken(rawToken)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}

	caPEM, err := fetchPinnedCA(server, token.CAHash)
	if err != nil {
		return nil, err
	}

	keyPEM, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		key, err := pki.NewKey()
		if err != nil {
			return nil, err
		}
		if keyPEM, err = pki.EncodeKey(key); err != nil {
			return nil, err
		}
		if err := pki.WriteFile(keyPath, keyPEM, 0o600); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	key, err := pki.ParseKey(keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", keyPath, err)
	}

	csr, err := pki.NewCSR(key, pki.NodeCN(nodeID))
	if err != nil {
		return nil, err
	}
	certPEM, err := requestNodeCert(server, caPEM, token.Secret, nodeID, csr)
	if err != nil {
		return nil, err
	}

	if err := pki.WriteFile(caPath, caPEM, 0o644); err != nil {
		return nil, err
	}
	if err := pki.WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, err
	}
	return clientTLS(caPEM, certPEM, keyPEM)
}

func loadNodeCredentials(caPath, certPath, keyPath, nodeID string) (*tls.Config, error) {
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, err
	}
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	cert, err := pki.ParseCert(certPEM)
	if err != nil {
		return nil, err
	}
	if cert.Subject.CommonName != pki.NodeCN(nodeID) {
		return nil, fmt.Errorf("stored certificate is for %q, not node %q", cert.Subject.CommonName, nodeID)
	}
	if time.Until(cert.NotAfter) < renewBefore {
		return nil, errors.New("node certificate expires soon")
	}
	return clientTLS(caPEM, certPEM, keyPEM)
}

// fetchPinnedCA downloads the CA without verifying TLS (we don't have the CA
// yet: that's the point) and accepts it only if it hashes to the value pinned
// in the join token. A man in the middle can serve a CA, but not one with
// that hash.
func fetchPinnedCA(server, wantHash string) ([]byte, error) {
	insecure := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // verified by hash below
			MinVersion:         tls.VersionTLS13,
		}},
	}
	resp, err := insecure.Get(strings.TrimRight(server, "/") + "/cacerts")
	if err != nil {
		return nil, fmt.Errorf("fetch CA: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, decodeHTTPError(resp)
	}
	caPEM, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, fmt.Errorf("read CA: %w", err)
	}
	if got := pki.Fingerprint(caPEM); got != wantHash {
		return nil, fmt.Errorf("controller CA hash %s does not match token (%s); refusing to join", got, wantHash)
	}
	return caPEM, nil
}

func requestNodeCert(server string, caPEM []byte, secret, nodeID string, csr []byte) ([]byte, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13}
	tlsConfig.RootCAs = poolFromPEM(caPEM)
	httpClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}

	body, err := json.Marshal(struct {
		NodeID string `json:"nodeId"`
		CSR    string `json:"csr"`
	}{NodeID: nodeID, CSR: string(csr)})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(server, "/")+"/nodes/join", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+secret)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("join: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("join: %w", decodeHTTPError(resp))
	}
	var out struct {
		Cert string `json:"cert"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("join: decode response: %w", err)
	}
	return []byte(out.Cert), nil
}
