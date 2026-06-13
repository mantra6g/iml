#!/usr/bin/env bash
#
# Shared configuration and helpers for the Experiment 2 scripts
# (exp2_baseline.sh, exp2_convergence.sh, exp2_disrupt.sh). This file is *sourced*,
# not run directly.

set -euo pipefail

# --- Configuration (override via environment) --------------------------------
TARGET="${TARGET:-bmv2-target}"
NAMESPACE="${NAMESPACE:-default}"
LOOM_NS="${LOOM_NS:-loom-system}"
NF="${NF:-exp2-nf}"
P4URL="${P4URL:-https://raw.githubusercontent.com/mantra6g/iml/refs/heads/main/examples/simple/logger.p4}"
P4C_IMAGE="${P4C_IMAGE:-p4lang/p4c}"                          # compiles the baseline pipeline if host p4c is absent
P4RT_DOCKER_IMAGE="${P4RT_DOCKER_IMAGE:-p4lang/p4runtime-sh}" # official P4Runtime client (run as a container)
TRIALS="${TRIALS:-20}"
POLL="${POLL:-0.2}"           # polling interval (seconds) while waiting for state changes
TIMEOUT="${TIMEOUT:-180}"     # per-trial / per-recovery timeout (seconds)

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"   # the exp2 dir (manifests live here; this file is in shared/)
OUTDIR="${OUTDIR:-${HERE}/results}"
mkdir -p "${OUTDIR}"

# --- Timing ------------------------------------------------------------------
now()   { date +%s.%N; }
since() { awk -v a="$1" -v b="$2" 'BEGIN{printf "%.3f", b - a}'; }   # seconds between two now() stamps

# --- Cluster state queries ---------------------------------------------------
nf_ready()        { kubectl get nf "${NF}" -n "${NAMESPACE}" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true; }
nf_target()       { kubectl get nf "${NF}" -n "${NAMESPACE}" -o jsonpath='{.spec.targetName}' 2>/dev/null || true; }
target_ready()    { kubectl get p4target "${TARGET}" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true; }
target_taints()   { kubectl get p4target "${TARGET}" -o jsonpath='{.spec.taints[*].key}' 2>/dev/null || true; }
target_pod()      { kubectl get pod -n "${LOOM_NS}" -l "infra.loom.io/targetDeployment=${TARGET}" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true; }
switch_restarts() { kubectl get pod -n "${LOOM_NS}" "$1" -o jsonpath='{.status.containerStatuses[?(@.name=="bmv2-switch")].restartCount}' 2>/dev/null || echo 0; }

# Wait (up to 120s) for the target under test to be Ready before measuring.
require_target_ready() {
  echo "==> Checking ${TARGET} is Ready..."
  local deadline=$(( SECONDS + 120 ))
  while [[ "$(target_ready)" != "True" ]]; do
    (( SECONDS >= deadline )) && { echo "ERROR: ${TARGET} not Ready (apply 00-target.yaml first?)." >&2; exit 1; }
    sleep "${POLL}"
  done
  echo "    ${TARGET} Ready=True, taints=[$(target_taints)]"
}

# --- Stats: read one number per line on stdin, print a summary line ----------
# Pre-sort with sort(1) so awk can index percentiles (mawk lacks asort()).
stats() {
  sort -n | awk '
    { value[NR] = $1; sum += $1 }
    END {
      n = NR
      if (n == 0) { print "    no samples"; exit }
      mean = sum / n
      for (i = 1; i <= n; i++) { diff = value[i] - mean; sumsq += diff * diff }
      stddev = sqrt(sumsq / n)                              # population standard deviation
      median = value[int((n + 1) / 2)]                      # middle sample of the sorted list
      p95 = int((95 * n + 99) / 100); if (p95 > n) p95 = n  # nearest-rank 95th percentile
      printf "    n=%d  min=%.3f  median=%.3f  mean=%.3f  p95=%.3f  max=%.3f  stddev=%.3f  (s)\n", \
             n, value[1], median, mean, value[p95], value[n], stddev
    }'
}
