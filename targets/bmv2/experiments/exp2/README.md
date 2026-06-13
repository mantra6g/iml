# Experiment 2 — Lifecycle & Recovery (RQ2)

Validates the two RQ2 claims from `../resources/Proposal.tex` (§Proposed Evaluation):

> **Lifecycle and recovery (RQ2).** Measure the time for an NF to converge from creation to a ready
> condition over repeated trials, and inject a disruption (e.g. a target or driver restart) during the
> NF lifecycle. *Success:* convergence within a pre-committed multiple of a direct gRPC baseline, and
> the controller detects the resulting divergence and reconverges the target toward its desired state.

So there are two halves:

- **(A) Convergence** — time `kubectl apply` → NF `Ready`, repeated, and compare to a **direct-gRPC
  baseline**. Pre-committed success: median (and p95) convergence **≤ 10×** the baseline median.
- **(B) Recovery** — disrupt the target (switch crash and/or driver/pod restart) while an NF is live,
  and check the control plane **detects** the divergence (Ready flips / taints appear) and
  **reconverges**.

## How it works

### Convergence path (what is timed)

```
kubectl apply nf-lifecycle.yaml
   └─ operator schedules: sets spec.targetName            ── "schedule" sub-stage
        └─ driver compiles + deploys + wires the NF       ── "configure" sub-stage
             └─ NF condition type=Ready flips status=True ── convergence end
```

`total = Ready − apply`, split into `schedule` (operator) and `configure` (driver). The driver's
per-phase logs (`Compiling…`, `Deploying…`, `Network function setup successfully`) are emitted at V(1);
enabling verbose driver logging surfaces them, but the condition timestamps alone are sufficient.

### Direct-gRPC baseline (the comparison floor)

The baseline uses the **official p4lang client, `p4runtime-shell`**, run as the `p4lang/p4runtime-sh`
container — no bespoke measurement code, so the instrument itself needs no separate validation. It pushes
the same `logger.p4` onto a **bare, driverless** BMv2 switch (`baseline/bare-switch.yaml`), and
`exp2_baseline.sh` wraps the invocation in `date(1)` for wall-clock timing.

Because the client is a Python CLI in a container, one invocation includes fixed container + interpreter +
gRPC-channel startup on top of the actual `SetForwardingPipelineConfig`. Rather than hide this, each trial
times **two** runs and records both columns in `results/baseline.csv`:

- `full_ms` — a full run **with** `--config` (connect + arbitrate + **push the pipeline**): the end-to-end
  standalone "time to a configured switch".
- `startup_ms` — a reference run **without** `--config` (connect + arbitrate only): the tool's fixed cost.

The isolated push is `full − startup` (also printed per trial). `full_ms` is the headline baseline;
`startup_ms` is shown for transparency so a reader can see how much is protocol vs. fixed startup, which
appears in both runs and cancels in `full − startup`. Compilation is done once up front by the script and
is never in the timed path.

### Recovery machinery (what detects + reconverges)

Driver defaults (`cmd/main.go`): Lease renew **5 s**, lease duration **40 s** (namespace `loom-system`,
lease named after the target); P4Target status pushed every **10 s**; `Healthy` flips to
`reason=SwitchUnreachable` when the BMv2 gRPC `Capabilities()` call fails. The operator
(`operator/internal/controller/core/p4target/`) watches the Lease and applies taints —
`Ready=False` → `p4target.loom.io/not-ready`; lease expired/Unknown → `p4target.loom.io/unreachable`;
`Ready=True` → both removed.

## Files

