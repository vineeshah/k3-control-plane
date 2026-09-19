package scheduler

import (
	"testing"
	"time"

	"k8/internal/api"
)

func TestChooseNodePrefersDeterministicSmallestIDWhenAllElseEqual(t *testing.T) {
	now := time.Now().UTC()
	scheduler := New(10 * time.Second)

	nodes := []api.Node{
		{
			ID:            "node-b",
			Capacity:      api.ResourceRequirements{CPU: 1000, Memory: 1024},
			LastHeartbeat: now,
		},
		{
			ID:            "node-a",
			Capacity:      api.ResourceRequirements{CPU: 1000, Memory: 1024},
			LastHeartbeat: now,
		},
	}

	nodeID, err := scheduler.ChooseNode(now, nodes, nil, api.ResourceRequirements{CPU: 100, Memory: 128}, api.Placement{}, nil)
	if err != nil {
		t.Fatalf("ChooseNode returned error: %v", err)
	}
	if nodeID != "node-a" {
		t.Fatalf("expected node-a, got %s", nodeID)
	}
}

func TestChooseNodeRespectsPlacementAndResources(t *testing.T) {
	now := time.Now().UTC()
	scheduler := New(10 * time.Second)

	nodes := []api.Node{
		{
			ID:            "general",
			Labels:        map[string]string{"tier": "general"},
			Capacity:      api.ResourceRequirements{CPU: 1000, Memory: 1024},
			LastHeartbeat: now,
		},
		{
			ID:            "batch",
			Labels:        map[string]string{"tier": "batch"},
			Capacity:      api.ResourceRequirements{CPU: 1000, Memory: 1024},
			LastHeartbeat: now,
		},
	}

	assignments := []api.Assignment{
		{
			ID:        "service-1",
			NodeID:    "batch",
			Resources: api.ResourceRequirements{CPU: 950, Memory: 128},
			Phase:     api.AssignmentPhaseRunning,
		},
	}

	_, err := scheduler.ChooseNode(now, nodes, assignments, api.ResourceRequirements{CPU: 100, Memory: 128}, api.Placement{
		RequiredLabels: map[string]string{"tier": "batch"},
	}, nil)
	if err == nil {
		t.Fatal("expected no eligible node because the only matching node is full")
	}

	nodeID, err := scheduler.ChooseNode(now, nodes, assignments, api.ResourceRequirements{CPU: 100, Memory: 128}, api.Placement{
		RequiredLabels: map[string]string{"tier": "general"},
	}, nil)
	if err != nil {
		t.Fatalf("ChooseNode returned error: %v", err)
	}
	if nodeID != "general" {
		t.Fatalf("expected general, got %s", nodeID)
	}
}
