package store

import (
	"testing"
	"time"

	"k8/internal/api"
)

func TestDeleteServiceRemovesItsAssignments(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now().UTC()
	s.UpsertService(api.Service{Name: "web", Image: "nginx"})
	s.SaveAssignment(api.Assignment{ID: "a1", OwnerKind: api.WorkloadKindService, OwnerName: "web", CreatedAt: now})
	s.SaveAssignment(api.Assignment{ID: "a2", OwnerKind: api.WorkloadKindJob, OwnerName: "web", CreatedAt: now})

	if !s.DeleteService("web") {
		t.Fatal("expected DeleteService to succeed for existing service")
	}
	if len(s.ListServices()) != 0 {
		t.Fatal("expected service to be gone")
	}
	if len(s.ListAssignments()) != 1 {
		t.Fatalf("expected only the job-owned assignment to remain, got %d", len(s.ListAssignments()))
	}
	if s.ListAssignments()[0].ID != "a2" {
		t.Fatalf("expected assignment a2 to remain, got %s", s.ListAssignments()[0].ID)
	}
}

func TestDeleteServiceReturnsFalseForMissing(t *testing.T) {
	s := NewMemoryStore()
	if s.DeleteService("nonexistent") {
		t.Fatal("expected false for missing service")
	}
}

func TestDeleteJobRemovesItsAssignments(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now().UTC()
	s.UpsertJob(api.Job{Name: "backup", Image: "alpine"})
	s.SaveAssignment(api.Assignment{ID: "a1", OwnerKind: api.WorkloadKindJob, OwnerName: "backup", CreatedAt: now})
	s.SaveAssignment(api.Assignment{ID: "a2", OwnerKind: api.WorkloadKindService, OwnerName: "backup", CreatedAt: now})

	if !s.DeleteJob("backup") {
		t.Fatal("expected DeleteJob to succeed for existing job")
	}
	if len(s.ListJobs()) != 0 {
		t.Fatal("expected job to be gone")
	}
	if len(s.ListAssignments()) != 1 {
		t.Fatalf("expected only the service-owned assignment to remain, got %d", len(s.ListAssignments()))
	}
	if s.ListAssignments()[0].ID != "a2" {
		t.Fatalf("expected assignment a2 to remain, got %s", s.ListAssignments()[0].ID)
	}
}

func TestDeleteJobReturnsFalseForMissing(t *testing.T) {
	s := NewMemoryStore()
	if s.DeleteJob("nonexistent") {
		t.Fatal("expected false for missing job")
	}
}
