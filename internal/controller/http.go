package controller

import "net/http"

type HTTPServer struct {
	controller *Controller
	auth       Auth
}

func NewHTTPServer(controller *Controller, auth Auth) *HTTPServer {
	return &HTTPServer{controller: controller, auth: auth}
}

func (s *HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()

	// Bootstrap: open, or authenticated by the join token.
	mux.HandleFunc("/cacerts", s.handleCACerts)
	mux.HandleFunc("/nodes/join", s.handleJoin)

	// User API: admin certificate.
	mux.HandleFunc("/services", adminOnly(s.handleServices))
	mux.HandleFunc("/services/", adminOnly(s.handleServices))
	mux.HandleFunc("/jobs", adminOnly(s.handleJobs))
	mux.HandleFunc("/jobs/", adminOnly(s.handleJobs))
	mux.HandleFunc("/nodes", adminOnly(s.handleNodes))
	mux.HandleFunc("/state", adminOnly(s.handleState))

	// Agent API: node certificate, and a node may only act as itself.
	mux.HandleFunc("/nodes/register", nodeOnly(s.handleNodeRegister))
	mux.HandleFunc("/nodes/heartbeat", nodeOnly(s.handleNodeHeartbeat))
	mux.HandleFunc("/nodes/", nodeOnly(s.handleNodeAssignments))
	mux.HandleFunc("/assignments/", nodeOnly(s.handleAssignmentStatus))
	return mux
}
