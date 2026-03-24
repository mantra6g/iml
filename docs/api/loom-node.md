# Loom Node

## Examples
LoomNode with assigned CIDR blocks for pods, SIDs, P4 targets, and tunnels.
```yaml
apiVersion: infra.loom.io/v1alpha1
kind: LoomNode
metadata:
  name: example-loom-node
spec:
  podCIDRs: ["10.0.0.0/24"]
  sidCIDRs: ["10.0.1.0/24"]
  p4TargetCIDRs: ["10.0.2.0/24"]
  tunnelCIDRs: ["10.0.3.0/24"]
status: {}
```

LoomNode with missing CIDR blocks, allowing the controller to automatically allocate them.
```yaml
apiVersion: infra.loom.io/v1alpha1
kind: LoomNode
metadata:
  name: example-loom-node
spec: {}
status: {}
```

## Spec fields
* `podCIDRs`: A list of CIDR blocks used for pod IPs on this node. This field is optional. 
  If left empty, the controller will automatically allocate a CIDR block for this node from the cluster's CIDR range set with the --cluster-cidr argument when starting the controller.
* `sidCIDRs`: A list of CIDR blocks used for segment identifiers (SID) on this node. This field is optional. 
  If left empty, the controller will automatically allocate a CIDR block for this node from the cluster's CIDR range set with the --cluster-cidr argument when starting the controller.
* `p4TargetCIDRs`: A list of CIDR blocks used for P4 targets on this node. This field is optional. 
  If left empty, the controller will automatically allocate a CIDR block for this node from the cluster's CIDR range set with the --cluster-cidr argument when starting the controller.
* `tunnelCIDRs`: A list of CIDR blocks used for tunnels on this node. This field is optional. 
  If left empty, the controller will automatically allocate a CIDR block for this node from the cluster's CIDR range set with the --cluster-cidr argument when starting the controller.

## Status fields
This resource does not currently have any status fields defined, but they can be added in the future 
to provide more information about the observed state of the LoomNode.
