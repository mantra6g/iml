# Installing IML

## Prerequisites

Before installing IML, you need:

1. **A Kubernetes cluster** (v1.11.3 or higher)
     - You can use [kind](https://kind.sigs.k8s.io/) to run a local cluster for development or testing
     - See [Setting Up a Kubernetes Cluster](#setting-up-a-kubernetes-cluster) below

2. **Required cluster add-ons:**
     - **Multus CNI** - for multiple network interface support
     - **Flannel** - for pod-to-pod networking
     - **cert-manager** - for certificate management and webhook validation

3. **Tools installed locally:**
     - `docker` (for building/running containers)
     - `kubectl` (v1.11.3 or higher)
     - `kind` (if you don't already have a cluster — see below)
     - `helm` (v3.0 or higher)
     - `docker` (for building/running containers)

4. Enable vrf kernel module.

```bash
sudo modprobe vrf
```

## Setting Up a Kubernetes Cluster

### Option 1: Using kind (Recommended for Development)

[kind](https://kind.sigs.k8s.io/) (Kubernetes in Docker) is the easiest way to run a local cluster for development and testing.

1. **Create a kind cluster:**
   ```bash
   kind create cluster --name iml
   ```

2. **Verify the cluster is running:**
   ```bash
   kubectl cluster-info --context kind-iml
   kubectl get nodes
   ```

### Option 2: Using an Existing Cluster

If you already have a Kubernetes cluster running (local or remote), you can use that instead. Make sure you have proper access via `kubectl`:

```bash
kubectl cluster-info
kubectl auth can-i create deployments --all-namespaces
```

## Installing Cluster Dependencies

After you have a cluster running, install the required add-ons:

### 1. Install Multus CNI

Multus enables attaching multiple network interfaces to pods:

```bash
kubectl apply -f https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/master/deployments/multus-daemonset.yml
```

### 2. Install Flannel

Flannel provides pod-to-pod networking:

```bash
kubectl apply -f https://raw.githubusercontent.com/flannel-io/flannel/v0.28.4/Documentation/kube-flannel.yml

# Wait for Flannel to be ready
kubectl wait --for=condition=Ready pod -l app=flannel -n kube-flannel --timeout=300s
```

### 3. Install cert-manager

cert-manager handles certificate provisioning and webhook validation for IML:

```bash
# Add the Jetstack Helm repository
helm repo add jetstack https://charts.jetstack.io
helm repo update

# Install cert-manager with CRDs
helm install cert-manager jetstack/cert-manager --namespace cert-manager --create-namespace --set installCRDs=true

# Wait for cert-manager to be ready
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/instance=cert-manager -n cert-manager --timeout=300s
```

### 4. Build and Load Local Images (kind/dev clusters)

The `operator`, `daemon`, and `cni` images are not published to a public registry, so for local/dev clusters (e.g. kind) you must build and load them yourself before installing IML — with either installation method below:

```bash
make docker-build-iml
make kind-load-iml
```

If you plan to run network functions on a local programmable target (e.g. the bundled `examples/`, which use BMv2), also build and load the target images:

```bash
make docker-build-targets
make kind-load-targets
```


## Installing IML

Now that your cluster has all the required dependencies, you can install IML. There are two methods:

1. **Installing via Helm** (recommended for production)
2. **Installing via kubectl manifests** (recommended for development)

### Installing via Helm

First, clone the IML repository and navigate to the `chart` directory:

```bash
git clone https://github.com/mantra6g/iml
cd iml/chart
```

Then, install the IML Helm chart:

```bash
helm install iml . --namespace loom-system --create-namespace
```

### Installing via kubectl manifests

*This installation method is recommended for development*. First, clone the IML repository and `cd` into it:

```bash
git clone https://github.com/mantra6g/iml
cd iml
```

Then, apply the kubectl manifests:

```bash
kubectl apply -f operator/dist/install.yaml
kubectl apply -f daemon/dist/install.yaml
```

Verify that the components are running:

```bash
kubectl get pods -n loom-system
```


## Uninstalling IML

### Uninstalling via Helm

To uninstall IML when installed via Helm, first `cd` into the IML repository if you haven't already:

```bash
cd iml
```

Run the following command:

```bash
helm uninstall iml --namespace loom-system
```

### Uninstalling via kubectl manifests

To uninstall IML when installed via kubectl manifests, first `cd` into the IML repository if you haven't already:

```bash
cd iml
```

Then, delete the kubectl manifests:

```bash
kubectl delete -f daemon/dist/install.yaml
kubectl delete -f operator/dist/install.yaml
```

## Cleaning Up the Cluster

### Removing kind Cluster (Development Only)

If you used kind for local development and want to remove it:

```bash
kind delete cluster --name iml
```

### Removing Cluster Dependencies (Optional)

If you no longer need the cluster dependencies, you can remove them:

```bash
# Remove Multus
kubectl delete -f https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/master/deployments/multus-daemonset.yml

# Remove Flannel (use the same version tag you installed)
kubectl delete -f https://raw.githubusercontent.com/flannel-io/flannel/v0.28.4/Documentation/kube-flannel.yml

# Remove cert-manager
helm uninstall cert-manager --namespace cert-manager
kubectl delete namespace cert-manager
```

**Note:** Only remove cluster dependencies if you're not using them for other applications.

