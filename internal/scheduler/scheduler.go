package scheduler

import (
	"errors"
	"sort"
	"time"

	"k8/internal/api"
)

var ErrNoEligibleNode = errors.New("no eligible node found")

type Scheduler struct {
	NodeTimeout time.Duration
}

func New(nodeTimeout time.Duration) *Scheduler {
	return &Scheduler{NodeTimeout: nodeTimeout}
}

func (s *Scheduler) ChooseNode(now time.Time, nodes []api.Node, assignments []api.Assignment, req api.ResourceRequirements, placement api.Placement) (string, error) {
	liveNodes := make([]api.Node, 0, len(nodes))
	liveNodeIDs := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if now.Sub(node.LastHeartbeat) > s.NodeTimeout {
			continue
		}
		liveNodes = append(liveNodes, node)
		liveNodeIDs[node.ID] = struct{}{}
	}

	sort.Slice(liveNodes, func(i, j int) bool { return liveNodes[i].ID < liveNodes[j].ID })

	used := make(map[string]api.ResourceRequirements, len(liveNodes))
	for _, assignment := range assignments {
		if !api.IsActivePhase(assignment.Phase) {
			continue
		}
		if _, ok := liveNodeIDs[assignment.NodeID]; !ok {
			continue
		}
		current := used[assignment.NodeID]
		current.CPU += assignment.Resources.CPU
		current.Memory += assignment.Resources.Memory
		used[assignment.NodeID] = current
	}

	for _, node := range liveNodes {
		if !matchesPlacement(node.Labels, placement.RequiredLabels) {
			continue
		}
		current := used[node.ID]
		available := api.ResourceRequirements{
			CPU:    node.Capacity.CPU - current.CPU,
			Memory: node.Capacity.Memory - current.Memory,
		}
		if available.CPU < req.CPU || available.Memory < req.Memory {
			continue
		}
		return node.ID, nil
	}

	return "", ErrNoEligibleNode
}

func matchesPlacement(nodeLabels, required map[string]string) bool {
	for key, value := range required {
		if nodeLabels[key] != value {
			return false
		}
	}
	return true
}
