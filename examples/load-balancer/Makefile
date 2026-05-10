# Image URL to use all building/pushing image targets
IMG_LB_CONTROLLER ?= tomasagata/lb-controller:latest
IMG_WEBSERVER ?= tomasagata/lb-server:latest
ALL_IMAGES = $(IMG_LB_CONTROLLER) $(IMG_WEBSERVER)

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
KIND_CLUSTER ?= iml

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: help

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

##@ Build

# If you wish to build the driver image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build-all
docker-build-all: docker-build-controller docker-build-webserver ## Build all docker images.

.PHONY: docker-build-controller
docker-build-controller: ## Build docker images for the load-balancer controller.
	$(CONTAINER_TOOL) build -t ${IMG_LB_CONTROLLER} -f ./lb-control/Dockerfile ./lb-control

.PHONY: docker-build-webserver
docker-build-webserver: ## Build docker image for the cni.
	$(CONTAINER_TOOL) build -t ${IMG_WEBSERVER} -f webserver.Dockerfile .

.PHONY: docker-push-all
docker-push-all: ## Push all docker images.
	$(CONTAINER_TOOL) push ${ALL_IMAGES}

.PHONY: kind-load-all
kind-load-all: kind-load-controller kind-load-webserver ## Load all images into the local cluster.

.PHONY: kind-load-controller
kind-load-controller: ## Load all images into the local cluster.
	$(KIND) load docker-image ${IMG_LB_CONTROLLER} --name $(KIND_CLUSTER)

.PHONY: kind-load-webserver
kind-load-webserver: ## Load all target images into the local cluster.
	$(KIND) load docker-image ${IMG_WEBSERVER} --name $(KIND_CLUSTER)

## Tool Binaries
KUSTOMIZE ?= kubectl kustomize
KUBECTL ?= kubectl
KIND ?= kind
MAKE ?= make
