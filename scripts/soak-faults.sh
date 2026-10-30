#!/bin/sh
# usage: soak-faults.sh <namespace> <weir-release> <quiet-seconds> <window-seconds> <every-seconds>
# Waits quiet-seconds, then injects one fault every every-seconds for
# window-seconds, rotating through five faults and waiting for recovery after
# each. The namespace holds a three-member MongoDB StatefulSet "mongo", a Pod
# "elasticsearch" and a weir-charts release; only those are touched.
# KUBECTL selects the kubectl binary.
NS=$1 RELEASE=$2 QUIET=$3 WINDOW=$4 EVERY=$5
K="${KUBECTL:-kubectl} -n $NS"
WEIR=$RELEASE-weir

log() { echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) $*"; }

mongo_eval() { $K exec "$1" -- mongosh --quiet --eval "$2" 2>/dev/null | tail -1; }

primary() {
  for i in 0 1 2; do
    [ "$(mongo_eval mongo-$i 'db.hello().isWritablePrimary')" = "true" ] && echo mongo-$i && return
  done
}

mongo_members() {
  for i in 0 1 2; do
    n=$(mongo_eval mongo-$i 'rs.status().members.filter(m => m.stateStr == "PRIMARY" || m.stateStr == "SECONDARY").length')
    case $n in [0-9]) echo "$n"; return;; esac
  done
  echo 0
}

mongo_healthy() { [ -n "$(primary)" ] && [ "$(mongo_members)" = "3" ]; }
es_ready() { [ "$($K get pod elasticsearch -o jsonpath='{.status.containerStatuses[0].ready}')" = "true" ]; }
weir_ready() {
  counts=$($K get deployment "$WEIR" -o jsonpath='{.spec.replicas} {.status.availableReplicas} {.status.updatedReplicas} {.status.replicas}')
  set -- $counts
  [ "$#" -eq 4 ] && [ "$1" = "$2" ] && [ "$1" = "$3" ] && [ "$1" = "$4" ]
}

# wait_until <check> <timeout-seconds>
wait_until() {
  deadline=$(( $(date +%s) + $2 ))
  while ! $1; do
    if [ "$(date +%s)" -ge "$deadline" ]; then
      log "TIMEOUT waiting for $1"
      return 1
    fi
    sleep 5
  done
}

stepdown() {
  p=$(primary)
  log "fault mongo-stepdown $p"
  mongo_eval "$p" 'rs.stepDown(60)' > /dev/null
  sleep 5
  wait_until mongo_healthy 600
}

mongo_kill() {
  p=$(primary)
  log "fault mongo-primary-kill $p"
  $K delete pod "$p" --grace-period=1 --wait=false > /dev/null
  sleep 20
  wait_until mongo_healthy 900
}

es_restart() {
  log "fault elasticsearch-restart"
  $K exec elasticsearch -- kill 1 > /dev/null 2>&1
  sleep 15
  wait_until es_ready 600
}

weir_rollout() {
  log "fault weir-rolling-restart"
  $K rollout restart "deployment/$WEIR" > /dev/null
  $K rollout status "deployment/$WEIR" --timeout=15m > /dev/null || log "TIMEOUT rollout"
  wait_until weir_ready 300
}

weir_kill() {
  pods=$($K get pods -l "app.kubernetes.io/instance=$RELEASE" -o name)
  p=$(echo "$pods" | sed -n "$(( $(date +%s) % $(echo "$pods" | wc -l) + 1 ))p")
  log "fault weir-pod-kill $p"
  $K delete "$p" --grace-period=1 --wait=false > /dev/null
  sleep 10
  wait_until weir_ready 600
}

log "quiet for ${QUIET}s"
sleep "$QUIET"
end=$(( $(date +%s) + WINDOW ))
n=0
while [ "$(date +%s)" -lt "$end" ]; do
  started=$(date +%s)
  case $(( n % 5 )) in
    0) stepdown ;;
    1) weir_rollout ;;
    2) es_restart ;;
    3) mongo_kill ;;
    4) weir_kill ;;
  esac
  log "recovered after $(( $(date +%s) - started ))s"
  n=$(( n + 1 ))
  rest=$(( EVERY - ($(date +%s) - started) ))
  [ "$rest" -gt 0 ] && sleep "$rest"
done
log "faults done: $n"
