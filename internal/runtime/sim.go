package runtime

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"k8/internal/api"
)

// SimExecutor simulates a container runtime in memory, for running the control
// plane without containerd. Services run until stopped; jobs finish after
// JobDuration and fail if their command contains "fail". Like a real runtime,
// its state belongs to it and not to the agent that calls it.
type SimExecutor struct {
	JobDuration time.Duration

	mu    sync.Mutex
	tasks map[string]*simTask
}

type simTask struct {
	stop    chan struct{}
	phase   api.AssignmentPhase
	message string
	report  Reporter
}

func NewSimExecutor(jobDuration time.Duration) *SimExecutor {
	return &SimExecutor{
		JobDuration: jobDuration,
		tasks:       make(map[string]*simTask),
	}
}

func (s *SimExecutor) Start(_ context.Context, assignment api.Assignment, report Reporter) error {
	s.mu.Lock()
	if task, ok := s.tasks[assignment.ID]; ok {
		task.report = report
		phase, message := task.phase, task.message
		s.mu.Unlock()
		report(phase, message)
		return nil
	}
	task := &simTask{
		stop:    make(chan struct{}),
		phase:   api.AssignmentPhaseRunning,
		message: "simulated runtime started assignment",
		report:  report,
	}
	s.tasks[assignment.ID] = task
	s.mu.Unlock()

	report(task.phase, task.message)
	if assignment.OwnerKind == api.WorkloadKindJob {
		go s.runJob(assignment, task)
	}
	return nil
}

func (s *SimExecutor) runJob(assignment api.Assignment, task *simTask) {
	select {
	case <-task.stop:
		return
	case <-time.After(s.JobDuration):
	}

	phase, message := api.AssignmentPhaseSucceeded, "simulated job completed"
	if shouldFail(assignment.Command) {
		phase, message = api.AssignmentPhaseFailed, "simulated job failed because command contains 'fail'"
	}

	s.mu.Lock()
	task.phase, task.message = phase, message
	report := task.report
	s.mu.Unlock()
	report(phase, message)
}

func (s *SimExecutor) Stop(_ context.Context, assignmentID string) error {
	s.mu.Lock()
	task, ok := s.tasks[assignmentID]
	delete(s.tasks, assignmentID)
	s.mu.Unlock()
	if !ok {
		return nil
	}

	close(task.stop)
	if !api.IsTerminalPhase(task.phase) {
		task.report(api.AssignmentPhaseStopped, "stopped by agent")
	}
	return nil
}

func (s *SimExecutor) List(context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.tasks))
	for id := range s.tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func shouldFail(command []string) bool {
	for _, part := range command {
		if strings.Contains(strings.ToLower(part), "fail") {
			return true
		}
	}
	return false
}
