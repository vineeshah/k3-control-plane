package store

import (
	"sort"
	"time"

	"k8/internal/api"
)

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
