#!/usr/bin/env bash
# E2E test for examples/load-balancer through DPCS: checks the switch tables
# against the pods, sends traffic, and scales web-server 1 -> 3 -> 1.
# Usage: hack/load-balancer-dpcs-e2e.sh <output-dir>

set -uo pipefail

OUT=${1:?usage: hack/load-balancer-dpcs-e2e.sh <output-dir>}
LOOM_NS=loom-system
SYNC_TIMEOUT=120
REQUESTS=30
FAIL=0
mkdir -p "$OUT"

log() { echo; echo "=== $* ==="; }
check() {
  if [ "$2" = "$3" ]; then echo "PASS: $1 (expected=$2, actual=$3)"
  else echo "FAIL: $1 (expected=$2, actual=$3)"; FAIL=1; fi
}

bmv2_pod() { kubectl get pods -n $LOOM_NS -o name | grep bmv2-target | head -1; }

# name ipv4 ipv6, one running pod per line
pod_addresses() {
  kubectl get pod -l app="$1" --field-selector=status.phase=Running -o json | jq -r '
    .items[] | select(.metadata.deletionTimestamp == null)
    | (.metadata.annotations["k8s.v1.cni.cncf.io/network-status"] // "[]" | fromjson
       | map(select(.name | test("loom-cni"))) | .[0].ips // []) as $ips
    | [.metadata.name, ($ips | map(select(contains(":") | not)) | .[0] // "-"),
       ($ips | map(select(contains(":"))) | .[0] // "-")] | @tsv'
}

ip_hex() { python3 -c 'import ipaddress,sys; print(ipaddress.ip_address(sys.argv[1]).packed.hex())' "$1"; }

# entry lines: "entry <key-hex> <action> <params>", default line: "default <action> <params>"
table_state() {
  kubectl exec -n $LOOM_NS "$(bmv2_pod)" -c bmv2-switch -- \
    sh -c "echo table_dump MyIngress.$1 | simple_switch_CLI" 2>/dev/null | awk '
      /Dumping entry/ { kind = "entry"; key = "" }
      /Dumping default entry/ { kind = "default"; key = "" }
      /EXACT/ { key = $NF }
      /Action entry:/ {
        action = $3; params = ($5 == "" ? "-" : $5)
        if (kind == "entry") print "entry", key, action, params
        else print "default", action, params
      }'
}

sorted() { tr ' ' '\n' | grep -v '^$' | sort | tr '\n' ' ' | sed 's/ $//'; }

verify_family() { # phase, family (ipv4|ipv6), dummy addresses, server addresses
  local phase=$1 family=$2 dummies=$3 servers=$4
  local count state want got
  count=$(echo "$servers" | wc -w)

  state=$(table_state "dummy_pod_${family}_table"); echo "$state" > "$OUT/$phase-dummy_pod_${family}_table.txt"
  want=$(for a in $dummies; do ip_hex "$a"; done | sorted)
  got=$(echo "$state" | awk '$1=="entry" && $3=="MyIngress.mark_to_load_balance" {print $2}' | sorted)
  check "$phase dummy_pod_${family}_table keys" "$want" "$got"

  state=$(table_state "lb_pod_${family}_table"); echo "$state" > "$OUT/$phase-lb_pod_${family}_table.txt"
  want=$(for a in $servers; do ip_hex "$a"; done | sorted)
  got=$(echo "$state" | awk '$1=="entry" && $3=="MyIngress.mark_to_return" {print $2}' | sorted)
  check "$phase lb_pod_${family}_table keys" "$want" "$got"

  state=$(table_state "ecmp_nhop_${family}"); echo "$state" > "$OUT/$phase-ecmp_nhop_${family}.txt"
  got=$(echo "$state" | awk -v a="MyIngress.set_nhop_${family}" '$1=="entry" && $3==a {print $4}' | sorted)
  check "$phase ecmp_nhop_${family} next hops" "$want" "$got"
  want=$(for i in $(seq 0 $((count - 1))); do printf '%08x\n' "$i"; done | sorted)
  got=$(echo "$state" | awk '$1=="entry" {print $2}' | sorted)
  check "$phase ecmp_nhop_${family} selectors" "$want" "$got"

  state=$(table_state "ecmp_group_${family}"); echo "$state" > "$OUT/$phase-ecmp_group_${family}.txt"
  got=$(echo "$state" | awk '$1=="default" {print $2, $3}')
  [ "${got##* }" != "-" ] && got="${got% *} $((16#${got##* }))"
  check "$phase ecmp_group_${family} default action" "MyIngress.set_ecmp_select_${family} $count" "$got"
  check "$phase ecmp_group_${family} entries" 0 "$(echo "$state" | grep -c '^entry' || true)"
}

ecmp_count() {
  local params
  params=$(table_state ecmp_group_ipv6 | awk '$1=="default" {print $3}')
  [ -n "$params" ] && [ "$params" != "-" ] && echo $((16#$params)) || echo 0
}

wait_for_sync() { # expected server count; prints seconds until the switch follows
  local start now
  start=$(date +%s)
  while :; do
    now=$(date +%s)
    if [ "$(ecmp_count)" = "$1" ] && [ "$(table_state ecmp_nhop_ipv6 | grep -c '^entry')" = "$1" ]; then
      echo "switch followed the pod change in $((now - start))s"; return 0
    fi
    if [ $((now - start)) -ge $SYNC_TIMEOUT ]; then
      echo "switch did NOT follow the pod change within ${SYNC_TIMEOUT}s"; return 1
    fi
    sleep 2
  done
}

send_requests() { # phase, family, url -> prints "<ok-count> <responders>"
  local phase=$1 family=$2 url=$3 client replies
  client=$(kubectl get pod -l app=web-client --field-selector=status.phase=Running -o name | head -1)
  replies=$(kubectl exec "$client" -- sh -c "for i in \$(seq 1 $REQUESTS); do wget -qO- -T 2 '$url' 2>/dev/null; echo; done" | grep -o 'web-server-[a-z0-9-]*' || true)
  echo "$replies" | grep -v '^$' | sort | uniq -c > "$OUT/$phase-http-$family.txt"
  echo "$(echo "$replies" | grep -c . || true) $(echo "$replies" | sort -u | sorted)"
}

run_phase() { # phase name
  local phase=$1 dummies servers result ok responders
  dummies=$(pod_addresses dummy-web-server)
  servers=$(pod_addresses web-server)
  echo "$dummies" > "$OUT/$phase-pods-dummy.txt"; echo "$servers" > "$OUT/$phase-pods-server.txt"
  echo "dummy pods:"; echo "$dummies"; echo "server pods:"; echo "$servers"

  verify_family "$phase" ipv4 "$(echo "$dummies" | awk '{print $2}' | sorted)" "$(echo "$servers" | awk '{print $2}' | sorted)"
  verify_family "$phase" ipv6 "$(echo "$dummies" | awk '{print $3}' | sorted)" "$(echo "$servers" | awk '{print $3}' | sorted)"

  result=$(send_requests "$phase" ipv6 "http://[$(echo "$dummies" | awk 'NR==1 {print $3}')]/")
  ok=${result%% *}; responders=${result#* }
  check "$phase ipv6 http replies" "$REQUESTS" "$ok"
  check "$phase ipv6 http responders outside web-server pods" "" \
    "$(comm -13 <(echo "$servers" | awk '{print $1}' | sort) <(echo "$responders" | tr ' ' '\n' | grep . | sort) | sorted)"

  result=$(send_requests "$phase" ipv4 "http://$(echo "$dummies" | awk 'NR==1 {print $2}')/")
  echo "INFO: $phase ipv4 http replies ${result%% *}/$REQUESTS (not asserted)"
}

scale_servers() {
  kubectl scale deploy/web-server --replicas="$1" >/dev/null
  kubectl rollout status deploy/web-server --timeout=180s >/dev/null
  for _ in $(seq 1 60); do
    [ "$(pod_addresses web-server | awk '$2 != "-"' | wc -l)" = "$1" ] && break
    sleep 2
  done
}

{
  log "PHASE 1: one web-server replica"
  run_phase phase1

  log "PHASE 2: scale web-server to 3"
  scale_servers 3
  wait_for_sync 3 || FAIL=1
  run_phase phase2

  log "PHASE 3: scale web-server back to 1"
  scale_servers 1
  wait_for_sync 1 || FAIL=1
  run_phase phase3

  echo
  if [ $FAIL -eq 0 ]; then echo "ALL CHECKS PASS"; else echo "SOME CHECKS FAILED"; fi
} 2>&1 | tee "$OUT/result.txt"

grep -q "ALL CHECKS PASS" "$OUT/result.txt"
