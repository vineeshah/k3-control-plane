# k3 homelab

A Kubernetes control plane I built from scratch — and the thing that schedules the workloads on my homelab.

To learn and deploy k8 on real nodes, I built: a controller with a reconcile loop, a deterministic scheduler, and node agents that register, heartbeat, and run whatever the control plane hands them. No Kubernetes libraries, no frameworks — just Go, HTTP, and the same ideas the real thing is built on. It runs on my homelab nodes - 3 as of now and so far it works great!

This repo is deliberately small and personal. There's one controller binary, one agent binary, and one CLI. No CRDs, no webhooks, no etcd - might seem counter-intutive and unnecessary but because it's custom-made and entirely mine, when a specific bug bites I can trace it to a single line and fix it in minutes instead of debugging code I didn't write.

## How it thinks

Every node runs an agent. The agent registers with the controller, heartbeats so the controller knows it's alive, and polls for assignments — nothing more. The agent is deliberately dumb.

The controller holds the *desired state*: the services and jobs you've asked for. A reconcile loop runs every few seconds, comparing what should exist against what actually does. When something's missing, it asks the scheduler where it should go. The scheduler is a pure function over the current nodes and their used resources — it picks a live node with enough CPU and memory that matches your placement labels. The agent picks up the assignment and runs it, reporting status back. Then the loop re-checks and converges again.

- **Controller** — the API, the desired-state store, and the reconcile loop
- **Scheduler** — deterministic CPU / memory / label-aware placement
- **Agents** — one per node: register, heartbeat, poll, execute
- **Runtime** — a swappable executor behind an interface (a fake today, a real container runtime next)



## Quick start

Requires Go 1.22+.

```sh
make build
```

Three terminals, three moving parts:

```sh
# terminal 1 — the control plane
./bin/controller

# terminal 2 — a node
./bin/agent -node-id pi-0 -cpu 1000 -memory 2048 -labels tier=general

# terminal 3 — deploy something
./bin/k8ctl service create -name web -image nginx -replicas 2 -cpu 100 -memory 256
./bin/k8ctl job create -name backup -image alpine -command "backup now" -retries 1
```

Within a reconcile tick or two, `k8ctl state` shows the cluster converging:

```
NODES
ID    LABELS        CPU   MEMORY  HEARTBEAT
pi-0  tier=general  1000  2048    0s
pi-1  tier=batch    500   1024    0s

SERVICES
NAME  IMAGE  REPLICAS  RUNNING  PENDING  ASSIGNMENTS
web   nginx  2         2        0        web-1-1,web-1-2

JOBS
NAME    IMAGE   ATTEMPTS  STATUS
backup  alpine  0         succeeded

ASSIGNMENTS
ID          OWNER        NODE  PHASE      IMAGE   CPU  MEMORY
web-1-1     service/web  pi-0  Running    nginx   100  256
web-1-2     service/web  pi-0  Running    nginx   100  256
backup-1-3  job/backup   pi-0  Succeeded  alpine  100  128
```

Try it: drop `web`'s replicas to 1, or kill an agent, and watch the controller pull the cluster back to what you asked for.

## Commands


| Command                                                                        | What it does                               |
| ------------------------------------------------------------------------------ | ------------------------------------------ |
| `k8ctl service create -name web -image nginx -replicas 2 -cpu 100 -memory 256` | Declare a service and its desired replicas |
| `k8ctl service delete web`                                                     | Remove the service and its assignments     |
| `k8ctl job create -name backup -image alpine -command "backup now" -retries 1` | Run a one-shot job                         |
| `k8ctl job delete backup`                                                      | Remove a job and its assignments           |
| `k8ctl get nodes|services|jobs|assignments [--json]`                           | Look at one part of the cluster            |
| `k8ctl state [--json]`                                                         | Full cluster snapshot                      |


A job command containing the word `fail` fails on purpose, so you can watch retries and the failed state settle:

```sh
./bin/k8ctl job create -name flaky -image alpine -command "fail once" -retries 1
```

Point any tool at a different controller with `-server http://host:8080`. To run it on your own machines for real, `make install` drops `k8-controller`, `k8-agent`, and `k8ctl` into `~/.local/bin`, and `deploy/` has systemd units for the controller and per-node agents.

## Honest limits

It is not production Kubernetes, and it's not pretending to be. State lives in memory (restart the controller and you start fresh), reconciliation is single-threaded, there's no persistence or auth, and the runtime fakes container execution behind a swappable interface. That's the point: it's the smallest version of the real thing that I could build, understand completely, and actually run at home.