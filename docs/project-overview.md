# Project Overview

IML (Infrastructure Management Layer) is a local Network Function Virtualization (NFV) orchestrator for Kubernetes environments. It manages service-level connectivity between applications by deploying and chaining packet-processing Network Functions (NFs).

## What IML does

IML provides a control plane to:

- Register **Applications** as traffic endpoints.
- Register and deploy **Network Functions** (middleboxes) such as packet processors.
- Define **Service Chains** that describe packet flow from one application to another through one or more NFs.
- Configure the data path so traffic follows the requested chain.

## Core concepts

### Applications

Applications are source and destination workloads in the cluster. In IML, service chaining starts and ends at application resources.

### Network Functions

Network Functions inspect, transform, or route packets in-line. They are Kubernetes-backed workloads orchestrated by IML.

### Service Chains

A service chain defines a **directional** traffic path (`from` -> `to`) and the ordered list of NFs to execute for that direction.

## Main project components

The repository includes multiple runtime components that work together:

- `operator/`: Kubernetes Operator and CRDs for IML custom resources.
- `cni/`: CNI plugin integration used to attach workloads to IML-managed networking.
- `go-daemon/`: Main daemon services and internal control logic.
- `iml-oakestra-agent/`: Integration agent for Oakestra-based workflows.
- `docs/`: Project documentation (guides, architecture, and API reference).

## Supported workflows

IML supports two common operation modes:

- **Standalone mode**: users define IML resources directly in Kubernetes.
- **SMO-integrated mode**: a higher-level orchestrator (for example, Oakestra) automates application + service definition and delegates deployment to IML.

## Typical deployment flow

1. Install IML on a Kubernetes cluster.
2. Create `Application` resources.
3. Create `NetworkFunction` resources.
4. Create `ServiceChain` resources that bind applications through NFs.
5. Deploy workloads with IML CNI metadata.
6. Validate packet flow and NF behavior.

For a hands-on walkthrough, continue with [Getting Started](getting-started/installation.md).
