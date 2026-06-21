#!/usr/bin/env bash
#
# Experiment 3 (B) — membership-change detection.
#
# Removes one NF (the qos stage) from the chain and shows that the daemon's
# reconcile loop observes the change. ServiceChain exposes no status condition,
# so the daemon's log is the signal:
#
#   "Reconciling ServiceChain"  -> a reconcile fired (within a small number of cycles)
#   "found matching NFs"         -> the new, smaller NF set        (needs debug logging)
#   "... file exists"            -> RouteAdd is not idempotent, so the kernel route
#                                   keeps the removed NF's segment (a deliberate RQ3 finding)
#
# Enable debug logging once so "found matching NFs" is emitted:
#   kubectl -n loom-system patch ds/loom-daemon --type=json \
#     -p '[{"op":"add","path":"/spec/template/spec/containers/0/args","value":["-zap-log-level=debug"]}]'
#
# Usage: ./exp3_membership.sh

source "$(dirname "${BASH_SOURCE[0]}")/shared/common.sh"

chain_exists || { echo "ERROR: ${CHAIN} not found (run ./exp3_deploy.sh first)." >&2; exit 1; }

# Membership before the change (the chain's NF stages).
echo "==> ${CHAIN} stages before:"
kubectl get servicechain "${CHAIN}" -n "${NAMESPACE}" \
  -o jsonpath='{range .spec.functions[*]}{.matchLabels.nf}{"\n"}{end}' | sed 's/^/      /'

# Remove stage ${REMOVE_STAGE} (qos) and read daemon logs from this moment on.
echo "==> Removing stage ${REMOVE_STAGE} (qos) from ${CHAIN}..."
since="$(iso)"
kubectl patch servicechain "${CHAIN}" -n "${NAMESPACE}" --type=json \
  -p "[{\"op\":\"remove\",\"path\":\"/spec/functions/${REMOVE_STAGE}\"}]"

sleep 3   # give the daemon a moment to reconcile

# How the daemon reacted, summarised from its logs since the patch.
echo
echo "==> Daemon reaction:"

# 1. The chain was reconciled — the detection, within a small number of cycles.
cycles="$(daemon_logs_since "${since}" | grep -c "Reconciling ServiceChain.*${CHAIN}" || true)"
echo "    reconciled ${cycles} time(s)"

# 2. It re-resolved a smaller NF set (qos gone). The NF names live deep in the
#    "found matching NFs" debug line, so pull just those out. Needs debug logging.
names="$(daemon_logs_since "${since}" | grep "found matching NFs" | tail -1 \
  | grep -oE '"metadata":\{"name":"[^"]+"' | sed -E 's/.*"name":"([^"]+)"/\1/' | paste -sd' ' -)"
echo "    new membership: ${names:-<enable daemon debug logging to see this>}"

# 3. RouteAdd is not idempotent, so the route update failed (the RQ3 finding).
err="$(daemon_logs_since "${since}" | grep "file exists" | tail -1 | grep -oE '"error": "[^"]+"' | sed -E 's/"error": "(.*)"/\1/')"
echo "    route update: ${err:-<none>}"

# Data-plane outcome: RouteAdd is not idempotent, so the route keeps qos's segment.
echo
echo "==> Route after removal (qos IP still present => change NOT applied to the data plane):"
node_seg6_routes | sed 's/^/      /'

# Put the chain back to full membership.
echo
echo "==> Restoring full membership"
kubectl apply -f "${DEMO_DIR}/nfchains.yml" >/dev/null
echo "==> Done."
