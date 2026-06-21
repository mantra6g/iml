#!/usr/bin/env bash
#
# Experiment 3 — reset. Deletes everything the demo deploys (chains, NFs,
# configs, apps, deployments, services, targets) AND restarts the daemon.
#
# The daemon restart is required, not optional: the dataplane's app-subnet
# teardown (Software.DeleteAppInstance) is currently a no-op, so the in-memory
# d.appSubnets map (keyed by namespace/name) survives a delete. On the next
# deploy the recreated apps reuse the cached subnet and the daemon never
# re-patches the new Application's (empty) status.subnets — so chains get no
# route. Restarting the daemon clears that cache; the fresh deploy's new pods
# then re-register against an empty map and their status is written correctly.
#
# Usage: ./shared/reset.sh

source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

echo "==> Resetting Experiment 3 artifacts (demo dir: ${DEMO_DIR})"

# Delete in reverse dependency order; ignore anything already gone.
for m in nodeports.yml nfchains.yml deployments.yml nfs.yml apps.yml targets.yml; do
  kubectl delete -f "${DEMO_DIR}/${m}" --ignore-not-found 2>/dev/null || true
done

echo "==> Restarting the daemon to clear its in-memory app-subnet cache..."
kubectl rollout restart "ds/${DAEMON_DS}" -n "${LOOM_NS}"
kubectl rollout status  "ds/${DAEMON_DS}" -n "${LOOM_NS}" --timeout=90s || true

echo "==> Done.  Next: ./exp3_deploy.sh"
