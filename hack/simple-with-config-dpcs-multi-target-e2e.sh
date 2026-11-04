#!/usr/bin/env bash
# E2E test: one device_id mapped to two BMv2 targets through the DPCS

set -euo pipefail

LOOM_NS=loom-system
DPCS_NODEPORT=30500
TARGET_A=bmv2-target
TARGET_B=bmv2-target-2
EXAMPLE_DIR="$(cd "$(dirname "$0")/../examples/simple-with-config" && pwd)"
FAIL=0

log() { echo; echo "=== $* ==="; }

loom_ip() {
  kubectl get pod -l app="$1" -o jsonpath="{.items[0].metadata.annotations.k8s\.v1\.cni\.cncf\.io/network-status}" \
    | grep -o "10\.123\.[0-9.]*" | head -1
}

target_pod() { kubectl get pods -n $LOOM_NS -l "bmv2target.loom.io/name=$1" -o name | head -1 | sed "s|pod/||"; }

switch_log_count() {
  local target=$1 pattern=$2
  kubectl logs -n $LOOM_NS "$(target_pod "$target")" -c bmv2-switch 2>/dev/null | grep -c "$pattern" || true
}

hits() { echo $(( $(switch_log_count $TARGET_A "Function applied!") + $(switch_log_count $TARGET_B "Function applied!") )); }

transit() { echo $(( $(switch_log_count $TARGET_A "Processing packet") + $(switch_log_count $TARGET_B "Processing packet") )); }

switch_cli() {
  local target=$1 command=$2
  kubectl exec -n $LOOM_NS "$(target_pod "$target")" -c bmv2-switch -- sh -c "echo $command MyIngress.log_table | simple_switch_CLI" 2>/dev/null
}

table_must_hold() {
  local target=$1 dump want got ip hexip
  shift
  sleep 2
  dump=$(switch_cli "$target" table_dump)
  want=$#
  got=$(echo "$dump" | grep -c "Action entry: MyIngress.log" || true)
  if [ "$got" -ne "$want" ]; then
    echo "ERROR: $target holds $got entries, expected $want."
    echo "$dump" | grep -E "EXACT|Action entry" || true
    exit 1
  fi
  for ip in "$@"; do
    hexip=$(printf "%02x%02x%02x%02x" $(echo "$ip" | tr "." " "))
    if ! echo "$dump" | grep -qi "$hexip"; then
      echo "ERROR: entry for $ip ($hexip) is missing from $target."
      exit 1
    fi
  done
  echo "OK: $target in sync ($want entries)"
}

dpcs_client() { "$CLIENT_BIN" -dpcs-addr "$DPCS_ADDR" "$@"; }

restart_dpcs() {
  kubectl delete pod -n $LOOM_NS -l app=dpcs >/dev/null
  kubectl rollout status deploy/loom-dpcs -n $LOOM_NS --timeout=120s >/dev/null
}

dpcs_reload_count() { kubectl logs -n $LOOM_NS deploy/loom-dpcs 2>/dev/null | grep -c "device-map reloaded" || true; }

wait_for_device_map_reload() {
  local reloads_before=$1
  for i in $(seq 1 48); do
    dpcs_client -read >/dev/null 2>&1 || true
    [ "$(dpcs_reload_count)" -gt "$reloads_before" ] && return 0
    sleep 5
  done
  echo "ERROR: the DPCS did not reload the device-map"; return 1
}

wait_for_target_ready() {
  local target=$1
  for i in $(seq 1 60); do
    if [ "$(kubectl get p4target "$target" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)" = "True" ]; then
      return 0
    fi
    sleep 5
  done
  echo "ERROR: P4Target $target did not become ready"; return 1
}

wait_for_logger_nfs() {
  local expected=$1 running
  for i in $(seq 1 60); do
    running=$(kubectl get nf -l nf=logger -o jsonpath='{range .items[*]}{.status.phase}{"\n"}{end}' | grep -c Running || true)
    [ "$running" -eq "$expected" ] && return 0
    sleep 5
  done
  echo "ERROR: expected $expected running logger NFs"; kubectl get nf -l nf=logger; return 1
}

