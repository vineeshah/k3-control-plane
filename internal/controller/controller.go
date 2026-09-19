package controller

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"k8/internal/api"
	"k8/internal/scheduler"
	"k8/internal/store"
)

type Controller struct {
	store       *store.MemoryStore
	scheduler   *scheduler.Scheduler
	nodeTimeout time.Duration
	seq         uint64
}

func New(state *store.MemoryStore, scheduler *scheduler.Scheduler, nodeTimeout time.Duration) *Controller {
	return &Controller{
		store:       state,
		scheduler:   scheduler,
		nodeTimeout: nodeTimeout,
	}
}

func (c *Controller) Store() *store.MemoryStore {
	return c.store
}

func (c *Controller) Reconcile(now time.Time) {
	c.markLostAssignments(now)
	for _, service := range c.store.ListServices() {
		c.reconcileService(now, service)
	}
	for _, job := range c.store.ListJobs() {
		c.reconcileJob(now, job)
	}
}

// markLostAssignments moves active work on dead nodes to Lost so the owners
// reschedule it. Because Lost is terminal, a node that comes back is no longer
// handed that work and its agent stops it: no duplicate replicas.
func (c *Controller) markLostAssignments(now time.Time) {
	liveNodes := liveNodeSet(now, c.store.ListNodes(), c.nodeTimeout)
	for _, assignment := range c.store.ListAssignments() {
		if !api.IsActivePhase(assignment.Phase) {
			continue
		}
		if _, ok := liveNodes[assignment.NodeID]; ok {
			continue
		}
		message := fmt.Sprintf("node %s stopped heartbeating", assignment.NodeID)
		c.store.UpdateAssignmentStatus(assignment.ID, api.AssignmentPhaseLost, message, now)
	}
}

func liveNodeSet(now time.Time, nodes []api.Node, timeout time.Duration) map[string]struct{} {
	liveNodes := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if now.Sub(node.LastHeartbeat) <= timeout {
			liveNodes[node.ID] = struct{}{}
		}
	}
	return liveNodes
}

func pickAssignmentsToRemove(assignments []api.Assignment, count int) []api.Assignment {
	sorted := make([]api.Assignment, len(assignments))
	copy(sorted, assignments)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Phase != sorted[j].Phase {
			return sorted[i].Phase == api.AssignmentPhasePending
		}
		if sorted[i].CreatedAt.Equal(sorted[j].CreatedAt) {
			return sorted[i].ID > sorted[j].ID
		}
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})

	if count > len(sorted) {
		count = len(sorted)
	}
	return sorted[:count]
}

func (c *Controller) nextAssignmentID(owner string, attempt int) string {
	seq := atomic.AddUint64(&c.seq, 1)
	safeOwner := strings.ReplaceAll(strings.ToLower(owner), " ", "-")
	return fmt.Sprintf("%s-%d-%d", safeOwner, attempt, seq)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
