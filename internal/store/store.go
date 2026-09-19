package store

import (
	"sync"
	"time"

	"k8/internal/api"
)

// Store is the controller's desired and observed state. The controller is the
// only writer, so implementations only need to be safe for concurrent use by
// its HTTP handlers and reconcile loop, not across processes.
type Store interface {
	UpsertNode(node api.Node)
	TouchNode(id string, now time.Time) bool
	ListNodes() []api.Node

	UpsertService(service api.Service)
	UpdateServiceStatus(name string, status api.ServiceStatus) bool
	ListServices() []api.Service
	DeleteService(name string) bool

	UpsertJob(job api.Job)
	UpdateJobStatus(name string, status api.JobStatus) bool
	ListJobs() []api.Job
	DeleteJob(name string) bool

	SaveAssignment(assignment api.Assignment)
	DeleteAssignment(id string) bool
	UpdateAssignmentStatus(id string, phase api.AssignmentPhase, message string, now time.Time) (api.Assignment, bool)
	ListAssignments() []api.Assignment
	ListNodeAssignments(nodeID string) []api.Assignment
	ListAssignmentsForOwner(kind api.WorkloadKind, name string) []api.Assignment
	HasAssignment(id string) bool

	Snapshot() api.StateSnapshot
}

var (
	_ Store = (*MemoryStore)(nil)
	_ Store = (*SQLiteStore)(nil)
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
