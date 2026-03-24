# Network Function Deployment

## Examples
NetworkFunctionDeployment of a network function with 3 replicas, using a rolling update strategy 
with maxUnavailable and maxSurge set to 25%, matching P4 targets with architecture `v1model`, and 
with a control plane pod defined:
```yaml
apiVersion: scheduling.loom.io/v1alpha1
kind: NetworkFunctionDeployment
metadata:
  name: example-nf-deployment
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: "25%"
      maxSurge: "25%"
  selector:
    matchLabels:
      app: example-nf
  template:
    metadata:
      labels:
        app: example-nf
    spec:
      p4File: https://example.org/p4program.p4
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
      targetSelector:
        p4target.loom.io/arch: v1model
```

## Spec fields
* `replicas`: The number of desired replicas of the network function. Defaults to 1 if not specified.
* `strategy`: The deployment strategy for the network function. 
  - `type`: It can be either `RollingUpdate` or `Recreate`. 
    If not specified, it defaults to `RollingUpdate`. 
  - `rollingUpdate`: Can be used to specify the parameters for the rolling update strategy, 
    such as `maxUnavailable` and `maxSurge`, which define the maximum number of replicas that can 
    be unavailable or created above the desired number of replicas during the update process, respectively. 
    Both fields can be specified as an absolute number (e.g., 1) or as a percentage (e.g., "25%"). If not specified, 
    they default to "25%".
* `selector`: A label query over network function instances that should match the replica count. It must 
  match the labels of the NetworkFunctionTemplate.
* `template`: The template describing the NetworkFunctionDeployment that will be created. 
  It includes the metadata and specs of the network function, such as the P4 file, 
  control plane configuration, and target selector.
* `minReadySeconds`: The minimum number of seconds for which a newly created NetworkFunction should be ready 
  without any of its container crashing, for it to be considered available. Defaults to 0 (the NetworkFunction
  will be considered available as soon as it is ready).

## Status fields
* `observedGeneration`: The most recent generation observed for this NetworkFunctionDeployment. It corresponds
  to the generation of the NetworkFunctionDeploymentSpec that was last processed by the controller.
* `replicas`: The total number of replicas observed by the controller.
* `updatedReplicas`: The total number of replicas that have been updated to match the desired state.
* `readyReplicas`: The number of ready replicas of the network function.
* `availableReplicas`: The number of replicas that are ready and stable for at least minReadySeconds. 
  A replica is considered available when its ready condition is true, and it has been ready for at least 
  minReadySeconds. Defaults to 0 (the replica will be considered available as soon as it is ready).
* `unavailableReplicas`: The number of unavailable replicas of the network function.
* `collisionCount`: The count of hash collisions for the NetworkFunctionDeployment. The number is 
  incremented by the controller when it detects a hash collision between NetworkFunctionReplicaSets with 
  different spec templates.
* `conditions`: Represents the latest available observations of the NetworkFunctionDeployment's current state. 
  Each condition has a type (e.g., `Available`, `Progressing`), a status (True, False, Unknown), and optional 
  fields for the last update time, last transition time, reason, and message.
