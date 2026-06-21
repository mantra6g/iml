# Experiment 3 — End-to-End Service Chain (RQ3)

Validates the RQ3 claim from `../resources/Proposal.tex` (§Proposed Evaluation):

> **End-to-end service chain (RQ3).** Deploy a service chain of SR-unaware NFs through the plug-in's
> standard CRDs and pass traffic across it. *Success:* traffic is steered through the deployed NFs via
> the plug-in, and a change in chain membership (e.g. removing an NF) is reflected within a small
> number of reconciler cycles.

This experiment **reuses `examples/demo/` as-is** (web + streaming chains, 4 BMv2 targets, the
`qos` / `traffic-manager` / `load-balancer` NFs, iperf3 traffic via proxies/NodePorts). No IML core
code is modified — the experiment only deploys the demo and observes existing signals.

So there are two halves:

- **(A) Traffic steered** — deploy the chain through the standard CRDs (`BMv2Target`,
  `NetworkFunction`, `Application`, `ServiceChain`) and show traffic crosses the SR-unaware NFs.
- **(B) Membership change reflected** — remove an NF from a chain and show the daemon's reconcile loop
  observes the change within a small number of cycles.

## How it works

### Steering (what is checked, Part A)

```
local-streaming-proxy ──► streaming-lb ──► qos ──► traffic-manager ──► streaming-server
        (Application)        └──────── ServiceChain "streaming-downlink" ────────┘
```

The daemon resolves each `spec.functions` stage to a Ready NF, takes its `status.assignedIP`, and
programs an **SRv6 route** whose segment list is the NF IPs. `exp3_steer.sh` verifies steering two ways:

1. **Route proof** — the route segments contain each stage's NF IP (deterministic).
2. **Connectivity proof** — real traffic from a healthy source app pod to the chain's dest app is
   forwarded end-to-end (`ping6` run *inside* the pod). Since the SRv6 route forces the packet through
   the NF segment(s), a successful exchange proves the NF is on the live path.

### Membership change (what is observed, Part B)

```
kubectl patch servicechain streaming-downlink  (remove /spec/functions/1, the qos stage)
   └─ generation bumps -> daemon "Reconciling ServiceChain" for the chain     ── detection
        └─ "found matching NFs" now lists 2 stages (qos gone)                  ── new membership
             └─ RouteAdd on the existing route -> "file exists"               ── data plane stale (finding)
```

`exp3_membership.sh` removes the qos stage, then summarises the daemon's reaction from its logs:
how many times the chain reconciled, the new (smaller) NF set, and the stale-route outcome — finally
restoring full membership.

## Files

| File | Purpose |
|------|---------|
| `shared/common.sh` | Shared config + helpers (sourced, not run): `DEMO_DIR`, `CHAIN`, timing, route/log queries. |
| `shared/reset.sh` | Deletes everything the demo deploys (chains, NFs, configs, apps, deployments, services, targets) and restarts the daemon. |
| `exp3_deploy.sh` | Applies the `examples/demo` manifests in order and waits until SRv6 routes are programmed. |
| `exp3_steer.sh` | Part A — route proof + end-to-end connectivity proof that traffic is steered through the chain. |
| `exp3_membership.sh` | Part B — removes a stage and summarises the daemon's reaction (reconcile, new membership, stale route) from its logs. |

The chain manifests themselves live in `examples/demo/` (referenced via `DEMO_DIR`); they are not
duplicated here.

**Extra tooling** (beyond a running IML cluster + `kubectl`): **Docker** (kind node access for route
inspection, and to build/load the demo images). The connectivity proof runs `ping6` *inside* the proxy
pod (the `swiss-army-knife`-based proxy image already ships `ping6`/`iperf3`/`curl`/`nc`), so no host
network tools are required.

## Prerequisites

The demo ships **custom images** (proxy, web/streaming servers, lb-controller, perf BMv2). Build and
load them, plus the standard IML images, once:

```sh
# IML itself (operator, daemon, cni, bmv2 driver):
make docker-build-all && make kind-create && make kind-load-all && make install
# Demo workload images:
make -C examples/demo docker-build-all kind-load-all
```

Optional but recommended for Part B, so the `found matching NFs` line is emitted (deploy-time arg, not
a code change):

```sh
kubectl -n loom-system patch ds/loom-daemon --type=json \
  -p '[{"op":"add","path":"/spec/template/spec/containers/0/args","value":["-zap-log-level=debug"]}]'
```

## Run by hand

### Deploy (Part A + B share this)

```sh
DEMO=../../../../examples/demo
# Apply the demo manifests in dependency order (targets first, chains last).
for f in targets.yml apps.yml nfs.yml deployments.yml nfchains.yml nodeports.yml; do
  kubectl apply -f "$DEMO/$f"
done
kubectl wait --for=condition=Ready nf --all -n default --timeout=120s   # wait for the 4 NFs to come up
kubectl get servicechain -n default                                     # the 4 chains should exist
```

### (A) Steering — route proof: NF IPs appear as SRv6 segments

```sh
# The daemon (hostNetwork) programs SRv6 routes in the node's netns; on kind the
# node is a Docker container, so we read its routing table directly.
NODE=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')        # the kind node / Docker container name
docker exec "$NODE" ip route show table all | grep -iE 'seg6 mode encap' # chain routes: each segment is an NF IP
# List the three NFs' assigned IPs so you can match them against the segments above.
kubectl get nf streaming-lb qos traffic-manager -n default -o jsonpath='{range .items[*]}{.metadata.name}={.status.assignedIP}{"\n"}{end}'
```