wait_for_pod_ready() {
  local target=$1 old_pod=$2 pod ready
  for i in $(seq 1 60); do
    pod=$(target_pod "$target")
    if [ -n "$pod" ] && [ "$pod" != "$old_pod" ]; then
      ready=$(kubectl get pod -n $LOOM_NS "$pod" -o jsonpath='{.status.containerStatuses[*].ready}' 2>/dev/null || true)
      [ "$ready" = "true true" ] && return 0
    fi
    sleep 5
  done
  echo "ERROR: replacement pod of $target did not become ready"; return 1
}

ping_from() {
  local deploy=$1 n=$2
  local t0 t1
  t0=$(transit)
  for i in $(seq 1 20); do
    if kubectl exec "deploy/$deploy" -- ping -c "$n" -W 2 "$SERVER_IP" >/dev/null 2>&1; then
      sleep 2
      t1=$(transit)
      if [ $((t1 - t0)) -lt "$n" ]; then
        echo "ERROR: ping succeeded but the switches only saw $((t1 - t0)) packets (expected >= $n) — traffic bypassed the NF."
        exit 1
      fi
      return 0
    fi
    sleep 3
  done
  echo "ERROR: ping from $deploy to $SERVER_IP failed"; exit 1
}

check() {
  local name=$1 expected=$2 actual=$3
  if [ "$expected" = "$actual" ]; then echo "PASS: $name (expected=$expected, actual=$actual)"
  else echo "FAIL: $name (expected=$expected, actual=$actual)"; FAIL=1; fi
}
check_ge() {
  local name=$1 expected=$2 actual=$3
  if [ "$actual" -ge "$expected" ]; then echo "PASS: $name (expected>=$expected, actual=$actual)"
  else echo "FAIL: $name (expected>=$expected, actual=$actual)"; FAIL=1; fi
}

cleanup() {
  local status=$?
  if [ "$status" -ne 0 ] || [ "$FAIL" -ne 0 ]; then
    log "DPCS log (last 40 lines)"
    kubectl logs -n $LOOM_NS deploy/loom-dpcs --tail=40 2>/dev/null | cut -c1-300 || true
  fi
  log "Teardown: restore the single-target example"
  if [ -n "${DPCS_ADDR:-}" ]; then
    for ip in "${C1_IP:-}" "${C2_IP:-}"; do
      [ -n "$ip" ] && { dpcs_client -delete "$ip" >/dev/null 2>&1 || true; }
    done
  fi
  kubectl delete -f "$EXAMPLE_DIR/replicated-logger.yml" --ignore-not-found >/dev/null 2>&1 || true
  kubectl delete p4target $TARGET_B --ignore-not-found >/dev/null 2>&1 || true
  kubectl apply -f "$EXAMPLE_DIR/simple-with-config.yml" >/dev/null 2>&1 || true
  restart_dpcs || true
  wait_for_logger_nfs 1 || true
}
trap cleanup EXIT

log "Setup: second client pod (web-client-2, registered to the same Application)"
kubectl apply -f - <<APPLY >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-client-2
  namespace: default
spec:
  selector:
    matchLabels:
      app: web-client-2
  template:
    metadata:
      labels:
        app: web-client-2
      annotations:
        k8s.v1.cni.cncf.io/networks: |
          [
            {
              "name": "loom-cni",
              "namespace": "loom-system",
              "cni-args": {
                "app_type": "application",
                "app_name": "web-client",
                "app_namespace": "default"
              }
            }
          ]
    spec:
      containers:
        - name: web-client
          image: alpine:3.20
          command: ["/bin/sh", "-c", "trap : TERM INT; sleep infinity & wait"]
APPLY
kubectl wait --for=condition=Ready pod -l app=web-client-2 --timeout=120s >/dev/null

SERVER_IP=$(loom_ip web-server)
C1_IP=$(loom_ip web-client)
C2_IP=$(loom_ip web-client-2)
echo "server=$SERVER_IP  client1=$C1_IP  client2=$C2_IP"
[ -n "$SERVER_IP" ] && [ -n "$C1_IP" ] && [ -n "$C2_IP" ] && [ "$C1_IP" != "$C2_IP" ]

log "Setup: DPCS test client"
CLIENT_DIR="$(cd "$(dirname "$0")/dpcs-client" && pwd)"
CLIENT_BIN="$CLIENT_DIR/dpcs-client"
if [ ! -x "$CLIENT_BIN" ]; then
  echo "building dpcs-client..."
  (cd "$CLIENT_DIR" && GOWORK=off go build -o dpcs-client .)
