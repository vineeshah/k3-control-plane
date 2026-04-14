package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"k8/internal/agent"
	"k8/internal/api"
	"k8/internal/client"
	engineruntime "k8/internal/runtime"
)

func main() {
	serverURL := flag.String("server", "http://127.0.0.1:8080", "controller base URL")
	nodeID := flag.String("node-id", "node-1", "node identifier")
	cpu := flag.Int("cpu", 1000, "node cpu capacity")
	memory := flag.Int("memory", 1024, "node memory capacity in MB")
	labels := flag.String("labels", "", "comma-separated key=value node labels")
	heartbeatInterval := flag.Duration("heartbeat-interval", 2*time.Second, "heartbeat cadence")
	pollInterval := flag.Duration("poll-interval", 2*time.Second, "assignment poll cadence")
	jobDuration := flag.Duration("fake-job-duration", 2*time.Second, "how long fake jobs take to complete")
	flag.Parse()

	node := api.Node{
		ID:     *nodeID,
		Labels: parseMap(*labels),
		Capacity: api.ResourceRequirements{
			CPU:    *cpu,
			Memory: *memory,
		},
	}

	client := client.New(*serverURL)
	executor := engineruntime.NewFakeExecutor(*jobDuration)
	agent := agent.New(node, client, executor, *heartbeatInterval, *pollInterval)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("agent %s connecting to %s", node.ID, *serverURL)
	if err := agent.Run(ctx); err != nil && err != context.Canceled {
		log.Fatal(err)
	}
}

func parseMap(input string) map[string]string {
	if input == "" {
		return nil
	}
	values := make(map[string]string)
	for _, pair := range strings.Split(input, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			continue
		}
		values[parts[0]] = parts[1]
	}
	return values
}
