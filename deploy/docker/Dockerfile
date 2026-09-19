# One simulated homelab node: containerd + runc + the k8 binaries.
# Nodes run privileged with their own cgroup namespace, like k3d/kind nodes,
# so workloads are real containers with real cgroup limits.

FROM golang:1.26 AS build
ENV GOTOOLCHAIN=auto CGO_ENABLED=0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN go build -trimpath -o /out/k8-controller ./cmd/controller \
 && go build -trimpath -o /out/k8-agent ./cmd/agent \
 && go build -trimpath -o /out/k8ctl ./cmd/k8ctl

FROM debian:bookworm-slim
ARG TARGETARCH
ARG CONTAINERD_VERSION=2.4.0
ARG RUNC_VERSION=1.5.1

# python3 is only there so Ansible modules can run on the node during drills.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl iproute2 procps python3 tini \
 && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-linux-${TARGETARCH}.tar.gz" \
      | tar -xz -C /usr/local \
 && curl -fsSL -o /usr/local/sbin/runc "https://github.com/opencontainers/runc/releases/download/v${RUNC_VERSION}/runc.${TARGETARCH}" \
 && chmod 0755 /usr/local/sbin/runc

COPY --from=build /out/ /usr/local/bin/
COPY deploy/docker/entrypoint.sh /usr/local/bin/entrypoint.sh

VOLUME ["/var/lib/containerd"]
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]
