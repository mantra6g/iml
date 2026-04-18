# Image URL to use all building/pushing image targets
IMG ?= bmv2-driver:local

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# KIND_CLUSTER defines the test cluster used to develop locally
KIND_CLUSTER ?= bmv2-dev

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

##@ Build

.PHONY: build
build: fmt vet ## Build driver binary.
	go build -o bin/driver cmd/main.go

.PHONY: run
run: fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

.PHONY: test
test: fmt vet ## Run tests.
	go test -v ./...

.PHONY: clean
clean: ## Clean build artifacts and binaries.
	rm -f bin/driver
	go clean

# If you wish to build the driver image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the driver.
	$(CONTAINER_TOOL) build -t ${IMG} --platform=linux/amd64 .

.PHONY: kind-create
kind-create: ## Create a local kind cluster.
	$(KIND) create cluster --name $(KIND_CLUSTER)

.PHONY: kind-delete
kind-delete: ## Delete the local kind cluster.
	$(KIND) delete cluster --name $(KIND_CLUSTER)

.PHONY: kind-load
kind-load: ## Load the driver image into a kind cluster.
	$(KIND) load docker-image ${IMG} --name $(KIND_CLUSTER)

.PHONY: test-up
test-up: ## Deploy the BMv2 test pod.
	$(KUBECTL) apply -f test.yaml

.PHONY: test-down
test-down: ## Remove the BMv2 test pod.
	$(KUBECTL) delete -f test.yaml --ignore-not-found

.PHONY: test-logs
test-logs: ## Show logs from the BMv2 test pod.
	$(KUBECTL) logs bmv2-test -c bmv2-driver -f

.PHONY: test-logs-switch
test-logs-switch: ## Show logs from the BMv2 switch container.
	$(KUBECTL) logs bmv2-test -c bmv2-switch -f

.PHONY: test-exec
test-exec: ## Execute shell in the BMv2 driver container.
	$(KUBECTL) exec -it bmv2-test -c bmv2-driver -- /bin/sh

.PHONY: test-exec-switch
test-exec-switch: ## Execute shell in the BMv2 switch container.
	$(KUBECTL) exec -it bmv2-test -c bmv2-switch -- /bin/sh

.PHONY: port-forward
port-forward: ## Forward local port 8080 to the test pod.
	$(KUBECTL) port-forward pod/bmv2-test 8080:8080

.PHONY: docker-push
docker-push: ## Push docker image with the driver.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-load
docker-load: docker-build kind-load ## Build docker image and load into kind cluster.

.PHONY: docker-clean
docker-clean: ## Remove docker image.
	$(CONTAINER_TOOL) rmi ${IMG} || true

# PLATFORMS defines the target platforms for the driver image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the driver for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name bmv2-driver-builder
	$(CONTAINER_TOOL) buildx use bmv2-driver-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm bmv2-driver-builder
	rm Dockerfile.cross

##@ P4 Programs

P4_PROGRAMS_DIR := p4-programs
P4_COMPILE_SCRIPT := $(P4_PROGRAMS_DIR)/compile.sh

.PHONY: p4-compile
p4-compile: ## Compile P4 programs for BMv2.
	@if [ ! -f "$(P4_COMPILE_SCRIPT)" ]; then \
		echo "Error: compile.sh not found in $(P4_PROGRAMS_DIR)"; \
		exit 1; \
	fi
	@for p4_file in $(P4_PROGRAMS_DIR)/*.p4; do \
		if [ -f "$$p4_file" ]; then \
			echo "Compiling $$p4_file..."; \
			cd $(P4_PROGRAMS_DIR) && bash compile.sh $$(basename $$p4_file) || exit 1; \
			cd ...; \
		fi \
	done

.PHONY: p4-clean
p4-clean: ## Clean compiled P4 programs.
	rm -rf $(P4_PROGRAMS_DIR)/compiled
	find $(P4_PROGRAMS_DIR) -name "*.p4info.txt" -delete
	find $(P4_PROGRAMS_DIR) -name "*.json" -delete

.PHONY: p4-all
p4-all: p4-clean p4-compile ## Clean and recompile all P4 programs.

##@ Deployment

.PHONY: deploy
deploy: docker-build kind-load test-up ## Build, load image, and deploy test pod.

.PHONY: undeploy
undeploy: test-down ## Remove test pod.

.PHONY: redeploy
redeploy: undeploy deploy ## Redeploy test pod.

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
GOLANGCI_LINT_VERSION ?= v2.1.6

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef

##@ API Testing

API_HOST ?= localhost:8080

.PHONY: api-health
api-health: ## Test the health endpoint.
	@echo "Testing /api/health endpoint..."
	@curl -s http://$(API_HOST)/api/health | jq . || echo "Error: Could not reach health endpoint"

.PHONY: api-tables
api-tables: ## Retrieve table entries from the switch.
	@echo "Testing /api/tables endpoint..."
	@curl -s http://$(API_HOST)/api/tables | jq . || echo "Error: Could not reach tables endpoint"

.PHONY: api-counters
api-counters: ## Retrieve counter data from the switch.
	@echo "Testing /api/counters endpoint..."
	@curl -s http://$(API_HOST)/api/counters | jq . || echo "Error: Could not reach counters endpoint"

.PHONY: api-get-program
api-get-program: ## Retrieve current P4 program information.
	@echo "Testing /api/p4/program (GET) endpoint..."
	@curl -s http://$(API_HOST)/api/p4/program | jq . || echo "Error: Could not reach program endpoint"

.PHONY: api-verify-program
api-verify-program: ## Verify P4 program without deploying (dry-run).
	@echo "Testing /api/p4/verify endpoint..."
	@curl -s -X POST http://$(API_HOST)/api/p4/verify \
		-H "Content-Type: application/json" \
		-d '{"program": "", "dry_run": true}' | jq . || echo "Error: Could not reach verify endpoint"

.PHONY: api-all-tests
api-all-tests: api-health api-tables api-counters api-get-program ## Run all API endpoint tests.