| File | Purpose |
|------|---------|
| `00-target.yaml` | The `BMv2Target` under test (same as Experiment 1; one NF slot is enough). |
| `nf-lifecycle.yaml` | The single NF (`exp2-nf`, `logger.p4`) whose convergence is timed. |
| `baseline/bare-switch.yaml` | A driverless `simple_switch_grpc` for the `p4runtime-shell` baseline. |
| `shared/common.sh` | Shared config + helpers, sourced by the scripts below (not run directly). |
| `shared/reset.sh` | Deletes this experiment's NFs and the baseline switch; keeps the target. |
| `exp2_baseline.sh` | Direct-gRPC baseline — times the `p4lang/p4runtime-sh` container against a bare switch. |
| `exp2_convergence.sh` | NF create→Ready timing over N trials. |
| `exp2_disrupt.sh` | Fault injection (`switch`/`pod`/`both`) + recovery timing. |
| `shared/plot_results.py` | Plots `converge.csv` vs `baseline.csv` → `results/resources/exp2_breakdown.png` (matplotlib). |
| `results/` | Output: `baseline.csv`, `converge.csv`, and the compiled `pipeline/` — created on first run. |

**Extra tooling** (beyond a running IML cluster + `kubectl`): **Docker** — the script runs the official
`p4runtime-shell` baseline client as the `p4lang/p4runtime-sh` container, and compiles the baseline
pipeline with `p4c` on PATH **or**, failing that, in a `p4lang/p4c` container (`p4c` lives in the driver
image, not on the dev host).

## Run by hand

```sh
# Prereqs (once): make docker-build-all && make kind-create && make kind-load-all && make install
kubectl apply -f 00-target.yaml
kubectl get p4target bmv2-target -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}{"\n"}'  # True

# --- (A) Convergence: one trial by hand ---
date +%s.%N; kubectl apply -f nf-lifecycle.yaml
kubectl get nf exp2-nf -o jsonpath='{.spec.targetName}{"\n"}'              # non-empty => scheduled
kubectl wait --for=condition=Ready nf/exp2-nf --timeout=180s; date +%s.%N  # convergence end
kubectl delete nf exp2-nf

# --- (A) Baseline: official p4runtime-shell (container) against a bare switch ---
# compile the pipeline once (host p4c, or: docker run --rm -v "$PWD/results/pipeline:/work" \
#   p4lang/p4c p4c --target bmv2 --arch v1model --p4runtime-files /work/p4info.txt \
#   --p4runtime-format text -o /work /work/logger.p4)
kubectl apply -f baseline/bare-switch.yaml
kubectl wait --for=condition=Ready pod/bmv2-baseline-switch --timeout=120s
kubectl port-forward pod/bmv2-baseline-switch 9559:9559 &      # note the PID
# full run pushes the pipeline (time this); a no-config run is the startup reference
time docker run --rm --network host -v "$PWD/results/pipeline:/cfg" p4lang/p4runtime-sh \
  --grpc-addr localhost:9559 --device-id 0 --election-id 0,1 \
  --config /cfg/p4info.txt,/cfg/logger.json </dev/null
kill %1; kubectl delete pod bmv2-baseline-switch

# --- (B) Disruption: switch crash (driver stays up) ---
# On kind the node is a local Docker container, so crash the switch via its CRI
# (kubectl exec 'kill 1' is a no-op — PID-1 signal protection; see caveat below).
kubectl apply -f nf-lifecycle.yaml; kubectl wait --for=condition=Ready nf/exp2-nf
POD=$(kubectl get pod -n loom-system -l infra.loom.io/targetDeployment=bmv2-target -o jsonpath='{.items[0].metadata.name}')
NODE=$(kubectl get pod -n loom-system "$POD" -o jsonpath='{.spec.nodeName}')
CID=$(docker exec "$NODE" crictl ps --name bmv2-switch | awk -v p="$POD" 'index($0,p){print $1}')
docker exec "$NODE" crictl stop --timeout 1 "$CID"     # repeat for ~14s to outlast the 10s health check
watch kubectl get p4target bmv2-target \
  -o 'jsonpath={.spec.taints[*].key}  ready={.status.conditions[?(@.type=="Ready")].status}'

# --- (B) Disruption: driver/pod restart ---
kubectl delete pod -n loom-system -l infra.loom.io/targetDeployment=bmv2-target
#   expect: lease ages -> unreachable/not-ready taint -> new pod re-pushes -> Ready=True, exp2-nf Ready=True
```