fi

NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
DPCS_ADDR="$NODE_IP:$DPCS_NODEPORT"
echo "dpcs=$DPCS_ADDR"

log "Setup: replicated logger on two BMv2 targets, one device_id"
RELOADS_BEFORE=$(dpcs_reload_count)
kubectl delete nf pkt-logger --ignore-not-found >/dev/null
kubectl apply -f "$EXAMPLE_DIR/replicated-logger.yml" >/dev/null
wait_for_target_ready $TARGET_B
wait_for_logger_nfs 2
kubectl get nf -l nf=logger -o custom-columns='NF:.metadata.name,TARGET:.spec.targetName,PHASE:.status.phase'
wait_for_device_map_reload "$RELOADS_BEFORE"
echo "OK: the DPCS picked up the two-target device-map without a restart"

log "Reset: empty both tables via the DPCS"
for ip in "$C1_IP" "$C2_IP"; do
  dpcs_client -delete "$ip" >/dev/null 2>&1 || true
done
table_must_hold $TARGET_A
table_must_hold $TARGET_B

log "PHASE 1: one INSERT via the DPCS lands on both targets"
dpcs_client -insert "$C1_IP" | tail -1
table_must_hold $TARGET_A "$C1_IP"
table_must_hold $TARGET_B "$C1_IP"

log "PHASE 2: traffic is matched on whichever target carries the service chain"
H0=$(hits)
ping_from web-client 3
sleep 3
H1=$(hits)
check_ge "client1 traffic is matched" 3 $((H1 - H0))
ping_from web-client-2 3
sleep 3
H2=$(hits)
check "client2 traffic is NOT matched" 0 $((H2 - H1))

log "PHASE 3: a target goes down — the write still reaches the others and the client gets the error"
OLD_POD=$(target_pod $TARGET_B)
kubectl delete pod -n $LOOM_NS "$OLD_POD" --grace-period=0 --force >/dev/null 2>&1
echo "killed $OLD_POD"
sleep 5
if OUT=$(dpcs_client -insert "$C2_IP" 2>&1); then
  echo "FAIL: INSERT succeeded although $TARGET_B is down: $OUT"; FAIL=1
else
  echo "$OUT" | tail -1
  if echo "$OUT" | grep -q -E "Unavailable|DeadlineExceeded"; then echo "PASS: the client received the failing target's error"
  else echo "FAIL: unexpected error: $OUT"; FAIL=1; fi
fi
table_must_hold $TARGET_A "$C1_IP" "$C2_IP"

log "PHASE 4: the target comes back empty — Read still serves the first target"
wait_for_pod_ready $TARGET_B "$OLD_POD"
wait_for_target_ready $TARGET_B
echo "new pod: $(target_pod $TARGET_B)"
table_must_hold $TARGET_B
READ_OUT=$(dpcs_client -read)
echo "$READ_OUT" | grep -E "entry:|Read OK" || true
check "read returns the first target's entries although the second is empty" 2 "$(echo "$READ_OUT" | grep -c 'entry:' || true)"

log "PHASE 5: re-INSERT repairs the restarted replica (existing entries are retried as MODIFY)"
for i in $(seq 1 36); do
  if OUT=$(dpcs_client -insert "$C1_IP,$C2_IP" 2>&1); then break; fi
  sleep 5
done
echo "$OUT" | tail -1
table_must_hold $TARGET_A "$C1_IP" "$C2_IP"
table_must_hold $TARGET_B "$C1_IP" "$C2_IP"

log "PHASE 6: unknown device_id is rejected with NotFound"
OUT=$("$CLIENT_BIN" -dpcs-addr "$DPCS_ADDR" -device-id 99 -read 2>&1 || true)
if echo "$OUT" | grep -q "NotFound"; then
  echo "PASS: rejected with NotFound"
else
  echo "FAIL: did not return NotFound: $OUT"; FAIL=1
fi

log "RESULT"
if [ "$FAIL" -eq 0 ]; then echo "ALL TESTS PASS"; else echo "THERE WERE FAILURES"; exit 1; fi
