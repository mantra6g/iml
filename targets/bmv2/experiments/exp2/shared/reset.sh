#!/usr/bin/env bash
#
# Experiment 2 — reset. Deletes everything the experiment scripts create
# (this experiment's NFs and the throwaway baseline switch) and leaves the
# target in place. Run between runs instead of relying on per-script cleanup.
#
# Usage: ./shared/reset.sh

source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

echo "==> Resetting Experiment 2 artifacts"
kubectl delete nf -l experiment=exp2 -n "${NAMESPACE}" --ignore-not-found
kubectl delete pod bmv2-baseline-switch -n "${NAMESPACE}" --ignore-not-found
echo "    target ${TARGET} left in place ('kubectl delete -f 00-target.yaml' to remove it)."
echo "    if a switch crash left the data plane wedged, restart the target so the driver re-pushes:"
echo "      kubectl delete pod -n ${LOOM_NS} -l infra.loom.io/targetDeployment=${TARGET}"
