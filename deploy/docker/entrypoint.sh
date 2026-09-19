#!/usr/bin/env bash
# Boots one node: containerd, the agent and, on the server node, the
# controller too (a k3s server also runs an agent). Each process is supervised
# like systemd's Restart=always so drills can kill it and watch it come back.
#
# Drills can keep a process down on purpose: while /run/k8/hold/<name> exists
# its supervisor waits instead of restarting it.
set -euo pipefail

: "${K8_NODE_ID:?K8_NODE_ID is required}"
ROLE=${K8_ROLE:-agent}
SERVER=${K8_SERVER:-https://node-0:6443}
HOLD_DIR=/run/k8/hold
mkdir -p "$HOLD_DIR" /var/log/k8

# cgroup v2: a cgroup with processes in it can't delegate controllers to
# children, so move ourselves into /init and enable every controller for the
# containers runc will create. Same trick kind and k3d use.
if [[ -f /sys/fs/cgroup/cgroup.controllers ]]; then
  mkdir -p /sys/fs/cgroup/init
  while read -r pid; do
    echo "$pid" > /sys/fs/cgroup/init/cgroup.procs 2>/dev/null || true
  done < /sys/fs/cgroup/cgroup.procs
  sed -e 's/ / +/g' -e 's/^/+/' < /sys/fs/cgroup/cgroup.controllers \
    > /sys/fs/cgroup/cgroup.subtree_control
fi

supervise() {
  local name=$1
  shift
  while true; do
    while [[ -e "$HOLD_DIR/$name" ]]; do sleep 1; done
    echo "[supervise] starting $name"
    "$@" 2>&1 | sed -u "s/^/[$name] /" || true
    echo "[supervise] $name exited, restarting in 2s"
    sleep 2
  done
}

supervise containerd containerd &
until [[ -S /run/containerd/containerd.sock ]]; do sleep 0.2; done

if [[ "$ROLE" == "server" ]]; then
  supervise controller k8-controller \
    -data-dir /var/lib/k8 \
    -backup-dir /mnt/nas/k8 \
    -backup-interval "${K8_BACKUP_INTERVAL:-30s}" \
    -node-timeout "${K8_NODE_TIMEOUT:-10s}" \
    -token "${K8_JOIN_SECRET:?server needs K8_JOIN_SECRET}" \
    -tls-san "$K8_NODE_ID" &

  # Stand-in for the operator copying the token to each node.
  until [[ -s /var/lib/k8/token ]]; do sleep 0.5; done
  cp /var/lib/k8/token /shared/token
fi

until [[ -s /shared/token ]]; do sleep 0.5; done

supervise agent k8-agent \
  -node-id "$K8_NODE_ID" \
  -server "$SERVER" \
  -token "$(cat /shared/token)" \
  -cpu "${K8_CPU:-4000}" \
  -memory "${K8_MEMORY:-2048}" \
  -labels "${K8_LABELS:-}" \
  -runtime containerd &

wait
