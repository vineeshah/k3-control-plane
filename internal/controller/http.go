package controller

import "net/http"

type HTTPServer struct {
	controller *Controller
}

func NewHTTPServer(controller *Controller) *HTTPServer {
	return &HTTPServer{controller: controller}
}

func (s *HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/services", s.handleServices)
	mux.HandleFunc("/services/", s.handleServices)
	mux.HandleFunc("/jobs", s.handleJobs)
	mux.HandleFunc("/jobs/", s.handleJobs)
	mux.HandleFunc("/nodes", s.handleNodes)
	mux.HandleFunc("/nodes/register", s.handleNodeRegister)
	mux.HandleFunc("/nodes/heartbeat", s.handleNodeHeartbeat)
	mux.HandleFunc("/state", s.handleState)
	mux.HandleFunc("/nodes/", s.handleNodeAssignments)
	mux.HandleFunc("/assignments/", s.handleAssignmentStatus)
	return mux
}
