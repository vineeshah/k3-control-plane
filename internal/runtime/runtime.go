package runtime

import (
	"context"

	"k8/internal/api"
)

type Reporter func(phase api.AssignmentPhase, message string)

// Executor runs assignments on a node. Workloads belong to the runtime, not to
// the agent process: they keep running when the agent exits, and a restarted
// agent finds them again through List.
type Executor interface {
	// Start runs the assignment, or re-attaches to it if the runtime already
	// has it. Start must be idempotent: calling it for a known assignment
	// re-reports its current phase instead of creating a second workload.
	Start(ctx context.Context, assignment api.Assignment, report Reporter) error
	// Stop terminates the assignment's workload and forgets it.
	Stop(ctx context.Context, assignmentID string) error
	// List returns the IDs of every assignment the runtime currently holds.
	List(ctx context.Context) ([]string, error)
}
