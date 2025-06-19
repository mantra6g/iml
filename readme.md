# Infrastructure Management Layer: Local NFV Orchestrator
This local NFVO is responsible for handling the available resources at the given DESIRE6G site, perform local optimization through selecting the proper resources to implement the network service graphs, deploy network functions and configure networking and the data network. The configuration of the data network includes the setup of various infrastructure network functions. 

![Local NFV Orchestrator Architecture](https://i.imgur.com/YrMYPd9.png)

The local network service descriptor (localNSD) is created by an SMO and handed to the local NFVO component of IML to initiate the deployment of the subgraph on the specific site. 

# Installation
1. Create a kubernetes cluster with:
    - 1GiB hugepages (**Required**. See steps [here](https://kubernetes.io/docs/tasks/manage-hugepages/scheduling-hugepages/))
2. Enable/Install a DNS addon for kubernetes. (This mostly goes for lightweight clusters like k3s/MicroK8s)
3. Install multus
```bash
kubectl apply -f https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/master/deployments/multus-daemonset.yml
```
4. Install flannel
```bash
kubectl apply -f https://github.com/flannel-io/flannel/releases/latest/download/kube-flannel.yml
```
5. Clone the repository
```bash
git clone -b tomas/docs-1 https://github.com/DESIRE6G/IML-LNFVO.git
cd IML-LNFVO
```
6. Install IML
```bash
kubectl apply -f deployments/iml.yml
```

# Workflows
There are two main ways to work with IML
* **SMO mode**: In this workflow, a higher level Service Management and Orchestration (SMO) component handles both the deployment of the applications/microservices and the design of the required network services and hands them over to the IML to be deployed. We recommend using [our fork of Oakestra](https://github.com/tomasagata/oakestra/tree/ns-deployments) along with [a plugin](https://github.com/tomasagata/plugin-kubernetes) that allows using kubernetes clusters as it includes a way to easily define IML's network services directly when creating applications. 
* **Standalone mode**: In this workflow, the user must directly hand over all network service requests to the IML while also deploying all applications manually using IML's CNI.

There is no functional difference between the two workflow modes, the only difference is that when working with an SMO, this directly automates the process of deploying applications and communicating with the IML, which makes the process of deploying applications easier. 


## Standalone workflow

### 1. Create a local Network Service Descriptor
The Network Service Descriptor (or NSD) tells the IML how the deployed network services should look like. It registers the applications and the data-plane forwarding graphs that the packets should follow. A minimal NSD consists of the following fields
```yaml
lnsd:
  ns:
    # First element is application-functions. This defines the applications 
    # that will be the source and sink of the network packets. It has two required
    # properties: 'af-instance-id' and 'af-id'.
    # * 'af-id': This is a unique identifier for the application. This property is essential as
    #   it allows the IML to keep track of when and where the applications are deployed. When deploying
    #   an application through kubernetes, it needs to be added as the `app_id' parameter in IML's CNI.
    # * 'af-instance-id': This is an alias for the application's id. Its only purpose is to be used as
    #   a shorthand for creating the forwarding graphs, as application IDs tend to be auto-generated and
    #   long or difficult to read.
    application-functions:
      - af-instance-id: "app_A"
        af-id: "curl"
      - af-instance-id: "app_B"
        af-id: "nginx"

    # The second element is forwarding-graphs. This describes the desired network connections.
    # Each forwarding graph uses its own NFRouter to provide connectivity between the applications.
    # It has two required properties: 'source' and 'target'. They simply represent the verteces
    # for end-to-end links. All graphs are non-directional, meaning a graph from app_A to app_B
    # implicitly includes the connection from app_B to app_A; there is no need to create a 
    # separate graph in the reverse direction.
    # The format for the values are <af-instance-id>:<interface-number>.
    forwarding_graphs:
      - source: "app_A:0"
        target: "app_B:0"
```

### 2. Hand over the NSD to the IML
Lookup what the IP of the NFVO is
```bash
kubectl describe pod <IML-NFVO's pod name> -n desire6g-system
```

Use that IP to send the NSD
```bash
curl -F file=@<nsd-name>.yml http://<IML's node IP address>:30050/iml/yaml/deploy
```

### 3. Deploy the applications using IML's CNI.
Once the network service is registered, deploy the applications with the CNI metadata. Here is an extract of what you should be adding to the kubernetes deployment.
The only field that needs modification on each deployment is the 'app_id' property. This must be changed to match the 'af-id' registered in the NSD.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata: 
  #...
spec:
  # ...
  template:
    metadata:
      # ...
      annotations:
        k8s.v1.cni.cncf.io/networks: |
          [{
            "name": "iml-cni",
            "cni-args": {
              "app_id": "nginx",
              "app_type": "application_function"
            }
          }]
    spec:
      # ...
```

### 4. Verify the network service is working
A good process to test everything is deployed correctly first starts by listing all pods
```bash
kubectl get pods -A
```
Here you should be able to see **ONE** NFRouter pod per every forwarding graph defined in the NSD.

After that, then I recommend you see the logs of the NFRouter to make sure nothing has errored out.
Here is an example output
```bash
kubectl logs -n desire6g <nfrouter's pod name>
```
```
EAL: Detected CPU lcores: 8
EAL: Detected NUMA nodes: 1
EAL: Detected static linkage of DPDK
EAL: Multi-process socket /var/run/dpdk/rte/mp_socket
EAL: Selected IOVA mode 'VA'
tap_nl_dump_ext_ack(): Cannot delete qdisc with handle of zero
tap_nl_dump_ext_ack(): Cannot delete qdisc with handle of zero
Neither ACL, LPM, EM, or FIB selected, defaulting to LPM
Initializing port 0 ... Creating queues: nb_rxq=2 nb_txq=2...  Address:2E:36:AD:45:33:3B, Destination:02:20:82:F8:94:4A, Allocated mbuf pool on socket 0
LPM: Adding route 10.100.24.134 / 32 (0) [net_tap0]
LPM: Adding route 10.100.22.67 / 32 (1) [net_tap1]
LPM: Adding route :: / 128 (0) [net_tap0]
txq=0,0,0 txq=1,1,0 
Initializing port 1 ... Creating queues: nb_rxq=2 nb_txq=2...  Address:D2:C2:E7:74:AD:11, Destination:02:6A:D0:8E:7F:5C, txq=0,0,0 txq=1,1,0 

Initializing rx queues on lcore 0 ... rxq=0,0,0 rxq=1,0,0 
L3FWD: entering main loop on lcore 1
L3FWD:  -- lcoreid=1 portid=0 rxqueueid=1
Initializing rx queues on lcore 1 ... rxq=0,1,0 rxq=1,1,0 

L3FWD:  -- lcoreid=1 portid=1 rxqueueid=1
L3FWD: entering main loop on lcore 0
L3FWD:  -- lcoreid=0 portid=0 rxqueueid=0
L3FWD:  -- lcoreid=0 portid=1 rxqueueid=0
```

The final test consists of pinging a container from its peer
```bash
kubectl exec -it <application's pod name> -- /bin/sh
```
```bash
ping <peer's IP>
```

### 5. Remove the network service
The network service can be removed by executing
```bash
curl -X DELETE http://<IML's node IP address>:30050/iml/yaml/deploy/<deployment id>
```

