# Loom Node

## Examples
LoomNode with assigned CIDR blocks.
```yaml
apiVersion: infra.loom.io/v1alpha1
kind: LoomNode
metadata:
  name: example-loom-node
spec:
  nodeCIDRs: ["10.123.0.0/24", "fd00:1::/32"]
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
* `nodeCIDRs`: A list of CIDR blocks used for pod IPs on this node. This field is recommended to be left blank. 
  When left empty, the controller will automatically allocate a CIDR block for this node from the cluster's CIDR range set with the --cluster-cidr argument when starting the controller.

## Status fields
This resource does not currently have any status fields defined, but they can be added in the future 
to provide more information about the observed state of the LoomNode.
