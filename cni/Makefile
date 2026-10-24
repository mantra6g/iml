# Image URL to use all building/pushing image targets
IMG ?= cni:local

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

#.PHONY: test
#test: manifests generate fmt vet setup-envtest ## Run tests.
#	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

##@ eBPF

# BPF_INCLUDE_ARCH points clang at the multiarch kernel headers (asm/types.h, ...).
BPF_INCLUDE_ARCH ?= /usr/include/$(shell uname -m)-linux-gnu
export BPF_INCLUDE_ARCH

# BPF2GO_STRIP falls back to a versioned llvm-strip (e.g. llvm-strip-19) when the unversioned one is missing.
BPF2GO_STRIP ?= $(shell command -v llvm-strip || ls /usr/bin/llvm-strip-* 2>/dev/null | sort -V | tail -n1)
export BPF2GO_STRIP

.PHONY: ebpf-generate
ebpf-generate: ## Compile the eBPF programs and regenerate their Go bindings with bpf2go.
	go generate ./ebpf/...

.PHONY: ebpf-verify
ebpf-verify: ## Load the eBPF programs through the kernel verifier and run their packet tests (needs sudo).
	go test -exec sudo -count=1 -v ./ebpf/...

.PHONY: ebpf-check
ebpf-check: ebpf-generate ## Fail if the committed eBPF objects and bindings are out of date.
	git diff --exit-code -- ebpf/
	@test -z "$$(git ls-files --others --exclude-standard -- ebpf/)" || \
		{ echo "untracked generated eBPF files:"; git ls-files --others --exclude-standard -- ebpf/; exit 1; }

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)
