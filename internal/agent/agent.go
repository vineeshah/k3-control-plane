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

	// tracked holds assignments this agent process has started or re-attached
	// to. It is only a cache: the runtime is the source of truth for what is
	// actually running, which is what lets workloads survive an agent restart.
	mu      sync.Mutex
	tracked map[string]struct{}
}

func New(node api.Node, client *client.Client, executor engineruntime.Executor, heartbeatInterval, pollInterval time.Duration) *Agent {
	return &Agent{
		node:              node,
		client:            client,
		executor:          executor,
		heartbeatInterval: heartbeatInterval,
		pollInterval:      pollInterval,
		tracked:           make(map[string]struct{}),
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
			// Leave workloads running. The next agent process adopts them.
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
