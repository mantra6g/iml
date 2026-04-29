# Network Function

## Examples
Network function matching P4 targets with architecture `v1model`:
```yaml
apiVersion: core.loom.io/v1alpha1
kind: NetworkFunction
metadata:
  name: example-nf
spec:
  p4File: https://example.org/p4program.p4
  targetSelector:
    p4target.loom.io/arch: v1model
```

Creating a network function referencing a NetworkFunctionConfig named `example-nf-config`:
```yaml
apiVersion: core.loom.io/v1alpha1
kind: NetworkFunction
metadata:
  name: example-nf
spec:
  p4File: https://example.org/p4program.p4
  configRef:
    name: example-nf-config
```

Scheduling a network function on a specific P4 target named `example-target`:
```yaml
apiVersion: core.loom.io/v1alpha1
kind: NetworkFunction
metadata:
  name: example-nf
spec:
  p4File: https://example.org/p4program.p4
  targetName: example-target
```

Network function matching P4 targets with architecture `v1model` and with control plane pod:
```yaml
apiVersion: core.loom.io/v1alpha1
kind: NetworkFunction
metadata:
  name: example-nf
spec:
  p4File: https://example.org/p4program.p4
  targetSelector:
    p4target.loom.io/arch: v1model
  controlPlane:
    image: example-control-plane-image:latest
    imagePullPolicy: IfNotPresent
    resources:
      requests:
        cpu: "500m"
        memory: "256Mi"
      limits:
        cpu: "1"
        memory: "512Mi"
```

## Spec fields
* `p4File`: The actual P4 program file for the network function. It can be the actual p4program encoded in base64
  or a s3://, http:// or https:// URL pointing to the P4 file location.
* `targetName`: An optional field that can be used to specify the name of the P4Target where this NetworkFunction 
  instance should be scheduled. If not specified, the scheduler will automatically select a suitable P4Target based 
  on the TargetSelector.
* `controlPlane`: Optional field to define the template for the control plane pod of the network function.
    * `image` is the container image for the control plane pod of the network function.
    * `imagePullPolicy` defines the image pull policy for the control plane pod. The default value is `IfNotPresent`.
    * `resources` defines the resource requests and limits for the control plane pod.
    * `nodeName` specifies the name of the node where the control plane pod should be scheduled. 
    If specified, the scheduler will attempt to schedule the pod on the specified node.
    * `nodeSelector` defines the node selector for the control plane pod.
    * `tolerations` defines the tolerations for the control plane pod.
    * `affinity` defines the affinity rules for the control plane pod.
    * `extraEnv` defines extra environment variables for the control plane pod. By default, the control plane pod 
    will have the `NF_NAME` and `NF_NAMESPACE` environment variables set to the name and namespace of the 
    NetworkFunction instance, respectively. This field can be used to add additional environment variables as needed.
    * `args` defines the command-line arguments for the control plane pod.
* `targetSelector`: Used to select P4 targets based on their supported architectures, or other labels. 
  The scheduler will use this selector to find suitable P4 targets for scheduling the NetworkFunction instance.
  A built-in label that can be used in the target selector is `p4target.loom.io/arch` for selecting P4 targets 
  based on their architecture (e.g., `v1model`, `psa`, etc.).
* `configRef`: An optional reference to a NetworkFunctionConfig resource that contains the configurations for this
  network function. If specified, the driver will automatically apply the configuration from the referenced 
  NetworkFunctionConfig to this NetworkFunction instance.

## Status fields
* `observedGeneration`: The most recent generation observed for this NetworkFunction. 
  This is used to determine if the status is up-to-date with the spec.
* `phase`: Indicates the current phase of the NetworkFunction. 
  Possible values include `Pending`, `Running`, and `Failed`.
* `conditions`: Represents the latest available observations of the NetworkFunction's current state. 
  Each condition has a type (e.g., `Initialized`, `Ready`, `Scheduled`, `DisruptionTarget`, `ReadyToStart`), a status 
  (True, False, Unknown), and optional fields for the last probe time, last transition time, reason, and message.
* `assignedIP` is the IP address assigned to the NetworkFunction, used for routing traffic to it.

