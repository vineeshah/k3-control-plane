package controller

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"k8/internal/api"
)

type HTTPServer struct {
	controller *Controller
}

func NewHTTPServer(controller *Controller) *HTTPServer {
	return &HTTPServer{controller: controller}
}

func (s *HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/services", s.handleServices)
	mux.HandleFunc("/jobs", s.handleJobs)
	mux.HandleFunc("/nodes", s.handleNodes)
	mux.HandleFunc("/nodes/register", s.handleNodeRegister)
	mux.HandleFunc("/nodes/heartbeat", s.handleNodeHeartbeat)
	mux.HandleFunc("/state", s.handleState)
	mux.HandleFunc("/nodes/", s.handleNodeAssignments)
	mux.HandleFunc("/assignments/", s.handleAssignmentStatus)
	return mux
}

func (s *HTTPServer) handleServices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.controller.Store().ListServices())
	case http.MethodPost:
		var service api.Service
		if !decodeJSON(w, r, &service) {
			return
		}
		if service.Name == "" || service.Image == "" {
			http.Error(w, "service name and image are required", http.StatusBadRequest)
			return
		}
		if service.Replicas <= 0 {
			service.Replicas = 1
		}
		service.Status.DesiredReplicas = service.Replicas
		s.controller.Store().UpsertService(service)
		writeJSON(w, http.StatusCreated, service)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.controller.Store().ListJobs())
	case http.MethodPost:
		var job api.Job
		if !decodeJSON(w, r, &job) {
			return
		}
		if job.Name == "" || job.Image == "" {
			http.Error(w, "job name and image are required", http.StatusBadRequest)
			return
		}
		s.controller.Store().UpsertJob(job)
		writeJSON(w, http.StatusCreated, job)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.controller.Store().ListNodes())
}

func (s *HTTPServer) handleNodeRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var node api.Node
	if !decodeJSON(w, r, &node) {
		return
	}
	if node.ID == "" {
		http.Error(w, "node id is required", http.StatusBadRequest)
		return
	}
	node.LastHeartbeat = time.Now().UTC()
	s.controller.Store().UpsertNode(node)
	writeJSON(w, http.StatusCreated, node)
}

func (s *HTTPServer) handleNodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		ID string `json:"id"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}
	if payload.ID == "" {
		http.Error(w, "node id is required", http.StatusBadRequest)
		return
	}
	if !s.controller.Store().TouchNode(payload.ID, time.Now().UTC()) {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *HTTPServer) handleNodeAssignments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasSuffix(r.URL.Path, "/assignments") {
		http.NotFound(w, r)
		return
	}
	trimmed := strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/assignments"), "/nodes/")
	if trimmed == "" {
		http.NotFound(w, r)
		return
	}
	assignments := s.controller.Store().ListNodeAssignments(trimmed)
	active := make([]api.Assignment, 0)
	for _, assignment := range assignments {
		if api.IsActivePhase(assignment.Phase) {
			active = append(active, assignment)
		}
	}
	writeJSON(w, http.StatusOK, active)
}

func (s *HTTPServer) handleAssignmentStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasSuffix(r.URL.Path, "/status") {
		http.NotFound(w, r)
		return
	}
	trimmed := strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/status"), "/assignments/")
	if trimmed == "" {
		http.NotFound(w, r)
		return
	}
	var payload struct {
		Phase   api.AssignmentPhase `json:"phase"`
		Message string              `json:"message"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}
	assignment, ok := s.controller.Store().UpdateAssignmentStatus(trimmed, payload.Phase, payload.Message, time.Now().UTC())
	if !ok {
		http.Error(w, "assignment not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, assignment)
}

func (s *HTTPServer) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.controller.Store().Snapshot())
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
