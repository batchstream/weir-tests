#!/bin/sh
# usage: rollout-scenarios.sh <namespace> <release> <chart> <values> <digest> <other-digest>
# Upgrades a weir-charts release with real Helm upgrades under load and fails if
# any caller saw an error, at each replica count in REPLICAS. Each round runs
# CLIENTS weir-soak processes with -no-errors in SOAK_POD and goes through the
# chart upgrade from FROM_CHART if set, version upgrades, scaling, a replica
# that starts late, a deleted Pod and an image that cannot be pulled.
#
# The namespace already holds SOAK_POD and the backends. SOAK_ARGS holds the
# soak's addresses and load; it may use double quotes, not single ones. A Pod
# that cannot be scheduled ends the run with the release scaled to zero, so that
# nothing stays Pending for a cluster autoscaler to act on.
NS=$1 RELEASE=$2 CHART=$3 VALUES=$4 DIGEST=$5 OTHER=$6
[ -n "$OTHER" ] && [ -f "$VALUES" ] || { sed -n 2p "$0" >&2; exit 2; }
[ -n "$SOAK_ARGS" ] || { echo "SOAK_ARGS names the soak's addresses and load" >&2; exit 2; }
K="${KUBECTL:-kubectl} -n $NS"
H="${HELM:-helm} -n $NS"
POD=${SOAK_POD:-client}
BIN=${SOAK_BIN:-/work/weir-soak}
DIR=${SOAK_DIR:-/work}
CLIENTS=${CLIENTS:-2}
REPLICAS=${REPLICAS:-1 3}
LATE_START=${LATE_START:-20}
LATE_START_IMAGE=${LATE_START_IMAGE:-busybox:1.37.0}
PAUSE=${PAUSE:-30}
UNPULLABLE=sha256:0000000000000000000000000000000000000000000000000000000000000000
SELECTOR=app.kubernetes.io/instance=$RELEASE
RUNNING=

log() { echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) $*"; }

fail() {
  log "FAILED: $*"
  stop_clients
  exit 1
}

deployment() { $K get deployment -l "$SELECTOR" -o name; }

# helm_set <chart> <replicas> <digest> starts an upgrade without waiting for it.
helm_set() {
  $H upgrade --install "$RELEASE" "$1" -f "$VALUES" --set "replicaCount=$2" --set "image.digest=$3" >/dev/null ||
    fail "helm upgrade to replicas=$2 digest=$3"
}

check_schedulable() {
  reasons=$($K get pods -l "$SELECTOR" -o jsonpath='{range .items[*]}{.status.conditions[?(@.type=="PodScheduled")].reason}{"\n"}{end}')
  case $reasons in
    *Unschedulable*)
      $K scale "$(deployment)" --replicas=0 >/dev/null
      fail "a Weir Pod cannot be scheduled" ;;
  esac
}

# settled reports that every replica runs the current template and no other
# Pod remains, terminating ones included.
settled() {
  counts=$($K get "$(deployment)" -o jsonpath='{.metadata.generation} {.status.observedGeneration} {.spec.replicas} {.status.updatedReplicas} {.status.availableReplicas}')
  set -- $counts
  [ "$#" -eq 5 ] && [ "$1" = "$2" ] && [ "$3" = "$4" ] && [ "$3" = "$5" ] || return 1
  pods=$($K get pods -l "$SELECTOR" --no-headers 2>/dev/null | wc -l)
  [ "$pods" -eq "$3" ]
}

progress_exceeded() {
  [ "$($K get "$(deployment)" -o jsonpath='{.status.conditions[?(@.type=="Progressing")].reason}')" = ProgressDeadlineExceeded ]
}

# wait_until <check> <timeout-seconds>
wait_until() {
  deadline=$(( $(date +%s) + $2 ))
  while ! $1; do
    check_schedulable
    [ "$(date +%s)" -lt "$deadline" ] || fail "timeout waiting for $1"
    sleep 2
  done
}

# Waits for a rollout and lets traffic settle before the next scenario.
finish_scenario() {
  wait_until settled 300
  check_clients
  log "$1 done"
  sleep "$PAUSE"
}

