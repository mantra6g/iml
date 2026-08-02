#!/usr/bin/env bash
# E2E test: create a second client and test logging feature through DPCS

set -euo pipefail

LOOM_NS=loom-system
DPCS_NODEPORT=30500
PF_PORT=19559
FAIL=0

log() { echo; echo "=== $* ==="; }

loom_ip() {
  kubectl get pod -l app="$1" -o jsonpath="{.items[0].metadata.annotations.k8s\.v1\.cni\.cncf\.io/network-status}" \
    | grep -o "10\.123\.[0-9.]*" | head -1
}

bmv2_pod() { kubectl get pods -n $LOOM_NS -o name | grep bmv2-target | head -1 | sed "s|pod/||"; }

hits() { kubectl logs -n $LOOM_NS "$(bmv2_pod)" -c bmv2-switch 2>/dev/null | grep -c "Function applied!" || true; }

transit() { kubectl logs -n $LOOM_NS "$(bmv2_pod)" -c bmv2-switch 2>/dev/null | grep -c "Processing packet" || true; }

table_dump() {
  kubectl exec -n $LOOM_NS "$(bmv2_pod)" -c bmv2-switch -- \
    sh -c "echo table_dump MyIngress.log_table | simple_switch_CLI" 2>/dev/null
}

table_must_hold() {
  local dump want got ip hexip
  sleep 2
  dump=$(table_dump)
  want=$#
  got=$(echo "$dump" | grep -c "Action entry: MyIngress.log" || true)
  if [ "$got" -ne "$want" ]; then
    echo "ERROR: the switch table holds $got entries, expected $want."
    echo "$dump" | grep -E "EXACT|Action entry" || true
    exit 1
  fi
  for ip in "$@"; do
    hexip=$(printf "%02x%02x%02x%02x" $(echo "$ip" | tr "." " "))
    if ! echo "$dump" | grep -qi "$hexip"; then
      echo "ERROR: entry for $ip ($hexip) is missing from the switch table."
      exit 1
    fi
  done
  echo "OK: table in sync ($want entries)"
}

dpcs_write() {
  if ! "$CLIENT_BIN" -p4info-addr "127.0.0.1:$PF_PORT" -dpcs-addr "$DPCS_ADDR" "$@"; then
    echo "retrying once (a stale driver connection is evicted on the first failure)..."
    "$CLIENT_BIN" -p4info-addr "127.0.0.1:$PF_PORT" -dpcs-addr "$DPCS_ADDR" "$@"
  fi
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
        echo "ERROR: ping succeeded but the switch only saw $((t1 - t0)) packets (expected >= $n) — traffic bypassed the NF."
        exit 1
      fi
      return 0
    fi
    sleep 3
  done
  echo "ERROR: ping from $deploy to $SERVER_IP failed"; exit 1
}

check() { # name, expected, actual
  if [ "$2" = "$3" ]; then echo "PASS: $1 (expected=$2, actual=$3)"
  else echo "FAIL: $1 (expected=$2, actual=$3)"; FAIL=1; fi
}
check_ge() {
  if [ "$3" -ge "$2" ]; then echo "PASS: $1 (expected>=$2, actual=$3)"
  else echo "FAIL: $1 (expected>=$2, actual=$3)"; FAIL=1; fi
}

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

log "Setup: DPCS test client and switch port-forward"
CLIENT_DIR="$(cd "$(dirname "$0")/dpcs-client" && pwd)"
CLIENT_BIN="$CLIENT_DIR/dpcs-client"
if [ ! -x "$CLIENT_BIN" ]; then
  echo "building dpcs-client..."
  (cd "$CLIENT_DIR" && GOWORK=off go build -o dpcs-client .)
fi

NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
DPCS_ADDR="$NODE_IP:$DPCS_NODEPORT"
echo "dpcs=$DPCS_ADDR"

kubectl port-forward -n $LOOM_NS "$(bmv2_pod)" "$PF_PORT:9559" >/dev/null 2>&1 &
PF_PID=$!
trap 'kill $PF_PID 2>/dev/null || true' EXIT
sleep 3

log "Reset: empty the log table via the DPCS"
for ip in "$C1_IP" "$C2_IP"; do
  dpcs_write -delete "$ip" >/dev/null 2>&1 || true
done
table_must_hold

log "PHASE 1: empty table — the logger must not match anything"
H0=$(hits)
ping_from web-client 3
ping_from web-client-2 3
sleep 3
H1=$(hits)
check "no hits with an empty table" 0 $((H1 - H0))

log "PHASE 2: entry for client1 only ($C1_IP), written via the DPCS"
dpcs_write -insert "$C1_IP" | tail -1
table_must_hold "$C1_IP"
H0=$(hits)
ping_from web-client 3
sleep 3
H1=$(hits)
check_ge "client1 traffic is matched" 3 $((H1 - H0))
ping_from web-client-2 3
sleep 3
H2=$(hits)
check "client2 traffic is NOT matched" 0 $((H2 - H1))

log "PHASE 3: entry for client2 as well ($C2_IP), written via the DPCS"
dpcs_write -insert "$C2_IP" | tail -1
table_must_hold "$C1_IP" "$C2_IP"
H0=$(hits)
ping_from web-client 3
ping_from web-client-2 3
sleep 3
H1=$(hits)
check_ge "both clients traffic is matched" 6 $((H1 - H0))

log "RESULT"
if [ "$FAIL" -eq 0 ]; then echo "ALL TESTS PASS"; else echo "THERE WERE FAILURES"; exit 1; fi
