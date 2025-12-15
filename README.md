# Infrastructure Management Layer: Local NFV Orchestrator
This local NFVO is responsible for handling the available resources at the given DESIRE6G site, perform local optimization through selecting the proper resources to implement the network service graphs, deploy network functions and configure networking and the data network. The configuration of the data network includes the setup of various infrastructure network functions. 

# Installation
1. Create a kubernetes cluster.
2. Enable/install a DNS addon for kubernetes. (This mostly goes for lightweight clusters like k3s/MicroK8s)
3. Install multus
```sh
kubectl apply -f https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/master/deployments/multus-daemonset.yml
```
4. Install flannel
```sh
kubectl apply -f https://github.com/flannel-io/flannel/releases/latest/download/kube-flannel.yml
```
5. Clone the repository
```sh
git clone -b tomas/feat/srv6 https://github.com/DESIRE6G/IML-LNFVO.git
cd IML-LNFVO
```
6. Install IML
```sh
./install.sh
```

## Important
Make sure to have VRF support enabled on each worker node. Some distributions don't have it enabled by default and leaving this out will prevent IML from working correctly.

```sh
sudo apt install linux-modules-extra-$(uname -r)
sudo modprobe vrf
```

# Workflows
There are two main ways to work with IML
* **SMO mode**: In this workflow, a higher level Service Management and Orchestration (SMO) component handles both the deployment of the applications/microservices and the design of the required network services and hands them over to the IML to be deployed. We recommend using [our fork of Oakestra](https://github.com/tomasagata/oakestra/tree/ns-deployments) along with [a plugin](https://github.com/tomasagata/plugin-kubernetes) that allows using kubernetes clusters as it includes a way to easily define IML's network services directly when creating applications. 
* **Standalone mode**: In this workflow, the user interacts with kubernetes to create all *Applications*, *Network Functions* and *Service Chains*. 

There is no functional difference on IML's side between the two workflow modes, the only difference is that when working with an SMO, this directly automates the process of deploying applications and communicating with the IML, which makes the process of deploying applications easier. However, there might be some functional differences on the SMO's side, for example, when working with Oakestra in SMO mode, it also allows MLOps capabilities such as a centralized model repository to store experiment results.


# Standalone workflow

In this workflow, we'll be following how to set up a basic setup with two applications with a network function in the middle. 

Disclaimer: If you prefer to do a quick deployment, and don't care about the
deployment procedure, you can view the `examples` folder, where three examples are proposed: this simple NF in between two functions, and an advanced scenario where an NF actually has two subfunctions that it can execute. 

### 1. Create the resource definitions

**Applications**

These are the source and destinations for the traffic. All traffic must flow from one application to another. Currently, these must be defined first in IML, and then deployed using the CNI. Here is an example extracted from the `examples/simple/definitions.yaml` file.

```yaml
apiVersion: cache.desire6g.eu/v1alpha1
kind: Application
metadata:
  name: web-client
  namespace: default
spec: 
  override_id: dead-beef
---
apiVersion: cache.desire6g.eu/v1alpha1
kind: Application
metadata:
  name: web-server
  namespace: default
spec: 
  override_id: face-feed
```

**Network Functions**
These are middleboxes that take in packets in real time and perform some operation with them. In this scenario, we'll be using a simple packet logger NF that essentially prints "Function executed" every time a packet from the App-App flow is successfully identified. Network Functions, unlike applications, are automatically deployed.

```yaml
apiVersion: cache.desire6g.eu/v1alpha1
kind: NetworkFunction
metadata:
  name: pkt-logger
  namespace: default
spec:
  type: simple
  replicas: 1
  containers:
    - name: pkt-logger
      image: tomasagata/pkt-logger:latest
```

**Service Chains**
They describe how the traffic should flow between the applications. In this case, we want the traffic from A to B to flow through the packet logger network function. **These traffic flows are NOT bidirectional**. Meaning, you can define a packet function when traffic flows from A to B and another network function when traffic flows from B to A. If you want to make it bidirectional, just create a service chain in the opposite direction.

```yaml
apiVersion: cache.desire6g.eu/v1alpha1
kind: ServiceChain
metadata:
  name: client-server-with-logger
  namespace: default
spec:
  from:
    name: web-client
    namespace: default
  to:
    name: web-server
    namespace: default
  functions:
  - name: pkt-logger
    namespace: default
```

### 2. Deploy the applications using IML's CNI.
Once the resources are registered, deploy the applications with the CNI metadata. Here is an extract of what you should be adding to the kubernetes deployment.
The only field that needs modification on each deployment is the 'app_id' property. This must be changed to match either kubernetes' UID of the resource or the registered `override_id` property.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-client
  namespace: default
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
          [
            { 
              "name": "iml-cni",
              "cni-args": {
                "app_type": "application_function",
                "app_id": "dead-beef"
              }
            }
          ]
    spec:
      nodeSelector:
        kubernetes.io/hostname: oai-01
      containers:
        - name: curl
          image: alpine:latest
          command: ['sh', '-c', 'echo "Hello, Kubernetes!" && sleep 3600']
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-server
  namespace: default
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
          [
            {
              "name": "iml-cni", 
              "cni-args": {
                "app_type": "application_function",
                "app_id": "face-feed"
              }
            }
          ]
    spec:
      nodeSelector:
        kubernetes.io/hostname: oai-01
      containers:
        - name: nginx
          image: alpine:latest
          ports:
            - containerPort: 80
```

### 4. Verify the network service is working
A good process to test everything is deployed correctly first starts by listing all pods
```sh
kubectl get pods -A
```
Here you should be able to see the deployed application pods and the packet logger network function.

After that, then I recommend you see the logs of the packet logger. Here you'll be able to see when packets are identified.
```sh
kubectl logs -f <Packet logger's pod name>
```

The final test consists of pinging a container from its peer
```sh
kubectl exec -it <application's pod name> -- /bin/sh
```
```sh
ping <peer's IP>
```

After doing that, you'll be able to see some output on the packet logger showcasing the packets that were identified.

### 5. Remove the network service
The network service can be removed by executing
```sh
kubectl delete servicechains <SERVICE_CHAINS>
kubectl delete applications <APP_NAME>
kubectl delete networkfunctions <NF_NAME>
```

# Uninstall

```sh
./uninstall.sh
```