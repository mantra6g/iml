#!/usr/bin/env bash
#
# Experiment 2 (A) — direct-gRPC baseline.
#
# Times the official p4runtime-shell client (p4lang/p4runtime-sh container)
# pushing logger.p4 onto a bare, driverless BMv2 switch — the floor that NF
# convergence is compared against. No bespoke measurement code: we only invoke
# the upstream client and time it.
#
# Usage: ./baseline.sh [N]      # N trials (default ${TRIALS})
#
# Each trial times two container runs:
#   full    — with --config: connect + arbitrate + push the pipeline.
#   startup — without --config: connect + arbitrate only (the fixed-cost reference).
# The push itself is full - startup. Results go to results/baseline.csv.

source "$(dirname "${BASH_SOURCE[0]}")/shared/common.sh"

# compile_pipeline -> results/pipeline/{logger.json,p4info.txt}, once.
# p4info is emitted as TEXT protobuf (what p4runtime-shell parses). Uses host p4c
# if present, else a throwaway p4lang/p4c container (p4c lives in the driver
# image, not on the dev host). Echoes the pipeline directory on stdout.
compile_pipeline() {
  local out="${OUTDIR}/pipeline"
  mkdir -p "${out}"
  if [[ -f "${out}/logger.json" && -f "${out}/p4info.txt" ]]; then echo "${out}"; return 0; fi
  echo "==> Compiling baseline pipeline from ${P4URL}" >&2
  curl -fsSL "${P4URL}" -o "${out}/logger.p4"
  if command -v p4c >/dev/null 2>&1; then
    p4c --target bmv2 --arch v1model --p4runtime-files "${out}/p4info.txt" --p4runtime-format text -o "${out}" "${out}/logger.p4" >&2
  elif command -v docker >/dev/null 2>&1; then
    echo "    host p4c not found; using ${P4C_IMAGE} container" >&2
    docker run --rm -v "${out}:/work" "${P4C_IMAGE}" \
      p4c --target bmv2 --arch v1model --p4runtime-files /work/p4info.txt --p4runtime-format text -o /work /work/logger.p4 >&2
  else
    echo "ERROR: need p4c or docker to compile the baseline pipeline." >&2; return 1
  fi
  echo "${out}"
}

# run_p4rt <pipeline-dir> <config|noconfig> — one p4runtime-sh container run.
# "config" pushes the pipeline; "noconfig" connects/arbitrates only (it errors
# because the switch has no pipeline yet, which is fine — we only time it). The
# dir is mounted at /cfg; --network host reaches the port-forwarded switch.
run_p4rt() {
  local pdir="$1" cfg=()
  [[ "$2" == "config" ]] && cfg=(--config "/cfg/p4info.txt,/cfg/logger.json")
  docker run --rm --network host -v "${pdir}:/cfg" "${P4RT_DOCKER_IMAGE}" \
    --grpc-addr localhost:9559 --device-id 0 --election-id 0,1 "${cfg[@]}" </dev/null
}

main() {
  local n="${1:-${TRIALS}}" pdir
  command -v docker >/dev/null 2>&1 || { echo "ERROR: docker is required (runs ${P4RT_DOCKER_IMAGE})." >&2; exit 1; }
  pdir="$(compile_pipeline)" || exit 1

  echo "==> Bringing up a bare BMv2 switch (no driver)..."
  kubectl apply -f "${HERE}/baseline/bare-switch.yaml" >/dev/null
  kubectl wait --for=condition=Ready pod/bmv2-baseline-switch -n "${NAMESPACE}" --timeout=120s

  echo "==> Port-forwarding 9559 -> bmv2-baseline-switch..."
  kubectl port-forward -n "${NAMESPACE}" pod/bmv2-baseline-switch 9559:9559 >/dev/null 2>&1 &
  local pf=$!
  # On exit, stop the port-forward. The bare switch is left running (re-applying
  # is idempotent and re-runs are faster); 'shared/reset.sh' removes it.
  trap 'kill "'"${pf}"'" 2>/dev/null || true' EXIT
  sleep 2

  local csv="${OUTDIR}/baseline.csv"
  echo "trial,full_ms,startup_ms" > "${csv}"
  echo "==> Timing ${n} pipeline pushes with ${P4RT_DOCKER_IMAGE}..."
  for ((i = 1; i <= n; i++)); do
    local s0 s1 f0 f1 startup full push
    s0="$(now)"; run_p4rt "${pdir}" noconfig >/dev/null 2>&1 || true; s1="$(now)"
    f0="$(now)"; run_p4rt "${pdir}" config   >/dev/null 2>&1 || true; f1="$(now)"
    startup="$(awk -v a="${s0}" -v b="${s1}" 'BEGIN{printf "%.1f",(b-a)*1000}')"
    full="$(awk -v a="${f0}" -v b="${f1}" 'BEGIN{printf "%.1f",(b-a)*1000}')"
    push="$(awk -v f="${full}" -v s="${startup}" 'BEGIN{d=f-s; if(d<0)d=0; printf "%.1f",d}')"
    echo "${i},${full},${startup}" >> "${csv}"
    printf "   trial %2d: full=%sms  startup=%sms  push~%sms\n" "${i}" "${full}" "${startup}" "${push}"
  done

  echo
  echo "==> Full standalone time (end-to-end):";          tail -n +2 "${csv}" | cut -d, -f2 | awk '{print $1/1000}' | stats
  echo "==> Startup reference (no pipeline pushed):";      tail -n +2 "${csv}" | cut -d, -f3 | awk '{print $1/1000}' | stats
  echo "==> CSV: ${csv}  (full_ms is the headline baseline; startup_ms for transparency)"
}

main "$@"
