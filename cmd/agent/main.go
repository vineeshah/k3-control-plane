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
	serverURL := flag.String("server", "https://127.0.0.1:6443", "controller base URL")
	token := flag.String("token", os.Getenv("K8_TOKEN"), "cluster join token K8<hash>::<secret>, needed on first join (also $K8_TOKEN)")
	dataDir := flag.String("data-dir", "/var/lib/k8-agent", "where the node's certificate and key are kept")
	nodeID := flag.String("node-id", "node-1", "node identifier")
	cpu := flag.Int("cpu", 1000, "node cpu capacity")
	memory := flag.Int("memory", 1024, "node memory capacity in MB")
	labels := flag.String("labels", "", "comma-separated key=value node labels")
	heartbeatInterval := flag.Duration("heartbeat-interval", 2*time.Second, "heartbeat cadence")
	pollInterval := flag.Duration("poll-interval", 2*time.Second, "assignment poll cadence")
	runtimeName := flag.String("runtime", "containerd", "container runtime: containerd or sim")
	containerdAddress := flag.String("containerd-address", "/run/containerd/containerd.sock", "containerd socket")
	logDir := flag.String("log-dir", "/var/log/k8", "where container stdout/stderr is written")
	jobDuration := flag.Duration("sim-job-duration", 2*time.Second, "how long simulated jobs take to complete (sim runtime)")
	flag.Parse()

	node := api.Node{
		ID:     *nodeID,
		Labels: parseMap(*labels),
		Capacity: api.ResourceRequirements{
			CPU:    *cpu,
			Memory: *memory,
		},
	}

	tlsConfig, err := client.BootstrapNode(*serverURL, *token, node.ID, *dataDir)
	if err != nil {
		log.Fatalf("join cluster: %v", err)
	}
	client := client.New(*serverURL, tlsConfig)
	var executor engineruntime.Executor
	switch *runtimeName {
	case "containerd":
		ctrd, err := engineruntime.NewContainerdExecutor(*containerdAddress, *logDir)
		if err != nil {
			log.Fatal(err)
		}
		defer ctrd.Close()
		executor = ctrd
	case "sim":
		executor = engineruntime.NewSimExecutor(*jobDuration)
	default:
		log.Fatalf("unknown runtime %q (want containerd or sim)", *runtimeName)
	}
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
