# Image URL to use all building/pushing image targets
IMG_OPERATOR ?= operator:local
IMG_DAEMON ?= daemon:local
IMG_CNI ?= cni:local
IMAGES = $(IMG_OPERATOR) $(IMG_DAEMON) $(IMG_CNI)

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
.PHONY: docker-build
docker-build: ## Build docker images for the cni, daemon and operator.
	$(MAKE) -C cni docker-build IMG=${IMG_CNI}
	$(MAKE) -C daemon docker-build IMG=${IMG_DAEMON} IMG_CNI=${IMG_CNI}
	$(MAKE) -C operator docker-build IMG=${IMG_OPERATOR}

.PHONY: docker-push
docker-push: ## Push docker image with the cni, daemon and operator.
	$(MAKE) -C cni docker-push IMG=${IMG_CNI}
	$(MAKE) -C daemon docker-push IMG=${IMG_DAEMON} IMG_CNI=${IMG_CNI}
	$(MAKE) -C operator docker-push IMG=${IMG_OPERATOR}

.PHONY: kind-create
kind-create: ## Create and configure a local kind cluster.
	$(KIND) create cluster --name $(KIND_CLUSTER)
	$(KUBECTL) wait --for=condition=Ready pod -n kube-system --all --timeout=120s
	$(KUBECTL) apply -f https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/master/deployments/multus-daemonset.yml
	$(KUBECTL) wait --for=condition=Ready pod -l app=multus -n kube-system --timeout=120s
	$(KUBECTL) apply -f https://github.com/flannel-io/flannel/releases/latest/download/kube-flannel.yml
	$(KUBECTL) wait --for=condition=Ready pod -l app=flannel -n kube-flannel --timeout=300s
	$(KUBECTL) apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.20.0/cert-manager.yaml
	$(KUBECTL) wait --for=condition=Ready pod -l app.kubernetes.io/instance=cert-manager -n cert-manager --timeout=300s

.PHONY: kind-delete
kind-delete: ## Delete the local kind cluster.
	$(KIND) delete cluster --name $(KIND_CLUSTER)

.PHONY: kind-load
kind-load: docker-build ## Load the cni, daemon and operator images into the local cluster.
	$(KIND) load docker-image ${IMG_CNI} ${IMG_DAEMON} ${IMG_OPERATOR} --name $(KIND_CLUSTER)

.PHONY: build-installer
build-installer: ## Generate a consolidated YAML with CRDs and deployment.
	$(MAKE) -C operator build-installer IMG=${IMG_OPERATOR}
	$(MAKE) -C daemon build-installer IMG=${IMG_DAEMON} IMG_CNI=${IMG_CNI}
	$(KUSTOMIZE) . > install.yaml

.PHONY: install
install: build-installer ## Deploy the BMv2 test pod.
	$(KUBECTL) apply -f install.yaml

.PHONY: uninstall
uninstall: build-installer ## Remove the BMv2 test pod.
	$(KUBECTL) delete -f install.yaml --ignore-not-found

## Tool Binaries
KUSTOMIZE ?= kubectl kustomize
KUBECTL ?= kubectl
KIND ?= kind
MAKE ?= make
