# Image URL to use all building/pushing image targets
IMG_LB_CONTROLLER ?= mantra6g/lb-controller-pl2:latest
IMG_WEB_SERVER ?= mantra6g/web-server-pl2:latest
IMG_PROXY ?= mantra6g/proxy-pl2:latest
IMG_STREAMING_SERVER ?= mantra6g/streaming-server-pl2:latest

ALL_IMAGES = $(IMG_LB_CONTROLLER) $(IMG_WEB_SERVER) $(IMG_STREAMING_SERVER) $(IMG_WEB_PROXY) $(IMG_STREAMING_PROXY)

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
docker-build-all: docker-build-lbcontroller docker-build-servers docker-build-proxy ## Build all docker images.

.PHONY: docker-build-lbcontroller
docker-build-lbcontroller: ## Build docker images for the load-balancer controller.
	$(CONTAINER_TOOL) build -t ${IMG_LB_CONTROLLER} -f ./load-balancer/Dockerfile ./load-balancer

.PHONY: docker-build-servers
docker-build-servers: docker-build-webserver docker-build-streamingserver## Build docker images for the web and streaming servers.

.PHONY: docker-build-webserver
docker-build-webserver: ## Build docker image for the cni.
	$(CONTAINER_TOOL) build -t ${IMG_WEB_SERVER} -f ./web-server/Dockerfile ./web-server

.PHONY: docker-build-streamingserver
docker-build-streamingserver: ## Build docker image for the cni.
	$(CONTAINER_TOOL) build -t ${IMG_STREAMING_SERVER} -f ./streaming-server/Dockerfile ./streaming-server

.PHONY: docker-build-proxy
docker-build-proxy: ## Build docker image for the proxy.
	$(CONTAINER_TOOL) build -t ${IMG_PROXY} -f ./proxy/Dockerfile ./proxy

.PHONY: docker-push-all
docker-push-all: ## Push all docker images.
	$(CONTAINER_TOOL) push ${ALL_IMAGES}

.PHONY: kind-load-all
kind-load-all: kind-load-lbcontroller kind-load-servers kind-load-proxy ## Load all images into the local cluster.

.PHONY: kind-load-lbcontroller
kind-load-lbcontroller: ## Load the load-balancer controller image into the local cluster.
	$(KIND) load docker-image ${IMG_LB_CONTROLLER} --name $(KIND_CLUSTER)

.PHONY: kind-load-servers
kind-load-servers: kind-load-webserver kind-load-streamingserver ## Load all server images into the local cluster.

.PHONY: kind-load-webserver
kind-load-webserver: ## Load the web server image into the local cluster.
	$(KIND) load docker-image ${IMG_WEB_SERVER} --name $(KIND_CLUSTER)

.PHONY: kind-load-streamingserver
kind-load-streamingserver: ## Load the streaming server image into the local cluster.
	$(KIND) load docker-image ${IMG_STREAMING_SERVER} --name $(KIND_CLUSTER)

.PHONY: kind-load-proxy
kind-load-proxy: ## Load the proxy image into the local cluster.
	$(KIND) load docker-image ${IMG_PROXY} --name $(KIND_CLUSTER)

## Tool Binaries
KUSTOMIZE ?= kubectl kustomize
KUBECTL ?= kubectl
KIND ?= kind
MAKE ?= make
