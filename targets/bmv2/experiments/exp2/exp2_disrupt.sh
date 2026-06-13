#!/usr/bin/env bash
#
# Experiment 2 (B) — disruption and recovery.
#
# With exp2-nf live, inject a fault and time how the control plane detects the
# divergence (NF/target Ready flips, or a taint appears) and reconverges.
#
#   switch — crash the BMv2 switch while the driver keeps running.
#   pod    — delete the whole target pod (driver + switch).
#   both   — run switch then pod (default).
#
# Usage: ./disrupt.sh [switch|pod|both]

source "$(dirname "${BASH_SOURCE[0]}")/shared/common.sh"

# Make sure exp2-nf is deployed and Ready before we break anything.
ensure_nf_ready() {
  if [[ "$(nf_ready)" != "True" ]]; then
    echo "==> Deploying ${NF} and waiting for Ready..."
    kubectl apply -f "${HERE}/nf-lifecycle.yaml" >/dev/null
    local deadline=$(( SECONDS + TIMEOUT ))
    while [[ "$(nf_ready)" != "True" ]]; do
      (( SECONDS >= deadline )) && { echo "ERROR: ${NF} never became Ready." >&2; exit 1; }
      sleep "${POLL}"
    done
  fi
  echo "    pre-disruption: NF Ready=$(nf_ready)  target Ready=$(target_ready)  taints=[$(target_taints)]"
}

# Observe and time divergence detection + reconvergence after an injection at t0
# (t0 is the epoch from now() captured right before the fault was injected).
observe_recovery() {
  local t0="$1" label="$2" deadline t_detect t_recover
  echo "--- ${label}: watching for divergence then recovery..."

  # Detection: NF no longer Ready, target no longer Ready, or a taint appeared.
  local detected=0; deadline=$(( SECONDS + TIMEOUT ))
  while (( SECONDS < deadline )); do
    if [[ "$(nf_ready)" != "True" || "$(target_ready)" != "True" || -n "$(target_taints)" ]]; then detected=1; break; fi
    sleep "${POLL}"
  done
  t_detect="$(now)"
  if (( ! detected )); then
    echo "    detection: NONE within ${TIMEOUT}s — control plane never saw the divergence (note this)."
    return
  fi
  echo "    detection: divergence after $(since "${t0}" "${t_detect}")s from injection  (NF=$(nf_ready) target=$(target_ready) taints=[$(target_taints)])"

  # Recovery: NF Ready and target Ready with no remaining taints. Reported both
  # from detection and from injection so the two latencies are unambiguous.
  local recovered=0; deadline=$(( SECONDS + TIMEOUT ))
  while (( SECONDS < deadline )); do
    if [[ "$(nf_ready)" == "True" && "$(target_ready)" == "True" && -z "$(target_taints)" ]]; then recovered=1; break; fi
    sleep "${POLL}"
  done
  t_recover="$(now)"
  if (( recovered )); then
    echo "    recovery:  reconverged $(since "${t_detect}" "${t_recover}")s after detection ($(since "${t0}" "${t_recover}")s from injection)"
  else
    echo "    recovery:  NOT reconverged within ${TIMEOUT}s  (NF=$(nf_ready) target=$(target_ready) taints=[$(target_taints)])"
    echo "               ^ a valid RQ2 finding: this disruption needs operator-driven pod replacement."
  fi
}

# id of this target's bmv2-switch CRI container, looked up through the kind node.
# Matching on the pod name avoids touching the unrelated bmv2-baseline-switch.
crictl_switch_cid() {
  docker exec "$2" crictl ps --name bmv2-switch 2>/dev/null | awk -v p="$1" 'index($0,p){print $1; exit}'
}

inject_switch_crash() {
  local pod node rc0 t0 cid crashed=0
  pod="$(target_pod)"; [[ -n "${pod}" ]] || { echo "ERROR: target pod not found." >&2; exit 1; }
  node="$(kubectl get pod -n "${LOOM_NS}" "${pod}" -o jsonpath='{.spec.nodeName}' 2>/dev/null)"
  rc0="$(switch_restarts "${pod}")"
  echo "==> Switch crash: killing the bmv2-switch container in ${pod} (restartCount=${rc0})"
  t0="$(now)"

  if command -v docker >/dev/null 2>&1 && docker ps --format '{{.Names}}' | grep -qx "${node}"; then
    # On kind the node is a local Docker container, so stop the switch through its
    # CRI. This kills ONLY the switch (the driver keeps running), unlike
    # 'kubectl exec kill 1' which PID-1 signal protection silently ignores. Hold
    # it down ~14s so the driver's ~10s health check is sure to observe the loss.
    echo "    stopping switch via ${node} CRI, holding it down ~14s (driver stays up)..."
    local hold_until=$(( SECONDS + 14 ))
    while (( SECONDS < hold_until )); do
      cid="$(crictl_switch_cid "${pod}" "${node}")"
      [[ -n "${cid}" ]] && docker exec "${node}" crictl stop --timeout 1 "${cid}" >/dev/null 2>&1 && crashed=1
      sleep 4
    done
  else
    # Non-kind fallback: in-container kill (likely a no-op under PID-1 protection).
    kubectl exec -n "${LOOM_NS}" "${pod}" -c bmv2-switch -- kill -9 1 >/dev/null 2>&1 || true
  fi

  # Confirm the switch actually crashed (restartCount went up); else fall back to
  # a pod restart so a disruption always happens.
  local restarted=0 deadline=$(( SECONDS + 30 ))
  while (( SECONDS < deadline )); do
    [[ "$(switch_restarts "${pod}")" -gt "${rc0}" ]] && { restarted=1; break; }
    sleep "${POLL}"
  done
  if (( restarted )); then
    echo "    bmv2-switch crashed and is restarting; driver stayed up."
    observe_recovery "${t0}" "switch-crash"
  else
    echo "    switch did not restart — falling back to pod restart."
    inject_pod_restart
  fi
}

inject_pod_restart() {
  local pod t0
  pod="$(target_pod)"; [[ -n "${pod}" ]] || { echo "ERROR: target pod not found." >&2; exit 1; }
  echo "==> Driver/pod restart: deleting pod ${pod}"
  t0="$(now)"
  kubectl delete pod -n "${LOOM_NS}" "${pod}" --wait=false >/dev/null
  observe_recovery "${t0}" "pod-restart"
}

main() {
  require_target_ready
  case "${1:-both}" in
    switch) ensure_nf_ready; inject_switch_crash ;;
    pod)    ensure_nf_ready; inject_pod_restart ;;
    both)   ensure_nf_ready; inject_switch_crash; echo; ensure_nf_ready; inject_pod_restart ;;
    *) echo "usage: $0 [switch|pod|both]" >&2; exit 1 ;;
  esac
}

main "$@"
