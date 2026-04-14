package store

import (
	"sort"
	"sync"
	"time"

	"k8/internal/api"
)

type MemoryStore struct {
	mu          sync.RWMutex
	nodes       map[string]api.Node
	services    map[string]api.Service
	jobs        map[string]api.Job
	assignments map[string]api.Assignment
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		nodes:       make(map[string]api.Node),
		services:    make(map[string]api.Service),
		jobs:        make(map[string]api.Job),
		assignments: make(map[string]api.Assignment),
	}
}

func (s *MemoryStore) UpsertNode(node api.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[node.ID] = node.Clone()
}

func (s *MemoryStore) TouchNode(id string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.nodes[id]
	if !ok {
		return false
	}
	node.LastHeartbeat = now
	s.nodes[id] = node
	return true
}

func (s *MemoryStore) ListNodes() []api.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nodes := make([]api.Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		nodes = append(nodes, node.Clone())
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes
}

func (s *MemoryStore) UpsertService(service api.Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.services[service.Name] = service.Clone()
}

func (s *MemoryStore) UpdateServiceStatus(name string, status api.ServiceStatus) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	service, ok := s.services[name]
	if !ok {
		return false
	}
	service.Status = status
	s.services[name] = service
	return true
}

func (s *MemoryStore) ListServices() []api.Service {
	s.mu.RLock()
	defer s.mu.RUnlock()

	services := make([]api.Service, 0, len(s.services))
	for _, service := range s.services {
		services = append(services, service.Clone())
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
	return services
}

func (s *MemoryStore) UpsertJob(job api.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.Name] = job.Clone()
}

func (s *MemoryStore) UpdateJobStatus(name string, status api.JobStatus) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[name]
	if !ok {
		return false
	}
	job.Status = status
	s.jobs[name] = job
	return true
}

func (s *MemoryStore) ListJobs() []api.Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]api.Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job.Clone())
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Name < jobs[j].Name })
	return jobs
}

func (s *MemoryStore) SaveAssignment(assignment api.Assignment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assignments[assignment.ID] = assignment.Clone()
}

func (s *MemoryStore) DeleteAssignment(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.assignments[id]; !ok {
		return false
	}
	delete(s.assignments, id)
	return true
}

func (s *MemoryStore) UpdateAssignmentStatus(id string, phase api.AssignmentPhase, message string, now time.Time) (api.Assignment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	assignment, ok := s.assignments[id]
	if !ok {
		return api.Assignment{}, false
	}
	assignment.Phase = phase
	assignment.StatusMessage = message
	if phase == api.AssignmentPhaseRunning && assignment.StartedAt == nil {
		assignment.StartedAt = &now
	}
	if api.IsTerminalPhase(phase) {
		assignment.FinishedAt = &now
	}
	s.assignments[id] = assignment
	return assignment.Clone(), true
}

func (s *MemoryStore) ListAssignments() []api.Assignment {
	s.mu.RLock()
	defer s.mu.RUnlock()

	assignments := make([]api.Assignment, 0, len(s.assignments))
	for _, assignment := range s.assignments {
		assignments = append(assignments, assignment.Clone())
	}
	sort.Slice(assignments, func(i, j int) bool {
		if assignments[i].CreatedAt.Equal(assignments[j].CreatedAt) {
			return assignments[i].ID < assignments[j].ID
		}
		return assignments[i].CreatedAt.Before(assignments[j].CreatedAt)
	})
	return assignments
}

func (s *MemoryStore) ListNodeAssignments(nodeID string) []api.Assignment {
	assignments := s.ListAssignments()
	filtered := make([]api.Assignment, 0)
	for _, assignment := range assignments {
		if assignment.NodeID == nodeID {
			filtered = append(filtered, assignment)
		}
	}
	return filtered
}

func (s *MemoryStore) ListAssignmentsForOwner(kind api.WorkloadKind, name string) []api.Assignment {
	assignments := s.ListAssignments()
	filtered := make([]api.Assignment, 0)
	for _, assignment := range assignments {
		if assignment.OwnerKind == kind && assignment.OwnerName == name {
			filtered = append(filtered, assignment)
		}
	}
	return filtered
}

func (s *MemoryStore) Snapshot() api.StateSnapshot {
	return api.StateSnapshot{
		Nodes:       s.ListNodes(),
		Services:    s.ListServices(),
		Jobs:        s.ListJobs(),
		Assignments: s.ListAssignments(),
	}
}
