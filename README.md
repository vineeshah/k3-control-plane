# k8

`k8` is a small Kubernetes-inspired learning project written in Go. This first slice intentionally focuses on the orchestration loop instead of real container execution, so you can study:

- desired state vs actual state
- reconciliation
- deterministic scheduling
- controller/agent communication
- service replicas and one-off jobs

## What exists today

- `cmd/controller`: central control plane with an in-memory store and reconcile loop
- `cmd/agent`: node agent that registers, heartbeats, polls for assignments, and runs fake workloads
- `cmd/k8ctl`: small CLI for creating services/jobs and inspecting cluster state
- `internal/scheduler`: deterministic CPU/memory/label-aware scheduler
- `internal/runtime`: fake executor that keeps services running and completes or fails jobs

## Run flow

1. Start the controller.
2. Start one or more agents.
3. Create a service or job with `k8ctl`.
4. Watch the controller reconcile desired state into assignments.
5. Inspect state with `k8ctl state`.

## Example commands

```bash
go run ./cmd/controller
go run ./cmd/agent -node-id node-a -cpu 1000 -memory 2048 -labels tier=general
go run ./cmd/agent -node-id node-b -cpu 500 -memory 1024 -labels tier=batch

go run ./cmd/k8ctl service create -name web -image nginx -replicas 2 -cpu 200 -memory 256
go run ./cmd/k8ctl job create -name backup -image alpine -command "backup now" -retries 1
go run ./cmd/k8ctl state
```

To simulate a failed job, include the word `fail` in the job command:

```bash
go run ./cmd/k8ctl job create -name flaky -image alpine -command "fail once" -retries 1
```

## Learning checkpoints

- Controller: how desired state turns into assignments
- Scheduler: how node eligibility and resource accounting work
- Agent: how polling and heartbeats coordinate with the controller
- Runtime: where fake execution can later be swapped for `containerd`
