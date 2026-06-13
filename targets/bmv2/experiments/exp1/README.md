# Experiment 1 — Admission Control (RQ1)

Validates that an over-limit NetworkFunction is **rejected at the operator scheduler with a
structured condition, before any P4Runtime call** — the success criterion from `Proposal.tex`
(§Proposed Evaluation).

## How it works

An NF created without `spec.targetName` is scheduled by the operator. `filterFeasible` keeps a
target only if `Capacity["loom.io/nf-slots"] − (NFs already bound) > 0`
(`operator/internal/controller/core/networkfunction/networkfunction_controller.go`). When slots are
full, no target is feasible, so the NF gets `type=Scheduled, status=False, reason=Unschedulable` and
never receives a `targetName` — so it never reaches the driver. Slot capacity comes from the driver
flag `--max-nf-slots` (default **1**, `operator/.../bmv2target/util/bmv2_util.go`).

## Files

| File | Purpose |
|------|---------|
| `00-target.yaml` | The `BMv2Target` under test (provisions driver+switch and the derived `P4Target`). |
| `nf-1.yaml` … `nf-3.yaml` | NFs deployed up to the slot limit `N`. Use `nf-1` for the default `N=1`. |
| `nf-overlimit.yaml` | The over-limit NF that must be rejected. |
| `../exp1_admission.sh` | Automates the whole run + `--reset`. |

## Run by hand (default N=1)

```sh
# Prereqs (once): make docker-build-all && make kind-create && make kind-load-all && make install
kubectl apply -f 00-target.yaml
kubectl get p4target bmv2-target -o jsonpath='{.status.capacity}{"\n"}'   # nf-slots: 1

# Step 1 — fill the slot
kubectl apply -f nf-1.yaml
kubectl get nf exp1-nf-1 -o jsonpath='{.spec.targetName}{"\n"}'           # non-empty => scheduled
kubectl get p4target bmv2-target -o jsonpath='{.status.allocatable}{"\n"}' # nf-slots: 0

# Step 2 — over-limit
kubectl apply -f nf-overlimit.yaml
sleep 12

# Step 3 — evidence
kubectl get nf exp1-nf-overlimit -o jsonpath='{.status.conditions[?(@.type=="Scheduled")]}{"\n"}'
#   expect: status=False, reason=Unschedulable, message="No matching targets found"
kubectl get nf exp1-nf-overlimit -o jsonpath='spec.targetName=[{.spec.targetName}]{"\n"}'
#   expect: spec.targetName=[]
kubectl logs -n loom-system -l infra.loom.io/targetDeployment=bmv2-target -c bmv2-driver \
  --since=5m | grep "Reconciling NetworkFunction" | grep "exp1-nf-overlimit" || echo "NONE (expected)"
```

## Larger N (e.g. 3)

Raise `--max-nf-slots` from `"1"` to `"3"` in `bmv2target/util/bmv2_util.go`, then
`make docker-build-iml && make kind-load-all`, restart the operator, and re-apply `00-target.yaml`.
Then apply `nf-1.yaml`, `nf-2.yaml`, `nf-3.yaml` before `nf-overlimit.yaml`.

## Reset

```sh
kubectl delete nf -l experiment=exp1 --ignore-not-found   # frees slots; keeps the target
# or, full reset:
kubectl delete -f 00-target.yaml
```

## One-shot script

```sh
kubectl apply -f 00-target.yaml      # once
../exp1_admission.sh 1                # run (N=1)
../exp1_admission.sh --reset         # clean up between trials
```

## Troubleshooting

### Every NF is `Unschedulable` / `kubectl get p4target` errors / no `P4Target` exists

Symptom chain (all stem from the `P4Target` CRD scope being changed under running controllers):

```
$ kubectl get p4target
Error from server (NotFound): ... the server could not find the requested resource (get p4targets.core.loom.io)

# driver logs (kubectl logs -n loom-system deploy/bmv2-target -c bmv2-driver):
ERROR p4target-status-updater Failed to create P4Target {"error": "an empty namespace may not be set during creation"}

# operator logs (kubectl logs -n loom-system deploy/loom-controller-manager):
ERROR core-p4target Failed to ensure readiness state for P4Target {"error": "an empty namespace may not be set when a resource name is provided"}
```

Root cause: `P4Target` is **cluster-scoped** in source (`api/core/v1alpha1/p4target_types.go`,
`+kubebuilder:resource:scope=Cluster`) and the driver creates it by name only (no namespace). If the
cluster still has the older **namespaced** CRD, the driver's create is rejected, so no `P4Target` ever
exists and the scheduler rejects every NF (`targetsProcessed: 0`).

**A CRD's `spec.scope` is immutable**, so `kubectl apply` cannot change it — the CRD must be deleted and
recreated. AND every long-running controller caches a RESTMapper at startup, so after the scope change
they keep using the *old* scope until restarted. Full fix:

```sh
# 1. Recreate the CRD with the correct (Cluster) scope. Safe: no P4Target CRs exist.
kubectl delete crd p4targets.core.loom.io
make install
kubectl get crd p4targets.core.loom.io -o jsonpath='{.spec.scope}{"\n"}'   # -> Cluster

# 2. Clear kubectl's stale discovery cache (it may have cached the resource's absence).
rm -rf ~/.kube/cache/discovery
kubectl get p4target                                                       # -> No resources found (no error)

# 3. Restart BOTH controllers so they rebuild their RESTMappers.
#    Operator: a normal rollout restart sticks.
kubectl rollout restart deploy/loom-controller-manager -n loom-system
#    Driver: the operator OWNS this deployment and reverts a rollout restart's template change,
#    so delete the pod directly to force a fresh one.
kubectl delete pod -n loom-system -l infra.loom.io/targetDeployment=bmv2-target
```

Within ~30s the chain settles: operator allocates `spec.nfCIDR` -> driver populates status. Verify:

```sh
kubectl get p4target bmv2-target -o jsonpath='nfCIDR=[{.spec.nfCIDR}] capacity={.status.capacity}{"\n"}'
# expect: nfCIDR=[fd00::/64] capacity={"loom.io/nf-slots":"1"}
kubectl get p4target   # READY -> True
```

Notes:
- A transient operator error `the object has been modified; please apply your changes to the latest
  version` is a harmless optimistic-concurrency race (operator and driver both patch the object) and
  resolves on retry.
- `table-entries` is absent from `capacity` until a P4 pipeline is loaded; the scheduler only checks
  `nf-slots`, so this does not affect admission control.

### `targetsProcessed: 0` even though the P4Target is Ready

Some other NFs may already occupy the slot(s). Check `kubectl get nf -A` and clear leftovers (e.g. a
demo `NetworkFunctionDeployment`): `kubectl delete networkfunctiondeployment <name> -n <ns>`.
