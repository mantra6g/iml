# API Overview

This section documents IML's Kubernetes APIs (Custom Resource Definitions) used to model applications, network functions, service chains, and programmable targets.

## API groups

IML resources are organized into three main API groups:

- `core.loom.io/v1alpha1`: service graph and programmable function resources.
- `scheduling.loom.io/v1alpha1`: rollout and replica management for network functions.
- `infra.loom.io/v1alpha1`: infrastructure target and node abstractions.

## Resource map

### Core (`core.loom.io/v1alpha1`)

- [`Application`](applications.md): declares traffic endpoints.
- [`NetworkFunction`](network-functions.md): defines packet-processing functions and target selection.
- [`ServiceChain`](service-chains.md): declares directional traffic paths through one or more NFs.
- [`P4Target`](p4targets.md): represents a schedulable programmable data-plane target.

### Scheduling (`scheduling.loom.io/v1alpha1`)

- [`NetworkFunctionDeployment`](network-function-deployment.md): declarative rollouts and update strategies.
- [`NetworkFunctionReplicaSet`](network-function-replicaset.md): stable set of replicated `NetworkFunction` instances.

### Infrastructure (`infra.loom.io/v1alpha1`)

- [`BMv2Target`](bmv2target.md): BMv2-based programmable target abstraction.
- [`LoomNode`](loom-node.md): node-level CIDR allocation model used by infrastructure components.
- [`NetworkFunctionConfig` (WIP)](network-function-config.md): evolving configuration API for NFs.

## How resources relate

1. Create `Application` resources for source and destination workloads.
2. Create `NetworkFunction` (or `NetworkFunctionDeployment`) resources.
3. Define a `ServiceChain` from source app to destination app through selected NFs.
4. Ensure compatible `P4Target`/`BMv2Target` resources exist for scheduling and execution.

## Version and compatibility notes

- Current docs describe the `v1alpha1` API surface.
- Alpha APIs may evolve; check per-resource pages for field-level details.
- Prefer label-based selection (`targetSelector`, service-chain function selectors) to reduce coupling to object names.