## Scripts

```sh
kubectl apply -f 00-target.yaml      # once
./exp2_baseline.sh 20        # -> results/baseline.csv (full_ms, startup_ms) via p4runtime-sh
./exp2_convergence.sh 20     # -> results/converge.csv + median/p95 (total, schedule, configure)
./exp2_disrupt.sh both       # switch crash + pod restart, timing detection & recovery (or: switch | pod)

# run the lot:
./exp2_baseline.sh && ./exp2_convergence.sh && ./exp2_disrupt.sh

# visualise baseline vs convergence (needs matplotlib):
python3 shared/plot_results.py     # -> results/resources/exp2_breakdown.png
```

The three scripts share `shared/common.sh` (config + helpers) — edit a default once, there. Clean up
between runs with `./shared/reset.sh`.

Tunables via env: `TRIALS` (20), `TIMEOUT` (180 s), `POLL` (0.2 s), `TARGET`,
`NAMESPACE`, `LOOM_NS`, `P4URL`, `P4C_IMAGE` (`p4lang/p4c`), `P4RT_DOCKER_IMAGE` (`p4lang/p4runtime-sh`).
The scripts report the convergence and baseline distributions; the *Interpreting the results* section
explains the recommended `full_ms` baseline and the ≤10× framing.


## Reset

```sh
./shared/reset.sh        # delete exp2 NFs + the baseline switch; keep the target
```

It also prints the command to restart the target if a switch crash left the data plane wedged. For a
full teardown (removes the target too):

```sh
kubectl delete -f 00-target.yaml
```

## Troubleshooting / notes

- **How the switch is crashed (and the PID-1 caveat).** `kubectl exec … kill -9 1` is a **no-op** here:
  a container's PID 1 ignores signals from its own namespace, so the switch keeps running. Instead, on
  kind the node is a local Docker container, so `inject_switch_crash` stops the switch through the node's
  CRI (`docker exec <node> crictl stop`), which kills only the switch while the driver stays up. It holds
  the switch down ~14s so the driver's ~10s health check is sure to observe the loss. If the node is not
  reachable that way (non-kind), it falls back to the in-container kill and, if that is absorbed (no
  `restartCount` bump), to a full pod restart — so a disruption always happens.
- **Switch crash recovery is control-plane only.** With the driver alive, losing the switch flips
  `Healthy=False` → the operator adds `p4target.loom.io/not-ready` and the target goes `Ready=False`
  (the divergence). When the switch restarts, the driver's `Capabilities()` succeeds again, so the taint
  clears and the target reports `Ready=True` — **but the driver does not re-push the pipeline** (no
  re-arbitration on stream loss; `cmd/main.go` logs `stream channel closed unexpectedly`), and the NF's
  `Ready` condition never changes. So `disrupt switch` shows the operator detecting and clearing a
  *target-level* divergence, while the **data plane is silently empty** afterward (the switch came back
  with `--no-p4`). The script can't see that without sending traffic — verifying it, or confirming a pod
  restart re-pushes the pipeline, is the honest way to report data-plane recovery.
- **Fast pod restart can be invisible.** If the Deployment recreates the pod and the new driver renews
  the Lease before the 40 s expiry, the operator may never mark divergence — `disrupt pod` then reports
  "detection: NONE", which legitimately means the system tolerated the blip.
- **Baseline image.** `baseline/bare-switch.yaml` uses `p4lang/behavioral-model:latest`; in `kind` make
  sure it is loaded/pullable (it is the same image the target uses, so `make kind-load-all` covers it).
- **P4Target CRD scope / Lease issues.** If the target never becomes Ready or `kubectl get p4target`
  errors, see the same troubleshooting section in `../resources/README.md` (cluster-scoped CRD +
  controller RESTMapper restart).

