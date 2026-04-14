package runtime

import (
	"context"
	"strings"
	"time"

	"k8/internal/api"
)

type Reporter func(phase api.AssignmentPhase, message string)

type Executor interface {
	Start(ctx context.Context, assignment api.Assignment, report Reporter) error
}

type FakeExecutor struct {
	JobDuration time.Duration
}

func NewFakeExecutor(jobDuration time.Duration) *FakeExecutor {
	return &FakeExecutor{JobDuration: jobDuration}
}

func (f *FakeExecutor) Start(ctx context.Context, assignment api.Assignment, report Reporter) error {
	go func() {
		report(api.AssignmentPhaseRunning, "fake runtime started assignment")

		if assignment.OwnerKind == api.WorkloadKindService {
			<-ctx.Done()
			report(api.AssignmentPhaseStopped, "service stopped by agent")
			return
		}

		select {
		case <-ctx.Done():
			report(api.AssignmentPhaseStopped, "job stopped before completion")
		case <-time.After(f.JobDuration):
			if shouldFail(assignment.Command) {
				report(api.AssignmentPhaseFailed, "fake job failed because command contains 'fail'")
				return
			}
			report(api.AssignmentPhaseSucceeded, "fake job completed")
		}
	}()
	return nil
}

func shouldFail(command []string) bool {
	for _, part := range command {
		if strings.Contains(strings.ToLower(part), "fail") {
			return true
		}
	}
	return false
}
