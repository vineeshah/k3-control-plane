GO ?= go
BIN := bin
PREFIX ?= $(HOME)/.local
ANSIBLE := cd deploy/ansible && ansible-playbook

.PHONY: build install test vet fmt clean cluster-up cluster-down drills drill-%

build:
	mkdir -p $(BIN)
	$(GO) build -o $(BIN)/controller ./cmd/controller
	$(GO) build -o $(BIN)/agent ./cmd/agent
	$(GO) build -o $(BIN)/k8ctl ./cmd/k8ctl

install: build
	install -d $(PREFIX)/bin
	install -m 0755 $(BIN)/controller $(PREFIX)/bin/k8-controller
	install -m 0755 $(BIN)/agent $(PREFIX)/bin/k8-agent
	install -m 0755 $(BIN)/k8ctl $(PREFIX)/bin/k8ctl

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

clean:
	rm -rf $(BIN)

# 3-node cluster in Docker: node-0 (server + agent), node-1, node-2.
cluster-up:
	$(ANSIBLE) playbooks/cluster-up.yml

cluster-down:
	$(ANSIBLE) playbooks/cluster-down.yml

# Every disaster drill, in order, against a running cluster.
drills:
	$(ANSIBLE) playbooks/drills.yml

# One drill: make drill-01, make drill-04, ...
drill-%:
	$(ANSIBLE) $(patsubst deploy/ansible/%,%,$(wildcard deploy/ansible/playbooks/$*-*.yml))
