# BMv2 Target

## Examples
BMv2 Target with 250m CPU and 128Mi memory requests, and 500m CPU and 256Mi memory limits.
```yaml
apiVersion: infra.loom.io/v1alpha1
kind: BMv2Target
metadata:
  name: example-bmv2-target
spec: 
  resources:
    limits:
      cpu: "500m"
      memory: "256Mi"
    requests:
      cpu: "250m"
      memory: "128Mi"
```

## Spec fields
* `resources`: Resource requirements for the BMv2 target. 
  This field is optional and can be used to specify the CPU and memory requests and limits for the BMv2 target.
    - `limits`: The maximum amount of resources that the BMv2 target can use. This field is optional.
      - `cpu`: The maximum amount of CPU that the BMv2 target can use. This field is optional and should 
      be specified in millicores (e.g., "500m" for 0.5 CPU).
      - `memory`: The maximum amount of memory that the BMv2 target can use. This field is optional and should
      be specified in bytes (e.g., "256Mi" for 256 mebibytes).
    - `requests`: The minimum amount of resources that the BMv2 target requires. This field is optional.
      - `cpu`: The minimum amount of CPU that the BMv2 target requires. This field is optional and should
      be specified in millicores (e.g., "250m" for 0.25 CPU).
      - `memory`: The minimum amount of memory that the BMv2 target requires. This field is optional and should
      be specified in bytes (e.g., "128Mi" for 128 mebibytes).

## Status fields
* `observedGeneration`: The most recent generation observed by the controller. 
  This field is optional and is used to track the generation of the resource that has been processed 
  by the controller. It is updated by the controller when it processes a new generation of the resource.
* `conditions`: A list of conditions that describe the current state of the BMv2 target. 
  This field is optional and can be used to provide more detailed information about the status of 
  the BMv2 target. Each condition includes a type, status, last transition time, reason, and message.
    - `type`: The type of condition (e.g., "Ready").
    - `status`: The status of the condition (e.g., "True", "False", "Unknown").
    - `lastTransitionTime`: The last time the condition transitioned from one status to another.
    - `reason`: A brief reason for the condition's last transition.
    - `message`: A human-readable message indicating details about the transition.
