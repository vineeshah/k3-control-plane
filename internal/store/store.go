package store

import (
	"sync"

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

func (s *MemoryStore) Snapshot() api.StateSnapshot {
	return api.StateSnapshot{
		Nodes:       s.ListNodes(),
		Services:    s.ListServices(),
		Jobs:        s.ListJobs(),
		Assignments: s.ListAssignments(),
	}
}
