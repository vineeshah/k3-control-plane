package controller

import (
	"net/http"
	"strings"
	"time"

	"k8/internal/api"
)

func (s *HTTPServer) handleServices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path != "/services" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, s.controller.Store().ListServices())
	case http.MethodPost:
		if r.URL.Path != "/services" {
			http.NotFound(w, r)
			return
		}
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
	case http.MethodDelete:
		name := strings.TrimPrefix(r.URL.Path, "/services/")
		if name == "" || strings.Contains(name, "/") {
			http.Error(w, "service name required", http.StatusBadRequest)
			return
		}
		if !s.controller.Store().DeleteService(name) {
			http.Error(w, "service not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path != "/jobs" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, s.controller.Store().ListJobs())
	case http.MethodPost:
		if r.URL.Path != "/jobs" {
			http.NotFound(w, r)
			return
		}
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
	case http.MethodDelete:
		name := strings.TrimPrefix(r.URL.Path, "/jobs/")
		if name == "" || strings.Contains(name, "/") {
			http.Error(w, "job name required", http.StatusBadRequest)
			return
		}
		if !s.controller.Store().DeleteJob(name) {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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

func (s *HTTPServer) handleNodeRegister(w http.ResponseWriter, r *http.Request, caller string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var node api.Node
	if !decodeJSON(w, r, &node) {
		return
	}
	if node.ID != caller {
		http.Error(w, "a node can only register itself", http.StatusForbidden)
		return
	}
	node.LastHeartbeat = time.Now().UTC()
	s.controller.Store().UpsertNode(node)
	writeJSON(w, http.StatusCreated, node)
}

func (s *HTTPServer) handleNodeHeartbeat(w http.ResponseWriter, r *http.Request, caller string) {
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
	if payload.ID != caller {
		http.Error(w, "a node can only heartbeat for itself", http.StatusForbidden)
		return
	}
	if !s.controller.Store().TouchNode(payload.ID, time.Now().UTC()) {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *HTTPServer) handleNodeAssignments(w http.ResponseWriter, r *http.Request, caller string) {
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
	if trimmed != caller {
		http.Error(w, "a node can only read its own assignments", http.StatusForbidden)
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

func (s *HTTPServer) handleAssignmentStatus(w http.ResponseWriter, r *http.Request, caller string) {
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
	if !isAgentReportablePhase(payload.Phase) {
		http.Error(w, "agents may report Running, Succeeded, Failed or Stopped", http.StatusBadRequest)
		return
	}
	if !s.ownsAssignment(caller, trimmed) {
		http.Error(w, "assignment not found on this node", http.StatusNotFound)
		return
	}
	assignment, ok := s.controller.Store().UpdateAssignmentStatus(trimmed, payload.Phase, payload.Message, time.Now().UTC())
	if !ok {
		http.Error(w, "assignment not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, assignment)
}

func (s *HTTPServer) ownsAssignment(nodeID, assignmentID string) bool {
	for _, assignment := range s.controller.Store().ListNodeAssignments(nodeID) {
		if assignment.ID == assignmentID {
			return true
		}
	}
	return false
}

// isAgentReportablePhase: Pending is the controller's starting point and Lost
// is its verdict on a dead node; neither is an agent's to set.
func isAgentReportablePhase(phase api.AssignmentPhase) bool {
	switch phase {
	case api.AssignmentPhaseRunning, api.AssignmentPhaseSucceeded, api.AssignmentPhaseFailed, api.AssignmentPhaseStopped:
		return true
	default:
		return false
	}
}

func (s *HTTPServer) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.controller.Store().Snapshot())
}
