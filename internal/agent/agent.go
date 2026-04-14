package agent

import (
	"context"
	"log"
	"sync"
	"time"

	"k8/internal/api"
	"k8/internal/client"
	engineruntime "k8/internal/runtime"
)

type Agent struct {
	node              api.Node
	client            *client.Client
	executor          engineruntime.Executor
	heartbeatInterval time.Duration
	pollInterval      time.Duration

	mu      sync.Mutex
	running map[string]context.CancelFunc
}

func New(node api.Node, client *client.Client, executor engineruntime.Executor, heartbeatInterval, pollInterval time.Duration) *Agent {
	return &Agent{
		node:              node,
		client:            client,
		executor:          executor,
		heartbeatInterval: heartbeatInterval,
		pollInterval:      pollInterval,
		running:           make(map[string]context.CancelFunc),
	}
}

func (a *Agent) Run(ctx context.Context) error {
	if err := a.client.RegisterNode(a.node); err != nil {
		return err
	}

	if err := a.syncAssignments(ctx); err != nil {
		log.Printf("initial assignment sync failed: %v", err)
	}

	heartbeatTicker := time.NewTicker(a.heartbeatInterval)
	defer heartbeatTicker.Stop()

	pollTicker := time.NewTicker(a.pollInterval)
	defer pollTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.stopAll()
			return ctx.Err()
		case <-heartbeatTicker.C:
			if err := a.client.Heartbeat(a.node.ID); err != nil {
				log.Printf("heartbeat failed: %v", err)
			}
		case <-pollTicker.C:
			if err := a.syncAssignments(ctx); err != nil {
				log.Printf("assignment sync failed: %v", err)
			}
		}
	}
}

func (a *Agent) syncAssignments(ctx context.Context) error {
	assignments, err := a.client.FetchAssignments(a.node.ID)
	if err != nil {
		return err
	}

	desired := make(map[string]api.Assignment, len(assignments))
	for _, assignment := range assignments {
		desired[assignment.ID] = assignment
	}

	a.mu.Lock()
	for id, cancel := range a.running {
		if _, ok := desired[id]; ok {
			continue
		}
		cancel()
		delete(a.running, id)
	}
	a.mu.Unlock()

	for _, assignment := range assignments {
		if !api.IsActivePhase(assignment.Phase) {
			continue
		}

		a.mu.Lock()
		_, alreadyRunning := a.running[assignment.ID]
		a.mu.Unlock()
		if alreadyRunning {
			continue
		}

		runCtx, cancel := context.WithCancel(ctx)
		a.mu.Lock()
		a.running[assignment.ID] = cancel
		a.mu.Unlock()

		err := a.executor.Start(runCtx, assignment, func(phase api.AssignmentPhase, message string) {
			if err := a.client.UpdateAssignmentStatus(assignment.ID, phase, message); err != nil {
				log.Printf("failed to update assignment %s status: %v", assignment.ID, err)
			}
			if api.IsTerminalPhase(phase) {
				a.mu.Lock()
				delete(a.running, assignment.ID)
				a.mu.Unlock()
			}
		})
		if err != nil {
			cancel()
			a.mu.Lock()
			delete(a.running, assignment.ID)
			a.mu.Unlock()
			_ = a.client.UpdateAssignmentStatus(assignment.ID, api.AssignmentPhaseFailed, err.Error())
		}
	}

	return nil
}

func (a *Agent) stopAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, cancel := range a.running {
		cancel()
		delete(a.running, id)
	}
}
