#!/usr/bin/env bash
#
# Experiment 3 — deploy the end-to-end service-chain PoC.
#
# Applies the examples/demo manifests (reused as-is) in dependency order and
# waits for the NetworkFunctions to become Ready. Once they are, the daemon
# programs the chains' SRv6 routes within a second or two — verify that with
# ./exp3_steer.sh. The demo's custom images (proxy, servers, lb-controller,
# perf bmv2) must already be built and loaded into the cluster:
#
#   make -C "${DEMO_DIR}" docker-build-all kind-load-all
#
# Usage: ./exp3_deploy.sh

source "$(dirname "${BASH_SOURCE[0]}")/shared/common.sh"

main() {
  echo "==> Demo dir: ${DEMO_DIR}"
  [[ -f "${DEMO_DIR}/nfchains.yml" ]] || { echo "ERROR: demo manifests not found at ${DEMO_DIR}." >&2; exit 1; }

  # Apply the demo manifests in dependency order: targets first, chains last.
  kubectl apply -f "${DEMO_DIR}/targets.yml"       # the 4 BMv2Target switches
  kubectl apply -f "${DEMO_DIR}/apps.yml"          # Application resources (chain endpoints)
  kubectl apply -f "${DEMO_DIR}/nfs.yml"           # NetworkFunctions (qos, traffic-manager, lb) + configs
  kubectl apply -f "${DEMO_DIR}/deployments.yml"   # servers, proxies, lb-controllers
  kubectl apply -f "${DEMO_DIR}/nfchains.yml"      # the ServiceChains
  kubectl apply -f "${DEMO_DIR}/nodeports.yml"     # NodePort services (30007 web, 30008 streaming)

  echo "==> Waiting for NetworkFunctions to become Ready..."
  # The chain only routes once each stage has a Ready NF with an IP.
  kubectl wait --for=condition=Ready nf --all -n "${NAMESPACE}" --timeout="${TIMEOUT}s" || \
    echo "    (some NFs not Ready yet; continuing — the daemon will route once they are)"

  kubectl get servicechain -n "${NAMESPACE}"
  echo
  echo "==> Deployed. Next:"
  echo "    ./exp3_steer.sh        # Part A: traffic steered through the chain"
  echo "    ./exp3_membership.sh   # Part B: membership-change detection"
}

main "$@"
