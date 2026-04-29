# Applications

## Example

```yaml
apiVersion: core.loom.io/v1alpha1
kind: Application
metadata:
  name: application-sample
spec: {}
status:
  subnets:
    ubuntu-1: ["10.0.0.0/24", "10.0.1.0/24"]
    ubuntu-2: ["10.0.2.0/24"]
```

## Spec fields
This resource does not have any spec fields. 
It is used as a marker for workloads that are part of the service chaining topology.

In order to mark a pod as part of an application, users must specify the following annotation on the pod template:
```yaml
metadata:
  annotations:
    k8s.v1.cni.cncf.io/networks: |
      [{
        "name": "iml-cni",
        "cni-args": {
          "app_name":      "<application-name>",
          "app_namespace": "<application-namespace>"
        }
      }]
```

## Status fields

* `subnets`: Map of nodes to the list of subnets that have been allocated for applications running on that node. 
  This information is used by the CNI plugin to determine which IP addresses should be assigned to workloads with the corresponding application label.
