GO ?= go
BIN := bin
PREFIX ?= $(HOME)/.local

.PHONY: build install test vet fmt clean

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
