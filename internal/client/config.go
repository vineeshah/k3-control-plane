package client

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// AdminConfig is k8ctl's equivalent of a kubeconfig (k3s writes one to
// /etc/rancher/k3s/k3s.yaml). The controller writes it on every start.
type AdminConfig struct {
	Server string `json:"server"`
	CA     string `json:"ca"`
	Cert   string `json:"cert"`
	Key    string `json:"key"`
}

func LoadAdminConfig(path string) (*Client, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read admin config: %w", err)
	}
	var cfg AdminConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	tlsConfig, err := clientTLS([]byte(cfg.CA), []byte(cfg.Cert), []byte(cfg.Key))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return New(cfg.Server, tlsConfig), nil
}

func poolFromPEM(caPEM []byte) *x509.CertPool {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil
	}
	return pool
}

func clientTLS(caPEM, certPEM, keyPEM []byte) (*tls.Config, error) {
	pool := poolFromPEM(caPEM)
	if pool == nil {
		return nil, errors.New("no CA certificate found")
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}
	return &tls.Config{
		RootCAs:      pool,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}, nil
}
