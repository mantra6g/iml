kubectl apply -f builder/dist/install.yaml
kubectl apply -f cni/install.yml
kubectl apply -f go-daemon/install.yml
kubectl apply -f runtime-proxy/install.yml
kubectl apply -f iml-oakestra-agent/install.yml