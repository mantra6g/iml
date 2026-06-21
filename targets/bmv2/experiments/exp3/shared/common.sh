#!/usr/bin/env bash
#
# Shared configuration and helpers for the Experiment 3 scripts
# (exp3_deploy.sh, exp3_steer.sh, exp3_membership.sh). This file is *sourced*,
# not run directly.

set -euo pipefail

# --- Configuration -----------------------------------------------------------
# Plain values (no environment-variable indirection) so the scripts read top to
# bottom. Change a setting by editing it here.
NAMESPACE="default"          # namespace the demo is deployed into
LOOM_NS="loom-system"        # namespace of the IML daemon

# The daemon programs the SRv6 routes and logs every ServiceChain reconcile.
DAEMON_LABEL="app.kubernetes.io/name=loom-daemon"
DAEMON_CONTAINER="daemon"
DAEMON_DS="loom-daemon"

CHAIN="streaming-downlink"   # multi-stage chain under test: streaming-lb -> qos -> traffic-manager
REMOVE_STAGE=1               # index into spec.functions to remove (1 == qos)

TIMEOUT=120                  # seconds to wait for NetworkFunctions to become Ready

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"        # the exp3 dir (this file lives in shared/)
DEMO_DIR="$(cd "${HERE}/../../../../examples/demo" && pwd)"    # reuse examples/demo as-is

# --- Timing ------------------------------------------------------------------
iso() { date -u +%Y-%m-%dT%H:%M:%SZ; }   # RFC3339 timestamp for kubectl logs --since-time

# --- Cluster / node helpers --------------------------------------------------
# First node name; on kind this is also the name of the node's Docker container,
# whose netns holds the SRv6 routes the (hostNetwork) daemon programs.
kind_node() { kubectl get nodes -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true; }

# SRv6 *encap* routes currently in the node netns — these are the service-chain
# routes whose segment list is the NF IPs on-path. Filtered to "seg6 mode encap"
node_seg6_routes() {
  docker exec "$(kind_node)" ip route show table all | grep -iE 'seg6 mode encap' || true
}

# Daemon logs since an RFC3339 timestamp.
daemon_logs_since() {
  kubectl logs -n "${LOOM_NS}" -l "${DAEMON_LABEL}" -c "${DAEMON_CONTAINER}" \
    --since-time="$1" --prefix=false 2>/dev/null || true
}

# --- ServiceChain queries ----------------------------------------------------
chain_exists() { kubectl get servicechain "${CHAIN}" -n "${NAMESPACE}" >/dev/null 2>&1; }
