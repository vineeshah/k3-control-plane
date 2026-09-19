# k3 homelab

A small Kubernetes-style control plane I built from scratch, shaped like k3s and sized for my 3-node Raspberry Pi homelab.

It has a controller with a reconcile loop, a deterministic scheduler, and node agents that join the cluster, heartbeat, and run real containers through containerd. No Kubernetes libraries, no frameworks: just Go, TLS, SQLite and containerd, and the same ideas the real thing is built on.

This repo is deliberately small and personal. There's one controller binary, one agent binary, and one CLI. No CRDs, no webhooks, no etcd. That might seem counter-intuitive, but because it's custom-made and entirely mine, when a specific bug bites I can trace it to a single line and fix it in minutes instead of debugging code I didn't write.

## How it thinks

```
node-0 (server + agent)              node-1 / node-2 (agent)
┌───────────────────────────┐        ┌────────────────────────┐
│ k8-controller :6443       │◄─mTLS──│ k8-agent               │
│   reconcile loop          │        │   └─ containerd (ns k8)│
│   scheduler               │        │        └─ containers   │
│   SQLite  /var/lib/k8     │        └────────────────────────┘
│     └─ snapshots → NAS    │
│ k8-agent + containerd     │
└───────────────────────────┘
```

Every node runs an agent. The agent joins the cluster with a token, heartbeats so the controller knows it's alive, and polls for assignments: nothing more. The agent is deliberately dumb.

The controller holds the *desired state*: the services and jobs you've asked for. A reconcile loop runs every few seconds, comparing what should exist against what actually does. When something's missing, it asks the scheduler where it should go. The scheduler is a pure function over the current nodes and their used resources: it picks a live node with enough CPU and memory, the right placement labels, and none of the service's ports already taken. The agent picks up the assignment, runs it as a container, and reports status back. Then the loop re-checks and converges again.

| Piece | What it does |
| --- | --- |
| **Controller** (`cmd/controller`) | API, reconcile loop, CA and join endpoint |
| **Scheduler** (`internal/scheduler`) | Deterministic CPU / memory / label / host-port placement |
| **Agent** (`cmd/agent`) | Join, heartbeat, poll, converge the local runtime |
| **Runtime** (`internal/runtime`) | containerd executor; a simulated one for running without containerd |
| **Store** (`internal/store`) | SQLite state, like k3s's kine; snapshots to the NAS |
| **PKI** (`internal/pki`) | Cluster CA, k3s-style join token, node and admin certs |

## Design decisions

**Containers belong to containerd, not to the agent.** Stopping or crashing the agent never touches workloads. A restarted agent lists what containerd holds, re-attaches to what's still wanted and reaps the rest. Same for containerd itself: container processes hang off their shims, so a containerd restart doesn't kill them either.

**Dead nodes lose their work for good.** When a node misses heartbeats for `-node-timeout`, its assignments are marked `Lost` and replaced elsewhere. `Lost` is terminal. If the node comes back, the controller no longer hands it that work, so its agent stops the old copies. Without this, a node returning from a partition would run its old replicas alongside the replacements.

**State is one SQLite file on local disk.** It's the k3s single-server trade: no etcd, just a database with WAL and `synchronous=FULL`. It doesn't live on the NAS, because SQLite's locking isn't safe over NFS/SMB. Instead the controller takes a consistent `VACUUM INTO` snapshot to the NAS every few minutes. If `state.db` is ever missing at boot, it restores the newest snapshot. The store fails stop: a database error crashes the controller, and the supervisor restarts it from the last committed state.

**A restart isn't an outage.** After a restart, every stored heartbeat is stale. Nodes known at boot get one `-node-timeout` of grace to check in, so the first reconcile doesn't declare the whole cluster dead. Assignment IDs carry a random suffix (`web-1-x7k2p`), not a counter, so a restore from an older snapshot can't reuse the ID of a container that's still running.

**Joining works like k3s.** On first boot the controller creates a CA and a token of the form `K8<sha256 of CA>::<secret>`. A new agent downloads the CA, accepts it only if it matches the hash in its token, then trades the secret and a CSR for its own client certificate. Everything after that is mutual TLS 1.3. A node's certificate only lets it act as itself: register, heartbeat, and read or report its own assignments. `k8ctl` uses an admin certificate from `/var/lib/k8/admin.conf`, the equivalent of `k3s.yaml`.

## What happens when things break

