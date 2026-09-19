package controller

import (
	"testing"
	"time"

	"k8/internal/api"
	"k8/internal/scheduler"
	"k8/internal/store"
)

func TestReconcileServiceCreatesAssignmentsAndScalesDown(t *testing.T) {
	now := time.Now().UTC()
	state := store.NewMemoryStore()
	state.UpsertNode(api.Node{
		ID:            "node-a",
		Capacity:      api.ResourceRequirements{CPU: 1000, Memory: 1024},
		LastHeartbeat: now,
	})

	ctrl := New(state, scheduler.New(10*time.Second), 10*time.Second)
	state.UpsertService(api.Service{
		Name:     "web",
		Image:    "nginx",
		Replicas: 2,
		Resources: api.ResourceRequirements{
			CPU:    100,
			Memory: 128,
		},
	})

	ctrl.Reconcile(now)

	assignments := state.ListAssignmentsForOwner(api.WorkloadKindService, "web")
	if len(assignments) != 2 {
		t.Fatalf("expected 2 assignments after first reconcile, got %d", len(assignments))
	}

	services := state.ListServices()
	if len(services) != 1 {
		t.Fatalf("expected one service, got %d", len(services))
	}
	if services[0].Status.PendingReplicas != 2 {
		t.Fatalf("expected 2 pending replicas, got %d", services[0].Status.PendingReplicas)
	}

	service := services[0]
	service.Replicas = 1
	state.UpsertService(service)
	ctrl.Reconcile(now.Add(2 * time.Second))

	assignments = state.ListAssignmentsForOwner(api.WorkloadKindService, "web")
	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment after scale down, got %d", len(assignments))
	}
}

func TestReconcileJobSchedulesRetryAfterFailure(t *testing.T) {
	now := time.Now().UTC()
	state := store.NewMemoryStore()
	state.UpsertNode(api.Node{
		ID:            "node-a",
		Capacity:      api.ResourceRequirements{CPU: 1000, Memory: 1024},
		LastHeartbeat: now,
	})

	ctrl := New(state, scheduler.New(10*time.Second), 10*time.Second)
	state.UpsertJob(api.Job{
		Name:    "backup",
		Image:   "alpine",
		Retries: 1,
		Resources: api.ResourceRequirements{
			CPU:    100,
			Memory: 128,
		},
	})

	ctrl.Reconcile(now)
	assignments := state.ListAssignmentsForOwner(api.WorkloadKindJob, "backup")
	if len(assignments) != 1 {
		t.Fatalf("expected first attempt to be scheduled, got %d assignments", len(assignments))
	}

	if _, ok := state.UpdateAssignmentStatus(assignments[0].ID, api.AssignmentPhaseFailed, "boom", now.Add(time.Second)); !ok {
		t.Fatalf("failed to mark assignment %s as failed", assignments[0].ID)
	}

	ctrl.Reconcile(now.Add(2 * time.Second))
	assignments = state.ListAssignmentsForOwner(api.WorkloadKindJob, "backup")
	if len(assignments) != 2 {
		t.Fatalf("expected retry attempt to be scheduled, got %d assignments", len(assignments))
	}

	jobs := state.ListJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected one job, got %d", len(jobs))
	}
	if jobs[0].Status.FailedAttempts != 1 {
		t.Fatalf("expected failedAttempts to be 1, got %d", jobs[0].Status.FailedAttempts)
	}
	if jobs[0].Status.ActiveAssignmentID == "" {
		t.Fatal("expected active retry assignment to be set")
	}
}

func TestReconcileMarksDeadNodeAssignmentsLostAndReschedules(t *testing.T) {
	now := time.Now().UTC()
	state := store.NewMemoryStore()
	for _, id := range []string{"node-a", "node-b"} {
		state.UpsertNode(api.Node{
			ID:            id,
			Capacity:      api.ResourceRequirements{CPU: 1000, Memory: 1024},
			LastHeartbeat: now,
		})
	}

	ctrl := New(state, scheduler.New(10*time.Second), 10*time.Second)
	state.UpsertService(api.Service{
		Name:      "web",
		Image:     "nginx",
		Replicas:  2,
		Resources: api.ResourceRequirements{CPU: 100, Memory: 128},
	})
	ctrl.Reconcile(now)

	original := state.ListAssignmentsForOwner(api.WorkloadKindService, "web")
	if len(original) != 2 || original[0].NodeID != "node-a" || original[1].NodeID != "node-a" {
		t.Fatalf("expected both replicas on node-a, got %+v", original)
	}

	// node-a stops heartbeating; node-b stays alive.
	later := now.Add(30 * time.Second)
	state.TouchNode("node-b", later)
	ctrl.Reconcile(later)

	activeOn := map[string]int{}
	for _, assignment := range state.ListAssignmentsForOwner(api.WorkloadKindService, "web") {
		if api.IsActivePhase(assignment.Phase) {
			activeOn[assignment.NodeID]++
			continue
		}
		if assignment.Phase != api.AssignmentPhaseLost {
			t.Fatalf("expected inactive assignment %s to be Lost, got %s", assignment.ID, assignment.Phase)
		}
	}
	if activeOn["node-b"] != 2 || activeOn["node-a"] != 0 {
		t.Fatalf("expected 2 active replicas on node-b only, got %v", activeOn)
	}

	// node-a comes back: its old replicas must not be resurrected.
	returned := later.Add(time.Second)
	state.TouchNode("node-a", returned)
	state.TouchNode("node-b", returned)
	ctrl.Reconcile(returned)

	active := 0
	for _, assignment := range state.ListAssignmentsForOwner(api.WorkloadKindService, "web") {
		if api.IsActivePhase(assignment.Phase) {
			active++
		}
	}
	if active != 2 {
		t.Fatalf("expected exactly 2 active replicas after node-a returns, got %d", active)
	}
}

func TestReconcileRetriesLostJobWithoutCountingFailure(t *testing.T) {
	now := time.Now().UTC()
	state := store.NewMemoryStore()
	for _, id := range []string{"node-a", "node-b"} {
		state.UpsertNode(api.Node{
			ID:            id,
			Capacity:      api.ResourceRequirements{CPU: 1000, Memory: 1024},
			LastHeartbeat: now,
		})
	}

	ctrl := New(state, scheduler.New(10*time.Second), 10*time.Second)
	state.UpsertJob(api.Job{
		Name:      "backup",
		Image:     "alpine",
		Retries:   0,
		Resources: api.ResourceRequirements{CPU: 100, Memory: 128},
	})
	ctrl.Reconcile(now)

	later := now.Add(30 * time.Second)
	state.TouchNode("node-b", later)
	ctrl.Reconcile(later)

	jobs := state.ListJobs()
	if jobs[0].Status.FailedAttempts != 0 {
		t.Fatalf("a lost node is not a job failure: expected 0 failed attempts, got %d", jobs[0].Status.FailedAttempts)
	}
	active := jobs[0].Status.ActiveAssignmentID
	if active == "" {
		t.Fatal("expected job to be rescheduled after its node was lost")
	}
	for _, assignment := range state.ListAssignmentsForOwner(api.WorkloadKindJob, "backup") {
		if assignment.ID == active && assignment.NodeID != "node-b" {
			t.Fatalf("expected retry on node-b, got %s", assignment.NodeID)
		}
	}
}
