# mBot2 Agent Lab. Run `make help` for an overview.

SHELL := /bin/bash
GO ?= go
PYTHON ?= python3
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

# Development secrets for deploy/local/lab.yaml. Never use these on the VM.
DEV_ENV := LAB_ADMIN_PASSWORD=dev MBOT1_PASSWORD=dev MBOT2_PASSWORD=dev \
	TEAM_ROT_TOKEN=rot-dev TEAM_BLAU_TOKEN=blau-dev MISTRAL_API_KEY=$${MISTRAL_API_KEY:-unset}

CONFIG ?=
ROBOTS ?= 2
WORLD ?= sim/worlds/arena.json

.PHONY: help dev sim test test-go test-py bundle build build-linux deploy clean

help: ## Show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

dev: ## Run the server locally (HTTP :8080, MQTT :1883)
	cd server && $(DEV_ENV) $(GO) run ./cmd/labserver -config ../deploy/local/lab.yaml

sim: ## Start simulated robots against the local server (ROBOTS=2 WORLD=...)
	$(PYTHON) sim/run.py --robots $(ROBOTS) --world $(WORLD) --trace 2

test: test-go test-py ## Run all tests

test-go: ## Go tests
	cd server && $(GO) vet ./... && $(GO) test ./...

test-py: ## Robot script and simulator tests
	$(PYTHON) -m unittest discover -s sim/tests -t sim/tests

bundle: ## Bundle robot/src into robot/dist/main.py (CONFIG=robot/config.py inlines secrets)
	$(PYTHON) robot/tools/bundle.py $(if $(CONFIG),--config $(CONFIG))

build: ## Build the server for this machine into bin/
	cd server && $(GO) build -ldflags "$(LDFLAGS)" -o ../bin/labserver ./cmd/labserver

build-linux: ## Build the server for the Azure VM (linux/amd64)
	cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o ../bin/labserver-linux-amd64 ./cmd/labserver

deploy: build-linux ## Copy the binary to the VM and restart (HOST=user@vm)
	@test -n "$(HOST)" || (echo "usage: make deploy HOST=user@vm" && exit 1)
	scp bin/labserver-linux-amd64 $(HOST):/tmp/labserver
	ssh $(HOST) 'sudo install -m 0755 /tmp/labserver /usr/local/bin/labserver && sudo systemctl restart labserver'

clean: ## Remove build output
	rm -rf bin robot/dist sim/.build server/lab-dev.db server/lab-dev.db-shm server/lab-dev.db-wal
