# Infrastructure Management Layer: Local NFV Orchestrator
This local NFVO is
responsible for handling the available resources at the given DESIRE6G site, perform local optimization
through selecting the proper resources to implement the network service subgraph, deploy both
network and application functions and configure networking including the virtual management
networks and the data network. The configuration of the data network includes the setup of various
infrastructure network functions. 

![Local NFV Orchestrator Architecture](https://i.imgur.com/YrMYPd9.png)

The local network service descriptor (localNSD) is created by SMO and handed to the local NFVO component of
IML to initiate the deployment of the subgraph on the specific site. 

# Installation Steps
Before installing and running the Local NFV Orchestrator, you should:
1. Install kubectl (see steps [here](https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/))
2. Install Helm (see steps [here](https://helm.sh/docs/intro/install/))
3. Clone the repository with
```bash
git clone https://github.com/DESIRE6G/IML-LNFVO
```
```bash
cd IML-LNFVO
```
4. Install Python, create and switch to a virtual environment
```bash
python3 -m venv .venv
```
```bash
source .venv/bin/activate
```
5. Install required Python packages
```bash
pip install -r requirements.txt
```

# Running LNFVO

## Start NFVO
```bash
python nfvo-api.py
```

## Deploy a Network Service Descriptor (NSD)
```bash
curl -F file=@<nsd-name>.yml http://localhost:5000/iml/yaml/deploy
```

This will generate the values.yaml in the deploy folder and deploy the graph-chart with it.

The generated values can be used to redeploy after chart deletion:
```bash
helm install --namespace desire6g --create-namespace <release-name> --post-renderer ./kustomize.sh -f <values-path> ./graph-chart/
```

## Verify
```bash
kubectl get -n desire6g pods -o wide
```

## Stop
```bash
kubectl delete namespace desire6g
```

## Execute commands inside container
```bash
kubectl exec -it deploy/<name> -- /bin/bash # or /bin/sh
```

## Ping
Deploy interpod or internode nsd and ping from inside the container to the dst ip

## Cluster prerequisites
* Configured hugepages
* Configured sr-iov vf-s on nodes with [sriov-network-device-plugin](https://github.com/k8snetworkplumbingwg/sriov-network-device-plugin)
* Configured vpp on nodes, if needed
