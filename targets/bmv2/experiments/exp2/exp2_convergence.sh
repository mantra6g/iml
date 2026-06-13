#!/usr/bin/env bash
#
# Experiment 2 (A) — NF convergence timing.
#
# Times each NF from `kubectl apply` to its Ready condition, split into operator
# scheduling (spec.targetName set) and driver configure (Ready=True). Repeats N
# times, deleting the NF between trials. Results go to results/converge.csv.
#
# Usage: ./convergence.sh [N]   # N trials (default ${TRIALS})

source "$(dirname "${BASH_SOURCE[0]}")/shared/common.sh"

main() {
  local n="${1:-${TRIALS}}"
  require_target_ready

  local csv="${OUTDIR}/converge.csv"
  echo "trial,total_s,schedule_s,configure_s" > "${csv}"
  echo "==> Running ${n} convergence trials (create -> Ready)..."

  for ((i = 1; i <= n; i++)); do
    kubectl delete nf "${NF}" -n "${NAMESPACE}" --ignore-not-found >/dev/null 2>&1 || true

    local t0 t_sched t_ready total sched conf bound ready deadline
    t0="$(now)"
    kubectl apply -f "${HERE}/nf-lifecycle.yaml" >/dev/null

    # Wait until the operator binds the NF to a target (spec.targetName is set).
    bound=0; deadline=$(( SECONDS + TIMEOUT ))
    while (( SECONDS < deadline )); do [[ -n "$(nf_target)" ]] && { bound=1; break; }; sleep "${POLL}"; done
    (( bound )) || { echo "   trial ${i}: TIMEOUT waiting for scheduling" >&2; continue; }
    t_sched="$(now)"

    # Wait until the driver reports the NF Ready.
    ready=0; deadline=$(( SECONDS + TIMEOUT ))
    while (( SECONDS < deadline )); do [[ "$(nf_ready)" == "True" ]] && { ready=1; break; }; sleep "${POLL}"; done
    (( ready )) || { echo "   trial ${i}: TIMEOUT waiting for Ready" >&2; continue; }
    t_ready="$(now)"

    total="$(since "${t0}" "${t_ready}")"
    sched="$(since "${t0}" "${t_sched}")"
    conf="$(since "${t_sched}" "${t_ready}")"
    echo "${i},${total},${sched},${conf}" >> "${csv}"
    printf "   trial %2d: total=%ss  (schedule=%ss  configure=%ss)\n" "${i}" "${total}" "${sched}" "${conf}"
  done

  kubectl delete nf "${NF}" -n "${NAMESPACE}" --ignore-not-found >/dev/null 2>&1 || true

  echo
  echo "==> total create->Ready:"; tail -n +2 "${csv}" | cut -d, -f2 | stats
  echo "==> of which scheduling:"; tail -n +2 "${csv}" | cut -d, -f3 | stats
  echo "==> of which configure:";  tail -n +2 "${csv}" | cut -d, -f4 | stats
  echo "==> CSV: ${csv}"
}

main "$@"
