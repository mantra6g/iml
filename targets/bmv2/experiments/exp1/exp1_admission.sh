#!/usr/bin/env bash
#
# Experiment 1 — Admission Control (RQ1)
#
# Deploys N NetworkFunctions up to the target's slot limit, then attempts one
# over-limit NF, and captures the evidence that the over-limit NF is rejected
# at the scheduler with a structured condition *before any P4Runtime call*.
#
# Usage:
#   ./exp1_admission.sh [N]      # run experiment with slot limit N (default 1)
#   ./exp1_admission.sh --reset  # delete this experiment's NFs and show slots
#
# Applies the manifests in ./exp1/. Requires kubectl, a
# running IML cluster, and the BMv2Target from exp1/00-target.yaml already applied
# with --max-nf-slots == N on it (default in code is 1). NFs nf-1..3 exist; for
# N>3 add more nf-<i>.yaml files following the same pattern.

set -euo pipefail

TARGET="${TARGET:-bmv2-target}"
NAMESPACE="${NAMESPACE:-default}"
LOOM_NS="${LOOM_NS:-loom-system}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFESTS="${HERE}"

reset() {
  echo "==> Resetting: deleting NetworkFunctions labelled experiment=exp1"
  kubectl delete nf -l experiment=exp1 -n "${NAMESPACE}" --ignore-not-found
  echo "==> Allocatable slots on ${TARGET}:"
  kubectl get p4target "${TARGET}" -o jsonpath='{.status.allocatable}{"\n"}' || true
}

if [[ "${1:-}" == "--reset" ]]; then
  reset
  exit 0
fi

N="${1:-1}"

echo "==> Capacity on ${TARGET} before run:"
kubectl get p4target "${TARGET}" -o jsonpath='{.status.capacity}{"\n"}'

echo "==> Step 1: deploying ${N} NF(s) up to the slot limit"
for i in $(seq 1 "${N}"); do
  manifest="${MANIFESTS}/nf-${i}.yaml"
  if [[ ! -f "${manifest}" ]]; then
    echo "ERROR: ${manifest} not found — add it for N=${N}." >&2
    exit 1
  fi
  kubectl apply -f "${manifest}"
  echo "   waiting for exp1-nf-${i} to be scheduled..."
  for _ in $(seq 1 30); do
    bound=$(kubectl get nf "exp1-nf-${i}" -n "${NAMESPACE}" -o jsonpath='{.spec.targetName}' 2>/dev/null || true)
    [[ -n "${bound}" ]] && break
    sleep 2
  done
  echo "   exp1-nf-${i} targetName=[${bound:-}]"
done

echo "==> Allocatable slots after filling (expect nf-slots: 0):"
kubectl get p4target "${TARGET}" -o jsonpath='{.status.allocatable}{"\n"}'

echo "==> Step 2: attempting the over-limit deployment"
kubectl apply -f "${MANIFESTS}/nf-overlimit.yaml"
sleep 12   # allow at least one scheduler requeue cycle (requeue is 10s)

echo
echo "==> Step 3: evidence"
echo "--- (a) structured Scheduled condition on the over-limit NF (expect status=False, reason=Unschedulable):"
kubectl get nf exp1-nf-overlimit -n "${NAMESPACE}" \
  -o jsonpath='{.status.conditions[?(@.type=="Scheduled")]}{"\n"}'

echo "--- (b) over-limit NF was never bound to a target (expect empty):"
echo "spec.targetName=[$(kubectl get nf exp1-nf-overlimit -n "${NAMESPACE}" -o jsonpath='{.spec.targetName}')]"

echo "--- (c) the driver never reconciled the over-limit NF (so no P4Runtime call)."
# The driver's NF controller has a WithEventFilter predicate that only admits NFs
# whose spec.targetName == this target; its entry-point log line
#   "Reconciling NetworkFunction" name=<nf>
# therefore never appears for an unscheduled (empty-targetName) NF.
echo "    over-limit NF (expect NONE):"
kubectl logs -n "${LOOM_NS}" -l infra.loom.io/targetDeployment="${TARGET}" \
  -c bmv2-driver --since=5m 2>/dev/null \
  | grep "Reconciling NetworkFunction" | grep "exp1-nf-overlimit" || echo "    NONE (expected)"
echo "    positive control — admitted NF exp1-nf-1 (expect a hit, proving the driver does see scheduled NFs):"
kubectl logs -n "${LOOM_NS}" -l infra.loom.io/targetDeployment="${TARGET}" \
  -c bmv2-driver --since=5m 2>/dev/null \
  | grep "Reconciling NetworkFunction" | grep "exp1-nf-1" | head -1 || echo "    (none found — check the driver is running)"

echo
echo "==> Done. Re-run with '--reset' to clean up between trials."
