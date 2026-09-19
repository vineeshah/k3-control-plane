package controller

import (
	"sort"
	"time"

	"k8/internal/api"
)

func (c *Controller) reconcileService(now time.Time, service api.Service) {
	assignments := c.store.ListAssignmentsForOwner(api.WorkloadKindService, service.Name)
	liveNodes := liveNodeSet(now, c.nodes(), c.nodeTimeout)

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
		nodeID, err := c.scheduler.ChooseNode(now, c.nodes(), c.store.ListAssignments(), service.Resources, service.Placement, service.Ports)
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
			Ports:     service.Ports,
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
