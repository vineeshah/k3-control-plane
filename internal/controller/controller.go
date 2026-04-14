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
	for _, service := range c.store.ListServices() {
		c.reconcileService(now, service)
	}
	for _, job := range c.store.ListJobs() {
		c.reconcileJob(now, job)
	}
}

func (c *Controller) reconcileService(now time.Time, service api.Service) {
	assignments := c.store.ListAssignmentsForOwner(api.WorkloadKindService, service.Name)
	liveNodes := liveNodeSet(now, c.store.ListNodes(), c.nodeTimeout)

	active := make([]api.Assignment, 0)
	for _, assignment := range assignments {
		if !api.IsActivePhase(assignment.Phase) {
			continue
		}
		if _, ok := liveNodes[assignment.NodeID]; !ok {
			continue
		}
		active = append(active, assignment)
	}

	if len(active) > service.Replicas {
		excess := pickAssignmentsToRemove(active, len(active)-service.Replicas)
		for _, assignment := range excess {
			c.store.DeleteAssignment(assignment.ID)
		}
		assignments = c.store.ListAssignmentsForOwner(api.WorkloadKindService, service.Name)
		active = make([]api.Assignment, 0)
		for _, assignment := range assignments {
			if api.IsActivePhase(assignment.Phase) {
				if _, ok := liveNodes[assignment.NodeID]; ok {
					active = append(active, assignment)
				}
			}
		}
	}

	for len(active) < service.Replicas {
		nodeID, err := c.scheduler.ChooseNode(now, c.store.ListNodes(), c.store.ListAssignments(), service.Resources, service.Placement)
		if err != nil {
			break
		}
		assignment := api.Assignment{
			ID:        c.nextAssignmentID(service.Name, 1),
			OwnerKind: api.WorkloadKindService,
			OwnerName: service.Name,
			NodeID:    nodeID,
			Image:     service.Image,
			Command:   service.Command,
			Env:       service.Env,
			Resources: service.Resources,
			Volumes:   service.Volumes,
			Attempt:   1,
			Phase:     api.AssignmentPhasePending,
			CreatedAt: now,
		}
		c.store.SaveAssignment(assignment)
		active = append(active, assignment)
	}

	finalAssignments := c.store.ListAssignmentsForOwner(api.WorkloadKindService, service.Name)
	status := api.ServiceStatus{
		DesiredReplicas:    service.Replicas,
		LastReconciledUnix: now.Unix(),
	}
	for _, assignment := range finalAssignments {
		if !api.IsActivePhase(assignment.Phase) {
			continue
		}
		if _, ok := liveNodes[assignment.NodeID]; !ok {
			continue
		}
		status.AssignmentIDs = append(status.AssignmentIDs, assignment.ID)
		switch assignment.Phase {
		case api.AssignmentPhasePending:
			status.PendingReplicas++
		case api.AssignmentPhaseRunning:
			status.RunningReplicas++
		}
	}
	sort.Strings(status.AssignmentIDs)
	c.store.UpdateServiceStatus(service.Name, status)
}

func (c *Controller) reconcileJob(now time.Time, job api.Job) {
	assignments := c.store.ListAssignmentsForOwner(api.WorkloadKindJob, job.Name)
	liveNodes := liveNodeSet(now, c.store.ListNodes(), c.nodeTimeout)

	status := api.JobStatus{}
	var latestTerminal *api.Assignment
	var active *api.Assignment

	for _, assignment := range assignments {
		if assignment.Phase == api.AssignmentPhaseSucceeded {
			finished := assignment.FinishedAt
			status.Succeeded = true
			status.CompletedAt = finished
			status.LastAssignmentID = assignment.ID
			c.store.UpdateJobStatus(job.Name, status)
			return
		}

		if api.IsActivePhase(assignment.Phase) {
			if _, ok := liveNodes[assignment.NodeID]; ok {
				item := assignment
				active = &item
			}
			continue
		}

		if latestTerminal == nil || latestTerminal.CreatedAt.Before(assignment.CreatedAt) {
			item := assignment
			latestTerminal = &item
		}
		if assignment.Phase == api.AssignmentPhaseFailed {
			status.FailedAttempts++
			status.LastFailure = assignment.StatusMessage
		}
	}

	if active != nil {
		status.ActiveAssignmentID = active.ID
		status.LastAssignmentID = active.ID
		status.FailedAttempts = max(status.FailedAttempts, active.Attempt-1)
		c.store.UpdateJobStatus(job.Name, status)
		return
	}

	if latestTerminal != nil {
		status.LastAssignmentID = latestTerminal.ID
	}

	if status.FailedAttempts > job.Retries {
		c.store.UpdateJobStatus(job.Name, status)
		return
	}

	nodeID, err := c.scheduler.ChooseNode(now, c.store.ListNodes(), c.store.ListAssignments(), job.Resources, job.Placement)
	if err != nil {
		c.store.UpdateJobStatus(job.Name, status)
		return
	}

	assignment := api.Assignment{
		ID:        c.nextAssignmentID(job.Name, status.FailedAttempts+1),
		OwnerKind: api.WorkloadKindJob,
		OwnerName: job.Name,
		NodeID:    nodeID,
		Image:     job.Image,
		Command:   job.Command,
		Env:       job.Env,
		Resources: job.Resources,
		Volumes:   job.Volumes,
		Attempt:   status.FailedAttempts + 1,
		Phase:     api.AssignmentPhasePending,
		CreatedAt: now,
	}
	c.store.SaveAssignment(assignment)
	status.ActiveAssignmentID = assignment.ID
	status.LastAssignmentID = assignment.ID
	c.store.UpdateJobStatus(job.Name, status)
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