### (A) Steering — connectivity proof: real traffic through the NF

```sh
# Use a healthy source (a proxy pod); the streaming-uplink chain steers its
# traffic through streaming-lb. (The 3-NF downlink source is a media server that
# crashloops here due to the Git-LFS *.mkv stubs.)
SRC=$(kubectl get pod -n default -l app=local-streaming-proxy   -o jsonpath='{.items[0].metadata.name}')  # source pod
DST=$(kubectl get pod -n default -l app=dummy-streaming-server  -o jsonpath='{.items[0].metadata.name}')  # destination pod
# Pull the destination's loom-cni (iml0) IPv6 address out of its multus network-status annotation.
DIP=$(kubectl get pod -n default "$DST" -o jsonpath='{.metadata.annotations.k8s\.v1\.cni\.cncf\.io/network-status}' | grep -oE 'fd00:[0-9a-f:]+' | head -1)
# Send traffic from the source to that IP; 0% loss proves it was steered through the NF and forwarded.
kubectl exec -n default "$SRC" -c proxy -- ping6 -c 5 -W 2 "$DIP"

# Best-effort extra: read the NF's P4 registers. (The demo NFs define P4 *registers*,
# not P4 *counters*, so /api/counters is always empty — don't rely on it.)
TGT=$(kubectl get nf streaming-lb -n default -o jsonpath='{.spec.targetName}')                            # NF's target switch
POD=$(kubectl get pod -n loom-system -l infra.loom.io/targetDeployment=$TGT -o jsonpath='{.items[0].metadata.name}')  # its driver pod
kubectl exec -n loom-system "$POD" -c bmv2-driver -- wget -qO- http://localhost:8080/api/registers; echo
```

### (B) Membership change: remove the qos stage, watch the daemon react

```sh
SINCE=$(date -u +%Y-%m-%dT%H:%M:%SZ)                          # timestamp, to read only new daemon logs
# Remove functions[1] (the qos stage) from the chain — i.e. drop an NF from its membership.
kubectl patch servicechain streaming-downlink -n default --type=json -p '[{"op":"remove","path":"/spec/functions/1"}]'
# Watch the daemon react: it reconciles, re-resolves a smaller NF set, and fails to update the route.
kubectl logs -n loom-system -l app.kubernetes.io/name=loom-daemon -c daemon --since-time="$SINCE" | grep -E "Reconciling ServiceChain|found matching NFs|file exists"
#   expect: a reconcile for streaming-downlink within ~1 cycle; "found matching NFs" now 2 stages;
#           "failed to add SRv6 route ... file exists" -> the route is NOT updated (the finding).
docker exec "$NODE" ip route show table all | grep -i seg6   # qos IP still present in the route (stale)
kubectl apply -f "$DEMO/nfchains.yml"                         # restore the chain to full membership
```

## Scripts

```sh
./shared/reset.sh            # clean slate (deletes demo + restarts daemon) — run before re-deploys
./exp3_deploy.sh             # apply the demo, wait for the NFs to be Ready
./exp3_steer.sh              # Part A — route proof + connectivity proof
./exp3_membership.sh         # Part B — remove the qos stage, show the daemon's reaction

# full clean cycle:
./shared/reset.sh && ./exp3_deploy.sh && ./exp3_steer.sh && ./exp3_membership.sh
```

All scripts source `shared/common.sh`, which holds the settings as plain values (`NAMESPACE`,
`LOOM_NS`, `CHAIN`, `REMOVE_STAGE`, `TIMEOUT`, paths). To change a setting, edit it there — the
scripts use no environment-variable overrides.

## Reset

```sh
./shared/reset.sh   # delete chains, NFs, apps, deployments, services, targets + restart daemon
```

`reset.sh` **restarts the daemon** as well as deleting resources — this is required, not cosmetic:
the dataplane's app-subnet teardown is a no-op, so without the restart a subsequent deploy reuses the
stale in-memory subnet cache and leaves source apps with `status.subnets=<none>` (no routes). The
reliable re-run cycle is therefore `./shared/reset.sh && ./exp3_deploy.sh`.

## Troubleshooting / notes

- **No SRv6 routes after deploy.** Routes only appear once every stage of a chain has a Ready NF with
  an `assignedIP` *and* the source/destination Applications have subnets on the node. Check
  `kubectl get nf -n default` (all Ready?) and the daemon log
  (`kubectl logs -n loom-system -l app.kubernetes.io/name=loom-daemon -c daemon`) for
  `No available routes ... skipping`.
- **`streaming-server` / `web-server` pods `CrashLoopBackOff` (ffmpeg "Invalid data … EBML header").**
  The demo's `fns1.mkv` / `fns2.mkv` are Git-LFS objects; without `git lfs pull` they are 133-byte
  pointer stubs and ffmpeg cannot read them. This does **not** block Parts A/B: CNI ADD runs at sandbox
  creation (before ffmpeg), so the source app still gets a subnet and the chain routes still program —
  only live iperf3 payload through those servers is affected. Install git-lfs and `git lfs pull` if you
  need the servers actually serving.
- **P4Target CRD scope / target never Ready.** Same root cause and fix as Experiments 1 & 2 — see the
  troubleshooting section in `../exp1/README.md`.
