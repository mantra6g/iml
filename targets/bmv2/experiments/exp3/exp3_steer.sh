#!/usr/bin/env bash
#
# Experiment 3 (A) — traffic steered through the chain.
#
# Demonstrates that the deployed ServiceChain actually steers traffic across its
# SR-unaware NFs, with two kinds of evidence:
#
#   1. Route proof        — the SRv6 route programmed by the daemon lists each
#                           stage's NF IP as a segment (deterministic).
#   2. Connectivity proof — real traffic generated from a healthy SOURCE app pod
#                           to the chain's DEST app is forwarded end-to-end; since
#                           the SRv6 route forces it through the NF segment(s), a
#                           successful exchange proves the NF is on the live path.
#
# Notes:
#  - The route proof checks ${CHAIN}'s stages (default the 3-NF streaming-downlink).
#    Its SOURCE app is a media server that crashloops in this environment (the
#    demo's *.mkv are Git-LFS stubs), so live traffic uses an UPLINK chain whose
#    source is a healthy proxy pod (default local-streaming-proxy -> dummy-streaming-
#    server via streaming-lb). With working video files the same probe from the
#    media server would traverse the full multi-NF chain.
#  - The demo P4 programs (marker/rifo/lb) define P4 *registers*, which is read best-effort.
#
# Usage: ./exp3_steer.sh

source "$(dirname "${BASH_SOURCE[0]}")/shared/common.sh"

NF_LABELS="streaming-lb qos traffic-manager"   # ${CHAIN} stages checked by the route proof
SRC_APP="local-streaming-proxy"                # healthy source app for the connectivity proof
DST_APP="dummy-streaming-server"               # its destination app
PROBE_NF="streaming-lb"                         # NF on that path (also read via /api/registers)
PINGS=5                                          # packets sent in the connectivity proof

nf_target() { kubectl get nf "$1" -n "${NAMESPACE}" -o jsonpath='{.spec.targetName}' 2>/dev/null || true; }
driver_pod_for() { kubectl get pod -n "${LOOM_NS}" -l "infra.loom.io/targetDeployment=$1" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true; }
app_pod() { kubectl get pod -n "${NAMESPACE}" -l "app=$1" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true; }

# The loom-cni (iml0) IPv6 address of a pod, from its multus network-status.
pod_iml6() {
  kubectl get pod -n "${NAMESPACE}" "$1" \
    -o jsonpath='{.metadata.annotations.k8s\.v1\.cni\.cncf\.io/network-status}' 2>/dev/null \
    | grep -oE 'fd00:[0-9a-f:]+' | head -1
}

# --- 1. Route proof ----------------------------------------------------------
# Show the SRv6 routes the daemon programmed, then the NF IPs, so you can see
# that each NF IP appears as a segment in the chain's route.
route_proof() {
  echo "==> [1] Route proof — ${CHAIN} NF IPs appear as SRv6 segments"
  local routes; routes="$(node_seg6_routes)"
  if [[ -z "${routes}" ]]; then
    echo "    no SRv6 chain routes found — is the demo deployed and are the NFs Ready?"
    echo "    (clean re-run: ./shared/reset.sh && ./exp3_deploy.sh)"
    exit 1
  fi
  echo "    SRv6 routes (each segment is an NF IP):"
  echo "${routes}" | sed 's/^/      /'
  echo "    NF IPs (match these against the segments above):"
  kubectl get nf ${NF_LABELS} -n "${NAMESPACE}" \
    -o jsonpath='{range .items[*]}{.metadata.name}={.status.assignedIP}{"\n"}{end}' | sed 's/^/      /'
}

# --- 2. Connectivity proof ---------------------------------------------------
# Send real traffic from a healthy source pod to the destination; the SRv6 route
# forces it through ${PROBE_NF}, so 0% packet loss proves it was forwarded.
connectivity_proof() {
  echo
  echo "==> [2] Connectivity proof — traffic ${SRC_APP} -> ${DST_APP} via ${PROBE_NF}"
  local src dst dip
  src="$(app_pod "${SRC_APP}")"            # source pod
  dst="$(app_pod "${DST_APP}")"            # destination pod
  dip="$(pod_iml6 "${dst}")"               # destination's loom-cni (iml0) IPv6
  echo "    ping6 ${SRC_APP} -> ${DST_APP} (${dip}):"
  kubectl exec -n "${NAMESPACE}" "${src}" -c proxy -- ping6 -c "${PINGS}" -W 2 "${dip}" | sed 's/^/      /'
  registers_best_effort
}

# --- /api/registers (best-effort) ------
registers_best_effort() {
  local tgt pod; tgt="$(nf_target "${PROBE_NF}")"; pod="$(driver_pod_for "${tgt}")"
  [[ -n "${pod}" ]] || return 0
  local regs; regs="$(kubectl exec -n "${LOOM_NS}" "${pod}" -c bmv2-driver -- \
    wget -qO- http://localhost:8080/api/registers 2>/dev/null || true)"
  echo "    ${PROBE_NF} /api/registers: ${regs:-<unavailable>}"
}

main() {
  route_proof
  connectivity_proof
}

main "$@"
