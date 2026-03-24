# Create your first programmable target
Programmable targets are a core component of IML. They allow running and executing P4 programs on a
variety of hardware and software targets. In this section, we will deploy a simple programmable switch based
on the Behavioral Model v2 (BMv2) software switch. This target will then be used in the following sections 
to run a simple firewall network function.

## Deploying a BMv2 Target
To deploy a BMv2 target, you can use the following manifest
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
This manifest creates a BMv2 target with the name `example-bmv2-target` and specifies resource requests
and limits for CPU and memory. You can adjust these values based on the expected workload and available
resources in your cluster. However for this walkthrough example, this should be enough to get you started.

## Verifying the BMv2 Target
Once you apply the manifest, you can verify that the BMv2 target has been created and is running 
by using the following command:
```bash
kubectl get bmv2targets
```

This should show you a list of BMv2 targets in your cluster, including the one you just created. 
You can also check the status of the target to ensure that it is ready and running
```bash
kubectl get bmv2target example-bmv2-target -o yaml
```

If you see that the target's "Ready" condition is "True", then the target is up and running and you can proceed
to the next steps of configuring it with a P4 program and including it in a service chain.

For more information about BMv2Targets, please refer to the [BMv2Target API documentation](../api/bmv2target.md).
