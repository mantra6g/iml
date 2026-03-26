# Contributing to IML (Infrastructure Management Layer)

## Project Contribution Policy

The IML project is currently limited to contributors we already know. If you are interested in contributing to this project, please reach out to **tomas@tomasagata.dev** with a detailed proposal of what you'd like to contribute. We welcome ideas and contributions from the community!

---

## Project Structure

The IML project consists of three main components, each residing in its own directory at the root level of the repository:

### 1. **CNI** (`/cni`)

**Purpose:** Container Network Interface (CNI) Plugin for Kubernetes

The CNI component is a Kubernetes network plugin that handles networking configuration for both network functions and application containers. It is responsible for:

- **Network Configuration**: Dynamically assigns IP addresses and network interfaces to containers when they are deployed
- **Container Integration**: Communicates with the IML daemon to fetch proper network configurations based on container type (network function or application function)
- **Network Setup**: Configures bridges, virtual interfaces, and routing rules for containers in the cluster
- **Service Chain Support**: Integrates containers into IML-managed service chains for network function virtualization

The CNI plugin is invoked by Kubernetes whenever a pod is created or destroyed, ensuring proper network connectivity for all workloads.

**Key Technologies:** Go, CNI specification, Netlink for network configuration

### 2. **Go-Daemon** (`/go-daemon`)

**Purpose:** Local NFV Management Daemon

The Go-Daemon is the core orchestration component that runs on each node in the cluster. It handles:

- **Application Management**: Manages the lifecycle of applications deployed on the local node
- **Virtual Network Function (VNF) Management**: Controls VNFs and their connectivity
- **Data Plane Configuration**: Manages the software data plane for traffic steering through service chains
- **IP and Routing Management**: Assigns and manages subnets for different types of workloads (applications, network functions, tunnels)
- **Event Bus**: Maintains an in-memory event system for communication between services
- **Route Calculation**: Computes optimal paths for traffic through the network
- **MQTT Integration**: Communicates with the central orchestration layer via MQTT for coordination
- **Database/Registry**: Maintains a registry of deployed services and their configurations

The daemon acts as the local intelligence layer that executes orchestration decisions made by the central operator component.

**Key Technologies:** Go, gRPC/REST APIs, MQTT, in-memory database

### 3. **Operator** (`/operator`)

**Purpose:** Kubernetes Operator and Central Orchestration Controller

The Operator is a Kubernetes controller that manages the global orchestration and resource scheduling across the cluster. It:

- **Custom Resource Definitions (CRDs)**: Defines and manages custom Kubernetes resources for:
  - **Applications**: User applications deployed in the cluster
  - **Network Functions**: Network services that process traffic
  - **P4 Targets**: Programmable network switch configurations (based on P4 language)
  - **Service Chains**: Graph definitions of how traffic flows through network functions
  - **Network Function Deployments**: Scheduling decisions for network functions
  - **Network Function ReplicaSets**: Horizontal scaling configurations for network functions
- **Scheduling and Orchestration**: Makes global scheduling decisions about where and how network functions should be deployed
- **Resource Management**: Coordinates resources across the cluster
- **Webhook Validation**: Validates custom resources before they are persisted
- **Controller Runtime**: Uses Kubernetes controller-runtime for standard operator patterns

The operator serves as the brain of the system, making high-level orchestration decisions and managing the overall state of network services in the cluster.

**Key Technologies:** Go, Kubernetes controller-runtime, kubebuilder, CRDs, webhooks

---

## Development Workflow

### Creating a Development Branch

To contribute changes, follow this branching strategy:

1. **Create a feature branch** from the main development branch:
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Use descriptive branch names** that reflect the component and feature:
   ```bash
   # Examples:
   git checkout -b feature/cni-vrf-support
   git checkout -b fix/daemon-mqtt-reconnect
   git checkout -b feature/operator-scheduling-improvement
   ```

3. **Make your changes** and commit regularly with clear commit messages

4. **Push your branch** and create a Pull Request for review:
   ```bash
   git push origin feature/your-feature-name
   ```

---

## Building Containers Locally

### Prerequisites