start_clients() {
  RUNNING=
  round=$1
  i=1
  while [ "$i" -le "$CLIENTS" ]; do
    out=$DIR/rollout-$round-$i
    $K exec "$POD" -- sh -c "rm -f $out.exit $out.pid; nohup sh -c '$BIN -no-errors -duration 6h -settle 20s -output $out $SOAK_ARGS & echo \$! > $out.pid; wait \$!; echo \$? > $out.exit' > $out.log 2>&1 &" ||
      fail "start client $i"
    RUNNING="$RUNNING $out"
    i=$((i + 1))
  done
}

check_clients() {
  for out in $RUNNING; do
    if $K exec "$POD" -- test -e "$out.exit" 2>/dev/null; then
      fail "client $out ended early: $($K exec "$POD" -- tail -n 3 "$out.log")"
    fi
  done
}

stop_clients() {
  for out in $RUNNING; do
    $K exec "$POD" -- sh -c "kill \$(cat $out.pid)" 2>/dev/null
  done
}

# Ends the clients and reports whether they all passed; the final check can
# wait up to five minutes for a recovering backend.
collect_clients() {
  stop_clients
  passed=true
  for out in $RUNNING; do
    deadline=$(( $(date +%s) + 420 ))
    until $K exec "$POD" -- test -s "$out.exit" 2>/dev/null; do
      [ "$(date +%s)" -lt "$deadline" ] || fail "client $out did not finish"
      sleep 5
    done
    code=$($K exec "$POD" -- cat "$out.exit")
    log "client $out exited $code; report in $POD:$out.json"
    if [ "$code" != 0 ]; then
      $K exec "$POD" -- tail -n 3 "$out.log"
      passed=false
    fi
  done
  RUNNING=
  $passed || { log "FAILED: a caller saw an error or an outcome did not hold"; exit 1; }
}

round() {
  replicas=$1
  log "round with $replicas replicas"
  helm_set "${FROM_CHART:-$CHART}" "$replicas" "$DIGEST"
  wait_until settled 300
  start_clients "$replicas"
  sleep "$PAUSE"

  if [ -n "$FROM_CHART" ]; then
    helm_set "$CHART" "$replicas" "$DIGEST"
    finish_scenario "chart upgrade"
  fi

  helm_set "$CHART" "$replicas" "$OTHER"
  finish_scenario "version upgrade"
  helm_set "$CHART" "$replicas" "$DIGEST"
  finish_scenario "version downgrade"

  helm_set "$CHART" "$((replicas + 1))" "$DIGEST"
  finish_scenario "scale up"
  helm_set "$CHART" "$replicas" "$DIGEST"
  finish_scenario "scale down"

  late=$(printf '{"spec":{"template":{"spec":{"initContainers":[{"name":"late-start","image":"%s","command":["sleep","%s"]}]}}}}' "$LATE_START_IMAGE" "$LATE_START")
  $K patch "$(deployment)" --type=strategic -p "$late" >/dev/null || fail "add a late start"
  finish_scenario "replacement that starts late"
  $K patch "$(deployment)" --type=json -p '[{"op":"remove","path":"/spec/template/spec/initContainers"}]' >/dev/null || fail "remove the late start"
  finish_scenario "late start removed"

  victim=$($K get pods -l "$SELECTOR" -o jsonpath='{.items[0].metadata.name}')
  $K delete pod "$victim" --wait=false >/dev/null || fail "delete $victim"
  finish_scenario "deleted Pod"

  helm_set "$CHART" "$replicas" "$UNPULLABLE"
  wait_until progress_exceeded 400
  available=$($K get "$(deployment)" -o jsonpath='{.status.conditions[?(@.type=="Available")].status}')
  [ "$available" = True ] || fail "the old replicas stopped serving during a failed upgrade"
  check_clients
  log "image that cannot be pulled held at the progress deadline"
  helm_set "$CHART" "$replicas" "$DIGEST"
  finish_scenario "recovery from that image"

  collect_clients
  log "round with $replicas replicas passed"
}

for replicas in $REPLICAS; do
  round "$replicas"
done
log "all rounds passed"
