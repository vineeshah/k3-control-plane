package controller

import (
	"time"

	"k8/internal/api"
)

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