Before building containers, ensure you have:

- **Go 1.24.0+** installed
- **Docker** installed and running
- **kubectl** installed (v1.11.3+)
- **kind** installed for local Kubernetes cluster testing
- **Make** installed

### Building Individual Components

#### 1. Building the CNI Plugin

```bash
cd cni

# For local testing with kind (recommended for development):
docker build -t iml-cni:local .

# For pushing to a registry (requires a registry server running at localhost:5000):
make docker-buildx IMG=localhost:5000/iml-cni:latest
```

#### 2. Building the Go-Daemon

```bash
cd go-daemon

# For local testing with kind (recommended for development):
docker build -t iml-daemon:local .

# For pushing to a registry (requires a registry server running at localhost:5000):
make docker-buildx IMG=localhost:5000/iml-daemon:latest
```

#### 3. Building the Operator

```bash
cd operator

# For local testing with kind (recommended for development):
docker build -t loom-operator:local .

# For pushing to a registry (requires a registry server running at localhost:5000):
make docker-build docker-push IMG=localhost:5000/loom-operator:latest
```

**Note:** The `localhost:5000` image registry is only needed if you want to push images to a central registry. For local development with kind, the simple `docker build` command is sufficient.

---

## Testing with Kind (Kubernetes in Docker)

Before testing your changes, you'll need to set up a local Kubernetes cluster with all required dependencies. Follow the [Installation Guide](../getting-started/installation.md) to:

1. Create a kind cluster
2. Install Multus CNI, Flannel, and cert-manager
3. Install IML (or a baseline version for comparison)

### Preparing Your Build for Testing

After you've built your container images locally, load them into your kind cluster:

```bash
# Load your built images into kind
kind load docker-image iml-cni:local --name iml-dev
kind load docker-image iml-daemon:local --name iml-dev
kind load docker-image loom-operator:local --name iml-dev
```


### Testing Your Changes

1. **Create test resources**:
   ```bash
   # Create sample applications, network functions, or service chains
   # Examples are provided in the examples/ directory
   kubectl apply -f examples/simple/
   ```

2. **Monitor deployments**:
   ```bash
   # Check operator logs
   kubectl logs -f deployment/loom-operator -n loom-system
   
   # Check daemon logs
   kubectl logs -f daemonset/iml-daemon -n loom-system
   
   # Check CNI plugin logs
   kubectl logs -f daemonset/iml-cni -n loom-system
   ```

3. **Verify resources are created**:
   ```bash
   # List custom resources
   kubectl get applications
   kubectl get networkfunctions
   kubectl get servicechains
   ```

### Cleaning Up

When done with testing your changes:

```bash
# Remove local images
docker rmi iml-cni:local iml-daemon:local loom-operator:local
```

For full cluster cleanup (removing kind cluster and dependencies), refer to the [Installation Guide Uninstalling section](../getting-started/installation.md#uninstalling-iml).

---

## Development Tips

- **Use local registries**: For faster iteration, configure a local container registry that your kind cluster can access without pulling from Docker Hub
- **Enable debug logging**: Most components support debug logging flags that can help troubleshoot issues
- **Test incrementally**: Deploy one component at a time and verify functionality before adding others
- **Review examples**: The `examples/` directory contains sample configurations for testing different scenarios
- **Monitor events**: Use `kubectl describe` and `kubectl events` to understand what's happening during deployment

---

## Getting Help

If you have questions or run into issues during development:

1. Check the [project documentation](../index.md) in the `docs/` directory
2. Review existing issues and pull requests on the repository
3. Reach out to tomas@tomasagata.dev with your questions or concerns

---

## Code Style and Standards

While not exhaustive, please follow these guidelines when contributing:

- **Go Code**: Follow standard Go formatting with `gofmt` and `goimports`
- **Kubernetes Manifests**: Use proper indentation and follow Kubernetes conventions
- **Documentation**: Keep code comments and documentation up-to-date
- **Testing**: Write tests for new functionality where applicable
- **Commit Messages**: Use clear, descriptive commit messages that explain the "why" behind changes

Thank you for your interest in contributing to IML!

