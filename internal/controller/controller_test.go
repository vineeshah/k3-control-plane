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
