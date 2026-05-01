# Service Chains

## Example

```yaml
apiVersion: core.loom.io/v1alpha1
kind: ServiceChain
metadata:
  name: servicechain-sample
spec: 
  from:
    name: application-1
    namespace: default
  to:
    name: application-2
    namespace: default
  functions:
  - matchLabels:
      nf: firewall
  - matchLabels:
      nf: load-balancer
status: {}
```

## Spec fields
* `from`: Source application for the service chain. Traffic originating from this application will be processed by the defined chain.
    - `name`: Name of the source application.
    - `namespace`: Namespace of the source application.
* `to`: Destination application for the service chain. Traffic destined for this application will be processed by the defined chain.
    - `name`: Name of the destination application.
    - `namespace`: Namespace of the destination application.
* `functions`: Ordered list of network functions to apply to traffic matching the `from` -> `to` criteria. 
  Each entry is a label selector that identifies a set of NFs to execute for the service chain. 
  For example, `nf: firewall` would select all NFs with the label `nf=firewall`.

## Status fields

This resource does not have any status fields at this time.
