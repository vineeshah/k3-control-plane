package controller

import (
	"crypto/rand"
	"fmt"
	"sort"
	"strings"
	"time"

	"k8/internal/api"
	"k8/internal/scheduler"
	"k8/internal/store"
)

type Controller struct {
	store       store.Store
	scheduler   *scheduler.Scheduler
	nodeTimeout time.Duration

	// startedAt is the time of the first reconcile. Only the reconcile loop
	// touches it.
	startedAt time.Time
}

func New(state store.Store, scheduler *scheduler.Scheduler, nodeTimeout time.Duration) *Controller {
	return &Controller{
		store:       state,
		scheduler:   scheduler,
		nodeTimeout: nodeTimeout,
	}
}

func (c *Controller) Store() store.Store {
	return c.store
}

func (c *Controller) Reconcile(now time.Time) {
	if c.startedAt.IsZero() {
		c.startedAt = now
	}
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
	liveNodes := liveNodeSet(now, c.nodes(), c.nodeTimeout)
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

// nodes returns the known nodes with startup grace applied. After a restart,
// or a restore from backup, every stored heartbeat is stale. Without grace
// the first reconcile would declare the whole cluster dead and reshuffle every
// workload. Instead a node known at boot counts as alive until one nodeTimeout
// after the controller started, giving its agent time to check back in.
func (c *Controller) nodes() []api.Node {
	nodes := c.store.ListNodes()
	for i := range nodes {
		if nodes[i].LastHeartbeat.Before(c.startedAt) {
			nodes[i].LastHeartbeat = c.startedAt
		}
	}
	return nodes
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

// nextAssignmentID returns "<owner>-<attempt>-<random>", like a Kubernetes pod
// name. The suffix is random rather than a counter so IDs stay unique across
// controller restarts and restores from an older backup, where a counter
// would roll back and reuse IDs of containers that are still running.
func (c *Controller) nextAssignmentID(owner string, attempt int) string {
	safeOwner := strings.ReplaceAll(strings.ToLower(owner), " ", "-")
	for {
		id := fmt.Sprintf("%s-%d-%s", safeOwner, attempt, randomSuffix(5))
		if !c.store.HasAssignment(id) {
			return id
		}
	}
}

const suffixAlphabet = "bcdfghjklmnpqrstvwxz2456789"

func randomSuffix(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	for i, b := range buf {
		buf[i] = suffixAlphabet[int(b)%len(suffixAlphabet)]
	}
	return string(buf)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
