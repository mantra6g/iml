# P4Target

## Examples
P4Target of architecture `v1model` with 16 CPUs and 64Gi of memory, with a condition indicating 
that it is ready to accept network functions
```yaml
apiVersion: core.loom.io/v1alpha1
kind: P4Target
metadata:
  name: example-target
  labels:
    p4target.loom.io/arch: v1model
spec: {}
status:
  capacity:
    cpu: "16"
    memory: "64Gi"
  allocatable:
    cpu: "14"
    memory: "60Gi"
  conditions:
    - type: Ready
      status: "True"
      reason: "P4TargetReady"
      message: "The P4 target is ready to accept network functions."
```

P4Target of architecture `v1model` with 16 CPUs and 64Gi of memory, with a condition indicating
that it is not ready to accept network functions because of failed health probes, and with 
a taint indicating that it is not ready
```yaml
apiVersion: core.loom.io/v1alpha1
kind: P4Target
metadata:
  name: example-target
  labels:
    p4target.loom.io/arch: v1model
spec: 
  taints:
    - key: p4target.loom.io/not-ready
      effect: NoSchedule
status:
  capacity:
    cpu: "16"
    memory: "64Gi"
  allocatable:
    cpu: "14"
    memory: "60Gi"
  conditions:
    - type: Ready
      status: "False"
      reason: "ProbeFailed"
      message: "The P4 target is not ready to accept network functions due to failed health probes."
```

P4Target of architecture `v1model` with 16 CPUs and 64Gi of memory, with a condition indicating
that it is ready to accept network functions, but with a taint and a condition indicating that 
it is unschedulable for new network functions because of an administrator marking it as unschedulable
```yaml
apiVersion: core.loom.io/v1alpha1
kind: P4Target
metadata:
  name: example-target
  labels:
    p4target.loom.io/arch: v1model
spec: 
  unschedulable: true
status:
  capacity:
    cpu: "16"
    memory: "64Gi"
  allocatable:
    cpu: "14"
    memory: "60Gi"
  conditions:
    - type: Ready
      status: "True"
      reason: "P4TargetReady"
      message: "The P4 target is ready to accept network functions"
    - type: Unschedulable
      status: "True"
      reason: "MarkedUnschedulable"
      message: "P4 target is marked as unschedulable"
```

## Spec fields
* `taints`: An optional list of taints applied to the P4 target, which can affect scheduling and operation of 
  network functions on it. Each taint consists of a key, an optional value, an effect 
  (e.g., `NoSchedule`, `PreferNoSchedule`, `NoExecute`), and an optional time when the taint was added. 
  Taints can be used to indicate conditions such as the target being not ready, unreachable, or unschedulable.
* `unschedulable`: An optional boolean field indicating whether the P4 target is unschedulable for 
  new network functions. If set to true, it indicates that new network functions should not be scheduled 
  on this target, but it does not affect already running network functions. This can be used by administrators 
  to mark a target as unschedulable for maintenance or other reasons.
* `targetIP`: The IP address assigned to the P4 target, which can be used for communication and management purposes.
  This field is typically updated by the driver when the target is registered and becomes available.
* `driverIPs`: A list of IP addresses belonging to the driver managing this programmable target. These IPs can be used
  for retrieving data-plane object information from external pods. These IPs are assigned by the *primary CNI*, when
  the driver pod is being initialized, and then they are updated to this P4Target's spec by the driver itself. This is
  a list exclusively for dual-stack configuration purposes.
* `nfCIDR`: Block of addresses dedicated to NFs running on this P4Target. This field is assigned and set by IML's 
  operator.

## Status fields
* `capacity`: Represents the total resources of the P4 target, such as CPU and memory
* `allocatable`: Represents the resources of the P4 target that are available for allocation to network functions, 
  which may be less than the total capacity due to reserved resources or other factors.
* `conditions`: Represents the latest available observations of the P4 target's current state. 
  Each condition has a type (e.g., `Ready`, `Unschedulable`), a status (True, False, Unknown), and 
  optional fields for the last heartbeat time, last transition time, reason, and message. Conditions can be 
  used to indicate the readiness and health status of the target, as well as whether it is unschedulable for 
  new network functions.
