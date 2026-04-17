package store

import (
	"testing"
	"time"

	"k8/internal/api"
)

func TestTouchNodeReturnsFalseForMissingNode(t *testing.T) {
	s := NewMemoryStore()
	if s.TouchNode("nonexistent", time.Now()) {
		t.Fatal("expected false for missing node")
	}
}

func TestTouchNodeUpdatesHeartbeat(t *testing.T) {
	s := NewMemoryStore()
	s.UpsertNode(api.Node{ID: "n1", Capacity: api.ResourceRequirements{CPU: 100, Memory: 256}})

	later := time.Now().Add(time.Minute)
	if !s.TouchNode("n1", later) {
		t.Fatal("expected true for existing node")
	}
	nodes := s.ListNodes()
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if !nodes[0].LastHeartbeat.Equal(later) {
		t.Fatalf("expected heartbeat %v, got %v", later, nodes[0].LastHeartbeat)
	}
}

func TestDeleteAssignmentReturnsFalseForMissing(t *testing.T) {
	s := NewMemoryStore()
	if s.DeleteAssignment("nonexistent") {
		t.Fatal("expected false for missing assignment")
	}
}

func TestDeleteAssignmentRemovesExisting(t *testing.T) {
	s := NewMemoryStore()
	s.SaveAssignment(api.Assignment{ID: "a1", NodeID: "n1", Phase: api.AssignmentPhasePending, CreatedAt: time.Now()})
	if !s.DeleteAssignment("a1") {
		t.Fatal("expected true for existing assignment")
	}
	if len(s.ListAssignments()) != 0 {
		t.Fatal("expected no assignments after delete")
	}
}

func TestUpdateAssignmentStatusSetsStartedAtOnRunning(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now().UTC()
	s.SaveAssignment(api.Assignment{ID: "a1", Phase: api.AssignmentPhasePending, CreatedAt: now})

	start := now.Add(time.Second)
	a, ok := s.UpdateAssignmentStatus("a1", api.AssignmentPhaseRunning, "", start)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if a.StartedAt == nil {
		t.Fatal("expected StartedAt to be set")
	}
	if !a.StartedAt.Equal(start) {
		t.Fatalf("expected StartedAt=%v, got %v", start, *a.StartedAt)
	}
}

func TestUpdateAssignmentStatusDoesNotOverrideStartedAt(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now().UTC()
	s.SaveAssignment(api.Assignment{ID: "a1", Phase: api.AssignmentPhasePending, CreatedAt: now})

	start := now.Add(time.Second)
	s.UpdateAssignmentStatus("a1", api.AssignmentPhaseRunning, "", start)

	later := now.Add(2 * time.Second)
	a, _ := s.UpdateAssignmentStatus("a1", api.AssignmentPhaseRunning, "", later)
	if !a.StartedAt.Equal(start) {
		t.Fatalf("StartedAt should not change on second Running update: got %v", *a.StartedAt)
	}
}

func TestUpdateAssignmentStatusSetsFinishedAtOnTerminal(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now().UTC()
	s.SaveAssignment(api.Assignment{ID: "a1", Phase: api.AssignmentPhaseRunning, CreatedAt: now})

	finish := now.Add(5 * time.Second)
	a, ok := s.UpdateAssignmentStatus("a1", api.AssignmentPhaseSucceeded, "done", finish)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if a.FinishedAt == nil {
		t.Fatal("expected FinishedAt to be set on terminal phase")
	}
	if !a.FinishedAt.Equal(finish) {
		t.Fatalf("expected FinishedAt=%v, got %v", finish, *a.FinishedAt)
	}
}

func TestUpdateAssignmentStatusReturnsFalseForMissing(t *testing.T) {
	s := NewMemoryStore()
	_, ok := s.UpdateAssignmentStatus("nonexistent", api.AssignmentPhaseRunning, "", time.Now())
	if ok {
		t.Fatal("expected false for missing assignment")
	}
}

func TestListAssignmentsSortedByCreatedAtThenID(t *testing.T) {
	s := NewMemoryStore()
	base := time.Now().UTC()

	s.SaveAssignment(api.Assignment{ID: "a2", CreatedAt: base.Add(time.Second)})
	s.SaveAssignment(api.Assignment{ID: "a1", CreatedAt: base.Add(time.Second)}) // same time, earlier ID
	s.SaveAssignment(api.Assignment{ID: "a3", CreatedAt: base.Add(2 * time.Second)})

	list := s.ListAssignments()
	if len(list) != 3 {
		t.Fatalf("expected 3 assignments, got %d", len(list))
	}
	order := []string{list[0].ID, list[1].ID, list[2].ID}
	want := []string{"a1", "a2", "a3"}
	for i, id := range want {
		if order[i] != id {
			t.Fatalf("expected order %v, got %v", want, order)
		}
	}
}

func TestListNodeAssignmentsFilters(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now().UTC()
	s.SaveAssignment(api.Assignment{ID: "a1", NodeID: "n1", CreatedAt: now})
	s.SaveAssignment(api.Assignment{ID: "a2", NodeID: "n2", CreatedAt: now.Add(time.Second)})

	list := s.ListNodeAssignments("n1")
	if len(list) != 1 || list[0].ID != "a1" {
		t.Fatalf("expected only a1 for n1, got %v", list)
	}
}

func TestListAssignmentsForOwnerFilters(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now().UTC()
	s.SaveAssignment(api.Assignment{ID: "a1", OwnerKind: api.WorkloadKindService, OwnerName: "svc", CreatedAt: now})
	s.SaveAssignment(api.Assignment{ID: "a2", OwnerKind: api.WorkloadKindJob, OwnerName: "svc", CreatedAt: now.Add(time.Second)})

	list := s.ListAssignmentsForOwner(api.WorkloadKindService, "svc")
	if len(list) != 1 || list[0].ID != "a1" {
		t.Fatalf("expected only a1 for service/svc, got %v", list)
	}
}
