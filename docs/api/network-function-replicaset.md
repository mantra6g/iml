# Network Function ReplicaSet

## Examples
NetworkFunctionReplicaSet of a network function with 3 replicas, matching P4 targets 
with architecture `v1model`, and with a control plane pod defined
```yaml
apiVersion: scheduling.loom.io/v1alpha1
kind: NetworkFunctionReplicaSet
metadata:
  name: example-nf-replicaset
spec:
  replicas: 3
  selector:
    matchLabels:
      app: example-nf
  template:
    metadata:
      labels:
        app: example-nf
    spec:
      p4File: https://example.org/p4program.p4
      targetSelector:
        p4target.loom.io/arch: v1model
      controlPlane:
        image: example-control-plane-image:latest
        resources:
          requests:
            cpu: "500m"
            memory: "256Mi"
          limits:
            cpu: "1"
            memory: "512Mi"
```

## Spec fields
* `replicas`: The number of desired replicas of the network function. Defaults to 1 if not specified.
* `selector`: A label query over network function instances that should match the replica count. 
  It must match the labels of the NetworkFunctionTemplate.
* `template`: The template describing the NetworkFunction that will be created. 
  It includes the metadata and specs of the network function, such as the P4 file, 
  control plane configuration, and target selector.
* `minReadySeconds`: The minimum number of seconds for which a newly created NetworkFunction should be ready 
  without any of its container crashing, for it to be considered available. Defaults to 0 (the NetworkFunction
  will be considered available as soon as it is ready).

## Status fields
* `replicas`: The total number of non-terminated replicas that are currently running and ready.
* `fullyLabeledReplicas`: The number of replicas that are fully labeled and ready.
* `readyReplicas`: The number of ready NetworkFunction replicas.
* `availableReplicas`: The number of available NetworkFunction replicas.
* `observedGeneration`: The most recent generation observed for this NetworkFunctionReplicaSet. 
  It corresponds to the generation of the most recently observed NetworkFunctionReplicaSet's desired state.
* `conditions`: Represents the latest available observations of the NetworkFunctionReplicaSet's current state. 
  It can include conditions such as `ReplicaFailure`, which indicates that one or more replicas failed to be 
  created or deleted due to issues such as insufficient quota, limit ranges, target selectors, or driver issues.
