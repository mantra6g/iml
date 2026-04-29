# Deploying the Service Chain
Once you have your programmable target, network functions, and applications set up, you can deploy a service chain 
to connect them together. A service chain defines the sequence of network functions that traffic will pass through
between applications.

## Creating the Service Chain
In this walkthrough example, we said that we wanted to steer traffic from a web client to a web server through a 
firewall network function. To do this, we will create a `ServiceChain` resource that specifies the source and 
destination applications, as well as the network function that should be applied to the traffic.

```yaml
apiVersion: core.loom.io/v1alpha1
kind: ServiceChain
metadata:
  name: web-client-to-web-server
spec:
  from:
    name: web-client
    namespace: default
  to:
    name: web-server
    namespace: default
  functions:
  - matchLabels:
      nf: firewall
```

This manifest creates a service chain named `web-client-to-web-server` that steers traffic 
from the `web-client` application to the `web-server` application through a network functions that match
the label `nf=firewall`.

Once you apply this manifest, IML will automatically configure the necessary networking and routing 
rules to ensure that traffic from the web client is steered through the firewall network function 
before reaching the web server.

## Verifying the Service Chain
To verify that the service chain is working as expected, you can generate traffic from the web client
to the web server and observe the behavior of the firewall network function. For example, you can
use `curl` from the web client to send requests to the web server and check if the firewall is correctly
allowing or blocking traffic based on its configuration.
```bash
kubectl exec -it <web-client-pod> -- curl http://<web-server-service-ip>
```

However, as we haven't configured the firewall network function with any specific rules, 
it will block all traffic by default. In order to allow traffic through the firewall, we will need to configure it 
with the appropriate rules. In order to do this, we'll need to create table entries for the firewall network function, 
which we will cover in the next section of this walkthrough.
