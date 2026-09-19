package agent

import (
	"context"
	"log"

	"k8/internal/api"
	engineruntime "k8/internal/runtime"
)

// syncAssignments converges the runtime on this node to what the controller
// wants. The controller only hands out active assignments, so anything the
// runtime holds that is not in that list (deleted, scaled down, marked Lost,
// or already finished and reported) gets stopped and reaped.
func (a *Agent) syncAssignments(ctx context.Context) error {
	assignments, err := a.client.FetchAssignments(a.node.ID)
	if err != nil {
		return err
	}

	desired := make(map[string]api.Assignment, len(assignments))
	for _, assignment := range assignments {
		if api.IsActivePhase(assignment.Phase) {
			desired[assignment.ID] = assignment
		}
	}

	existing, err := a.executor.List(ctx)
	if err != nil {
		return err
	}
	for _, id := range existing {
		if _, ok := desired[id]; ok {
			continue
		}
		if err := a.executor.Stop(ctx, id); err != nil {
			log.Printf("failed to stop assignment %s: %v", id, err)
			continue
		}
		a.untrack(id)
	}

	for _, assignment := range desired {
		if !a.track(assignment.ID) {
			continue
		}
		// Start is idempotent: for a workload that survived an agent restart
		// it re-attaches and re-reports instead of creating a duplicate.
		if err := a.executor.Start(ctx, assignment, a.reporter(assignment.ID)); err != nil {
			a.untrack(assignment.ID)
			if err := a.client.UpdateAssignmentStatus(assignment.ID, api.AssignmentPhaseFailed, err.Error()); err != nil {
				log.Printf("failed to update assignment %s status: %v", assignment.ID, err)
			}
		}
	}

	return nil
}

func (a *Agent) reporter(id string) engineruntime.Reporter {
	return func(phase api.AssignmentPhase, message string) {
		if err := a.client.UpdateAssignmentStatus(id, phase, message); err != nil {
			log.Printf("failed to update assignment %s status: %v", id, err)
		}
		// Once finished, stop tracking. If the report above was lost the
		// controller still lists the assignment as active, so the next sync
		// calls Start again and the runtime re-reports the final phase.
		if api.IsTerminalPhase(phase) {
			a.untrack(id)
		}
	}
}

// track marks id as handled by this process and reports whether it was new.
func (a *Agent) track(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.tracked[id]; ok {
		return false
	}
	a.tracked[id] = struct{}{}
	return true
}

func (a *Agent) untrack(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.tracked, id)
}
