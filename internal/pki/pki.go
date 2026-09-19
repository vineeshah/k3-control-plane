// Package pki holds the cluster's certificate authority and the certificates
// it issues: a server cert for the controller, client certs for nodes and the
// admin. It mirrors what k3s does on first boot, minus the many component
// certs k3s needs and we don't.
package pki

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	caValidity     = 10 * 365 * 24 * time.Hour
	leafValidity   = 365 * 24 * time.Hour
	clockSkewSlack = 5 * time.Minute

	// Identities carried in the client certificate's CommonName.
	AdminCN    = "k8:admin"
	nodePrefix = "k8:node:"
)

func NodeCN(nodeID string) string { return nodePrefix + nodeID }

// NodeIDFromCN returns the node ID for a node identity, or "" if cn is not one.
func NodeIDFromCN(cn string) string {
	if len(cn) > len(nodePrefix) && cn[:len(nodePrefix)] == nodePrefix {
		return cn[len(nodePrefix):]
	}
	return ""
}

type CA struct {
	Cert *x509.Certificate
	Key  crypto.Signer
	PEM  []byte
}

// LoadOrCreateCA reads ca.crt/ca.key from dir, creating them on first boot.
// The CA is what the join token pins, so it must never change once created.
func LoadOrCreateCA(dir string) (*CA, error) {
	certPath, keyPath := filepath.Join(dir, "ca.crt"), filepath.Join(dir, "ca.key")
	certPEM, certErr := os.ReadFile(certPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	if certErr == nil && keyErr == nil {
		cert, err := ParseCert(certPEM)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", certPath, err)
		}
		key, err := ParseKey(keyPEM)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", keyPath, err)
		}
		return &CA{Cert: cert, Key: key, PEM: certPEM}, nil
	}
	if !errors.Is(certErr, os.ErrNotExist) || !errors.Is(keyErr, os.ErrNotExist) {
		return nil, fmt.Errorf("CA is half present in %s (cert: %v, key: %v); refusing to replace it", dir, certErr, keyErr)
	}

	key, err := NewKey()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "k8-ca@" + fmt.Sprint(now.Unix())},
		NotBefore:             now.Add(-clockSkewSlack),
		NotAfter:              now.Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		return nil, fmt.Errorf("self-sign CA: %w", err)
	}
	certPEM = EncodeCert(der)
	keyPEM, err = EncodeKey(key)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, err
	}
	if err := WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, err
	}
	cert, _ := x509.ParseCertificate(der)
	return &CA{Cert: cert, Key: key, PEM: certPEM}, nil
}

// ServerCert issues a fresh serving certificate for the given DNS names and
// IPs. The controller calls it on every start, so SAN changes and expiry are
// handled by restarting.
func (ca *CA) ServerCert(dnsNames []string, ips []net.IP) (tls.Certificate, error) {
	key, err := NewKey()
	if err != nil {
		return tls.Certificate{}, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: "k8-controller"},
		DNSNames:     dnsNames,
		IPAddresses:  ips,
		NotBefore:    now.Add(-clockSkewSlack),
		NotAfter:     now.Add(leafValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.Cert, key.Public(), ca.Key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("issue server cert: %w", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

// ClientCert signs a client certificate for pub with the given identity.
func (ca *CA) ClientCert(cn string, pub crypto.PublicKey) ([]byte, error) {
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    now.Add(-clockSkewSlack),
		NotAfter:     now.Add(leafValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.Cert, pub, ca.Key)
	if err != nil {
		return nil, fmt.Errorf("issue client cert for %s: %w", cn, err)
	}
	return EncodeCert(der), nil
}

// SignNodeCSR verifies a node's certificate request and issues it a client
// cert for nodeID. The CSR's own subject is ignored: identity comes from the
// join request, which the caller has already authenticated with the token.
func (ca *CA) SignNodeCSR(nodeID string, csrPEM []byte) ([]byte, error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, errors.New("expected a PEM certificate request")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse csr: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("csr signature: %w", err)
	}
	return ca.ClientCert(NodeCN(nodeID), csr.PublicKey)
}

func (ca *CA) Pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	return pool
}

// Fingerprint is the hex SHA-256 of a CA bundle's PEM bytes, the value the
// join token pins.
func Fingerprint(caPEM []byte) string {
	sum := sha256.Sum256(caPEM)
	return hex.EncodeToString(sum[:])
}

func NewKey() (*ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	return key, nil
}

func NewCSR(key crypto.Signer, cn string) ([]byte, error) {
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: cn},
	}, key)
	if err != nil {
		return nil, fmt.Errorf("create csr: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), nil
}

func EncodeCert(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func EncodeKey(key crypto.Signer) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func ParseCert(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("expected a PEM certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}

func ParseKey(keyPEM []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, errors.New("expected a PEM PKCS#8 private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, errors.New("private key cannot sign")
	}
	return signer, nil
}

// WriteFile writes data atomically: temp file, fsync, rename.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func serial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	return n
}
