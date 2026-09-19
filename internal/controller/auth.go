package controller

import (
	"crypto/subtle"
	"net/http"
	"regexp"
	"strings"

	"k8/internal/pki"
)

// Auth is what the API needs to authenticate callers. Every connection is TLS.
// Admins and nodes prove who they are with client certificates issued by the
// cluster CA; a node gets its certificate by presenting the join secret once.
type Auth struct {
	CA         *pki.CA
	JoinSecret string
}

var validNodeID = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,61}[a-z0-9])?$`)

// callerCN returns the CommonName of a verified client certificate, or "".
func callerCN(r *http.Request) string {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.VerifiedChains[0]) == 0 {
		return ""
	}
	return r.TLS.VerifiedChains[0][0].Subject.CommonName
}

// adminOnly guards the user-facing API (what k8ctl calls).
func adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch callerCN(r) {
		case pki.AdminCN:
			next(w, r)
		case "":
			http.Error(w, "client certificate required", http.StatusUnauthorized)
		default:
			http.Error(w, "admin only", http.StatusForbidden)
		}
	}
}

// nodeOnly guards the agent API and passes the caller's node ID on, so each
// handler can make sure a node only acts as itself.
func nodeOnly(next func(w http.ResponseWriter, r *http.Request, nodeID string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cn := callerCN(r)
		nodeID := pki.NodeIDFromCN(cn)
		switch {
		case nodeID != "":
			next(w, r, nodeID)
		case cn == "":
			http.Error(w, "client certificate required", http.StatusUnauthorized)
		default:
			http.Error(w, "node credentials required", http.StatusForbidden)
		}
	}
}

// handleCACerts serves the CA bundle unauthenticated. It is public data; a
// joining agent checks it against the hash in its token before trusting it.
func (s *HTTPServer) handleCACerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	_, _ = w.Write(s.auth.CA.PEM)
}

type joinRequest struct {
	NodeID string `json:"nodeId"`
	CSR    string `json:"csr"`
}

type joinResponse struct {
	Cert string `json:"cert"`
}

// handleJoin trades the join secret and a CSR for a node client certificate.
// Anyone holding the token can join as any node ID, as with k3s: the token is
// the cluster secret, so guard it like one.
func (s *HTTPServer) handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	secret, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || subtle.ConstantTimeCompare([]byte(secret), []byte(s.auth.JoinSecret)) != 1 {
		http.Error(w, "invalid join token", http.StatusUnauthorized)
		return
	}

	var req joinRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validNodeID.MatchString(req.NodeID) {
		http.Error(w, "node id must be a lowercase DNS label", http.StatusBadRequest)
		return
	}
	cert, err := s.auth.CA.SignNodeCSR(req.NodeID, []byte(req.CSR))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, joinResponse{Cert: string(cert)})
}