Each row is a scripted drill in `deploy/ansible/playbooks`. It injects the fault, then checks three things: the controller's view, what containerd is actually running on every node (no orphans, no duplicates), and that every replica answers HTTP.

| Drill | Fault | Expected behaviour |
| --- | --- | --- |
| `01-node-failure` | `docker kill` a node | Replicas marked Lost after node-timeout and rescheduled; returning node reaps its stale containers |
| `02-agent-restart` | Kill the agent process | Replicas keep serving; restarted agent adopts the same containers (same PIDs) |
| `03-controller-restart` | Control plane down 2× node-timeout | Replicas keep serving; state reloads from SQLite; nothing rescheduled |
| `04-network-partition` | Disconnect a node's network | Replicas replaced elsewhere; on heal the isolated copies are stopped, leaving exactly N |
| `05-state-loss` | Delete `state.db` | Controller restores the newest NAS snapshot and converges |
| `06-containerd-restart` | Kill containerd | Containers survive via their shims; nothing recreated |

## Run it

### A 3-node cluster in Docker

The drills run against a local 3-node cluster: node containers that mirror the homelab, each running its own containerd, so workloads are real nested containers with real cgroup limits (the same approach as k3d and kind). On an Apple-silicon Mac, Docker's VM is arm64, the same architecture as the Pis.

Requires Docker, Ansible and the `community.docker` collection (`ansible-galaxy collection install community.docker`).

```sh
make cluster-up      # build the node image, start node-0..2, deploy a test service
make drills          # run every drill in order
make drill-04        # or just one
make cluster-down    # remove the nodes and their volumes
```

Poke at it directly:

```sh
docker exec k8-node-0 k8ctl state
docker exec k8-node-1 ctr -n k8 tasks ls
```

### On real machines

`make install` puts `k8-controller`, `k8-agent` and `k8ctl` in `~/.local/bin` (copy them to `/usr/local/bin` on the nodes). `deploy/` has systemd units:

1. On the server, start `k8-controller.service`. It writes `/var/lib/k8/token` and `/var/lib/k8/admin.conf`.
2. On every node (the server too), install containerd, put the token in `/etc/k8/agent.env` as `K8_TOKEN=...` along with the node's ID, CPU, memory and labels, then start `k8-agent.service`.
3. Mount the NAS at `/mnt/nas` on the server for state snapshots.

The token is only needed for a node's first join; after that the node uses its certificate from `/var/lib/k8-agent`.

## Commands

| Command | What it does |
| --- | --- |
| `k8ctl service create -name web -image nginx -replicas 2 -cpu 100 -memory 256 -ports 80` | Declare a service and its desired replicas |
| `k8ctl service delete web` | Remove the service; agents stop its containers |
| `k8ctl job create -name backup -image alpine -command "tar czf /backup/x.tgz /data" -retries 1` | Run a one-shot job |
| `k8ctl job delete backup` | Remove a job and its assignments |
| `k8ctl get nodes\|services\|jobs\|assignments [--json]` | Look at one part of the cluster |
| `k8ctl state [--json]` | Full cluster snapshot |

`cpu` is in millicores (1000 = one core) and becomes a CFS quota; `memory` is in MB and becomes a hard cgroup limit. A job whose command exits non-zero, e.g. `-command false`, is retried up to `-retries` times.

`k8ctl` reads `/var/lib/k8/admin.conf` by default; use `-config` or `K8_CONFIG` to point it elsewhere.

## Honest limits

It is not production Kubernetes, and it's not pretending to be.

- **One controller.** State survives crashes and disk loss, but while the controller is down nothing gets rescheduled. Real HA needs several servers and consensus (k3s uses embedded etcd for that).
- **No fencing.** During a network partition the isolated node keeps running its replicas while replacements start elsewhere, so for up to one node-timeout plus a poll interval there are more copies than asked for. The cluster converges back to exactly N once the partition heals.
- **Host networking only.** No CNI, no pod IPs, no service discovery or load balancing. A service's ports are node ports, and the scheduler keeps them from clashing.
- **Certificates rotate on restart.** The server cert is reissued each start; node certs are renewed when an agent starts within 30 days of expiry. There's no revocation, and anyone with the token can join under any node name, so the token is the cluster secret.
- **The drills run on one machine.** They exercise the real code paths (containerd, SQLite, TLS, fault handling), but in Docker nodes on one host, not on physical Pis.

That's the point: it's the smallest version of the real thing that I could build and understand completely.
