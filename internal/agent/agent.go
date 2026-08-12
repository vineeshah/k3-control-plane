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
