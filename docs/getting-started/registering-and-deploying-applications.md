# Registering and Deploying Applications
Applications are the core of any Kubernetes cluster. In IML, an `Application` represents a group 
of container replicas that generate or consume traffic. In order to allow your applications to be included
in service chains and be subject to network function processing, you first need to register an `Application` 
resource for each of your workloads. 

In the following scenario, we will register two applications: a web client,
which will be the source of the traffic and a web server, which will be the destination. 
In the following sections we will be creating a simple service chain that steers the traffic from the web client to 
the web server through a firewall network function.

## Registering Applications
The first step is to create `Application` resources for both the web client and the web server. 
```yaml
apiVersion: core.loom.io/v1alpha1
kind: Application
metadata:
  name: web-client
spec: {}
---
apiVersion: core.loom.io/v1alpha1
kind: Application
metadata:
  name: web-server
spec: {}
```

## Deploying Workloads
Once the `Application` resources are created, you'll need to deploy the actual workloads that will 
generate or consume traffic for these applications. However, in order to have the workloads be associated
with the correct `Application` resources, you must add the following annotation to the pod template of each workload:
```yaml
metadata:
  annotations:
    k8s.v1.cni.cncf.io/networks: |
      [{
        "name": "iml-cni",
        "cni-args": {
          "appName":      "<application-name>",
          "appNamespace": "<application-namespace>"
        }
      }]
```

For now, we will be using `curl` for the web client and `nginx` for the web server, but you can use any 
containerized application that generates or consumes traffic. Here are the deployment manifests for both applications:

### Web Client
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-client
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web-client
  template:
    metadata:
      labels:
        app: web-client
      annotations:
        k8s.v1.cni.cncf.io/networks: |
          [{
            "name": "iml-cni",
            "cni-args": {
              "appName":      "web-client",
              "appNamespace": "default"
            }
          }]
    spec:
      containers:
      - name: web-client
        image: curlimages/curl:latest
        command: ["sleep", "infinity"]
```

### Web Server
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-server
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web-server
  template:
    metadata:
      labels:
        app: web-server
      annotations:
        k8s.v1.cni.cncf.io/networks: |
          [{
            "name": "iml-cni",
            "cni-args": {
              "appName":      "web-server",
              "appNamespace": "default"
            }
          }]
    spec:
      containers:
      - name: web-server
        image: nginx:latest
        ports:
        - containerPort: 80
```

## Verifying Application Deployment
Once the applications have been deployed, you can verify that they are running and that the pods 
can communicate with each other by running the following commands:
```bash
# Get the pods for both applications
kubectl get pods -l app=web-client
kubectl get pods -l app=web-server
```

```bash
# Get the IP address of the web server pod from multus' network status annotation
WEB_SERVER_POD=$(kubectl get pods -l app=web-server -o jsonpath='{.items[0].metadata.name}')
WEB_SERVER_IP=$(kubectl get pod $WEB_SERVER_POD -o jsonpath='{.metadata.annotations.k8s\.v1\.cni\.cncf\.io/network-status}' | jq -r '.[0].ips[0]')
```

```bash
# Exec into the web client pod and try to curl the web server
WEB_CLIENT_POD=$(kubectl get pods -l app=web-client -o jsonpath='{.items[0].metadata.name}')
kubectl exec -it $WEB_CLIENT_POD -- curl http://$WEB_SERVER_IP
```

If everything is set up correctly, you should see the default nginx welcome page in the output of the curl command, 
indicating that the web client can successfully communicate with the web server.

The next step before creating a service chain is to create a P4 Target where we can deploy our network functions. 
Continue to the next section to learn how to create a bmv2-based p4 programmable target. More information about
creating and configuring Applications can be found in the [API reference documentation](../api/applications.md).
