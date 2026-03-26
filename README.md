# IML - Infrastructure Management Layer

A local Network Function Virtualization (NFV) orchestrator for Kubernetes that enables service chaining and intelligent traffic management through network functions.

## What is IML?

**IML (Infrastructure Management Layer)** is a Kubernetes-native orchestration platform that allows you to:

- **Deploy applications** and register them as traffic endpoints
- **Deploy network functions** (packet processors, middleboxes, etc.) that inspect or transform traffic
- **Create service chains** that define how traffic flows between applications through multiple network functions
- **Manage the data plane** automatically so traffic follows your specified paths

IML makes it easy to implement complex network service graphs while staying fully integrated with Kubernetes.

## Key Features

✨ **Kubernetes-Native** - Uses CRDs and standard Kubernetes patterns  
🔗 **Service Chaining** - Define complex, directional traffic paths through multiple network functions  
🔄 **Flexible Workflows** - Supports both standalone mode and SMO-integrated deployments  
⚡ **Local Optimization** - Intelligent scheduling for optimal resource placement  
🔒 **Secure** - Built-in cert-manager integration for secure communications  
🛠️ **Developer-Friendly** - Full containerization with local testing support via kind  

## Quick Start

### Prerequisites

- Kubernetes cluster v1.11.3+ (or [kind](https://kind.sigs.k8s.io/) for local development)
- `kubectl` and `helm` installed
- Docker installed (for building/testing)

### Installation

For complete installation instructions, including cluster setup and dependency installation, see the [Installation Guide](docs/getting-started/installation.md).

**Quick steps:**

1. Ensure you have a Kubernetes cluster running
2. Install required dependencies (Multus CNI, Flannel, cert-manager)
3. Clone this repository:
   ```bash
   git clone https://github.com/mantra6g/iml
   cd iml
   ```

4. Install IML via Helm:
   ```bash
   cd chart
   helm install iml . --namespace loom-system --create-namespace
   ```

### First Steps

After installation, explore these guides:

- **[Installation Guide](docs/getting-started/installation.md)** - Complete setup instructions
- **[Creating Your First Network Function](docs/getting-started/creating-a-network-function.md)** - Deploy a simple NF
- **[Project Overview](docs/project-overview.md)** - Learn core concepts
- **[Examples](examples/)** - Ready-to-run example deployments

## Workflows

IML supports two main operation modes:

### 1. Standalone Mode

Users define IML resources directly in Kubernetes using CRDs:

```bash
kubectl apply -f applications.yaml
kubectl apply -f network-functions.yaml
kubectl apply -f service-chains.yaml
```

This workflow is ideal for developers and operators who prefer direct Kubernetes management. See [Creating Your First Network Function](docs/getting-started/creating-a-network-function.md) for a detailed walkthrough.

### 2. SMO-Integrated Mode

A higher-level Service Management and Orchestration component (like [Oakestra](https://github.com/tomasagata/oakestra/tree/ns-deployments)) handles application and service chain deployment and delegates to IML.

This workflow is ideal for larger deployments and provides additional capabilities like MLOps (centralized model repositories, experiment tracking, etc.).

**Recommended setup:** [Oakestra with Kubernetes plugin](https://github.com/tomasagata/plugin-kubernetes)

## Important: Kernel Features Required

IML requires specific kernel features to be enabled on each worker node for proper functionality. These are not enabled by default on some Linux distributions.

### VRF Support

Virtual Routing and Forwarding (VRF) allows IML to manage multiple isolated routing tables for traffic segmentation.

**Enable VRF support:**

```bash
# Ubuntu/Debian
sudo apt install linux-modules-extra-$(uname -r)
sudo modprobe vrf
```

**Verify VRF support is enabled:**

```bash
# Check if the VRF kernel module is loaded
lsmod | grep vrf

# You should see output like: vrf    <number>  0
```

### SRv6 Support

Segment Routing IPv6 (SRv6) is used by IML for advanced traffic steering and service function chaining.

**Verify SRv6 support is enabled:**

```bash
# Check if SRv6 is compiled into the kernel
grep SEG6 /boot/config-$(uname -r)

# You should see output like:
# CONFIG_IPV6_SEG6_LWTUNNEL=y
# CONFIG_IPV6_SEG6_HMAC=y
# CONFIG_IPV6_SEG6_BPF=y
```

**If VRF and SRv6 are not enabled, IML will not function correctly.**

## Project Structure

IML consists of three main components:

| Component | Purpose | Location |
|-----------|---------|----------|
| **Operator** | Kubernetes controller for global orchestration and resource scheduling | `operator/` |
| **Go-Daemon** | Local node orchestration and data plane management | `go-daemon/` |
| **CNI Plugin** | Kubernetes network interface configuration | `cni/` |

For detailed explanations, see the [Contributing Guide - Project Structure](docs/contributing/contributing.md#project-structure).

## Documentation

Complete documentation is available in the `docs/` directory:

- **[Project Overview](docs/project-overview.md)** - Core concepts and architecture
- **[Installation Guide](docs/getting-started/installation.md)** - Setup and deployment
- **[Architecture](docs/architecture/overview.md)** - Technical deep dive
- **[API Reference](docs/api/overview.md)** - CRD and module reference
- **[Contributing Guide](docs/contributing/contributing.md)** - Development workflow and guidelines

## Uninstalling

To uninstall IML from your cluster:

```bash
# Via Helm
helm uninstall iml --namespace loom-system

# Or via kubectl manifests
kubectl delete -f operator/dist/install.yaml
kubectl delete -f cni/dist/install.yaml
kubectl delete -f go-daemon/dist/install.yaml
```

For complete cleanup instructions, see the [Installation Guide - Uninstalling](docs/getting-started/installation.md#uninstalling-iml).

## Contributing

Interested in contributing to IML? Check out the [Contributing Guide](docs/contributing/contributing.md) for:

- Project structure overview
- Development workflow
- Building and testing locally with kind
- Code style guidelines

## Project Status

IML is currently limited to contributors we know. If you're interested in contributing, please reach out to **tomas@tomasagata.dev** with a proposal.

## License

See LICENSE file for details.

## Support

For questions, issues, or suggestions:

- 📖 Check the [documentation](docs/index.md)
- 💬 Review existing issues and discussions
- 📧 Contact: tomas@tomasagata.dev

