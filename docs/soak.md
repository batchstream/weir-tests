# Soak

How to run long load through deployed Weir replicas under scheduled faults.

## Overview

`weir-soak` drives a steady write and read load through deployed Weir replicas
for hours, then checks every key against the databases directly. It creates a
fresh MongoDB database and Search index, drops them at the end unless
`-keep-data` is set, and never starts, stops or faults services.

## Profiles

Each worker uses the SDK in one way, its profile, and `-profiles` sets their
shares of a backend's workers. The default exercises every call:

| Profile | Writes | Reads |
| --- | --- | --- |
| `single` | a `Mutate` of one `MutationPut` | a `Read` of one key |
| `visible` | the same, with `WithQueryVisible` | a `Read` of one key |
| `lua` | a `Mutate` of one `MutationAtomicTransform` | a `Read` of one key |
| `batch` | a `Mutate` of `-batch-size` keys | a `Read` of as many |
| `stream` | a `Mutate` with `WithNext` and `WithMutateConsume`, held for `-stream-lifetime` | a `Read` with `WithNext` and `WithReadConsume`, held as long |
| `native` | a `Mutate` of one `MutationPut` | the backend's own read through `Native` |

`-read-percent` chooses between a worker's reads and writes; a worker in the
`stream` profile alternates, because each of its turns lasts for many records,
so a run lasts more than two `-stream-lifetime`s. Every record takes one token
of the backend's rate.

## Outcome checks

Each worker alone writes its own keys, with increasing sequence numbers and at
most one write in flight per key. The database may therefore hold only a key's
latest APPLIED sequence or one whose outcome was UNKNOWN, and a key with no
APPLIED write may be absent. Reads through Weir must return such a sequence.
After NOT_STARTED or NOT_APPLIED the worker reads the key directly to confirm
the write took no effect, so a refusal that took effect stays visible. Single
writes then retry with a fresh sequence; batches and streams leave the key for a
later write, as they do not send a record again, and a stream keeps a key out of
use until its check ran. An SDK error without a result counts as UNKNOWN,
including every record of a batch or stream it left unanswered. An UNKNOWN write
is assumed to land within four operation timeouts of being issued, or for a
stream's record, which has no deadline of its own, of the stream giving up on
it; a later APPLIED write retires it. `-verify-percent` of APPLIED writes are
also read back directly. Single-key MongoDB checks read linearizably from the
primary, so they see a write applied just before a refusal and are never stale
after a failover; the final check reads majority-committed data. Search checks
use realtime GET and `_mget`, and a document its shard cannot serve fails the
check instead of reading as absent.

## Run

```sh
weir-soak -weir-address weir.soak.svc.cluster.local.:7447 \
  -weir-diagnostics weir.soak.svc.cluster.local.:7449 \
  -mongo-uri 'mongodb://mongo-0.mongo.soak.svc:27017,mongo-1.mongo.soak.svc:27017,mongo-2.mongo.soak.svc:27017/?replicaSet=soak' \
  -search-url http://elasticsearch.soak.svc:9200 -duration 12h -output /work/soak
```

The Weir addresses are names that resolve to every replica, such as a headless
Service's full name with a trailing dot, which skips the Pod's DNS search
domains; the diagnostics name is resolved at every sample to scrape each
replica's goroutines, live memory, RSS, open files and sessions.

## Output

`<output>.jsonl`
holds one sample per `-sample-interval` with per-backend outcomes, latency
quantiles and database connections, and `<output>.json` the totals and final
check. Both break these down by SDK call under `apis`, with the calls made,
those that ended at a deadline and the last error and failure, so a call that
fails during a rollout shows by name; the longest read includes reads that
failed. The final check retries a backend that refuses reads for up to five
minutes. The run fails on any violation, an incomplete final check or a
configured SDK call that never succeeded on a backend, and with `-no-errors` on
any error a caller saw.

## Fault injection

`scripts/soak-faults.sh <namespace> <weir-release> <quiet> <window> <every>`
injects faults from a Pod in the same namespace, whose service account may get,
list, delete and exec into Pods, watch Deployments and ReplicaSets, and patch
Deployments and `deployments/scale` there. After `quiet` seconds it rotates
through a MongoDB primary stepdown, a rolling restart of the weir-charts
release, an Elasticsearch process restart, a MongoDB primary Pod deletion and a
Weir Pod deletion, one every `every` seconds for `window` seconds, and waits for
recovery after each. A fault that does not take effect or recover in time ends
the schedule with a non-zero exit. `FAULTS` replaces the rotation with a
space-separated list of those faults, `stepdown weir_rollout es_restart
mongo_kill weir_kill`, plus `weir_scale_up` and `weir_scale_down`, which change
the release's replica count by one as an autoscaler would and wait until no
replica is still terminating. Run scaling without KEDA on the release, which
would undo the change.

## Rollout scenarios

`scripts/rollout-scenarios.sh <namespace> <release> <chart> <values> <digest>
<other-digest>` checks what operators do to a release under load, from a machine
with `helm` and `kubectl`. At each replica count in `REPLICAS`, default `1 3`, it
installs the release, starts `CLIENTS`, default 2, soak processes with
`-no-errors` in the Pod `SOAK_POD`, and goes through a chart upgrade from
`FROM_CHART` if set, a version upgrade and downgrade, scaling up and down by one,
a replacement that starts `LATE_START` seconds late, a deleted Pod and an image
that cannot be pulled, held until the progress deadline and then recovered from.
Each is a real change of the release, and it waits for every old Pod to leave. The
script exits non-zero if a change does not complete, the old replicas stop
serving during the failed upgrade, or a client fails, which `-no-errors` makes
any error a caller saw. `SOAK_ARGS` holds the soak's addresses and load, and the
namespace must already hold the backends and the Pod, and the values must not
enable autoscaling, which leaves the replica count to the autoscaler. Check that
the namespace's nodes have room for three replicas more than the largest count:
a rollout adds one replica while terminating ones still hold theirs, and a Pod
that cannot be scheduled ends the run. A run that does not pass, interrupted ones
included, ends with its clients stopped and the release scaled to zero.

## Read the results

Each sample records every replica's `weir_store_records_total`, so the change
between samples is that replica's share of the traffic. A diagnostics name that
also publishes not-ready addresses, such as a headless Service with
`publishNotReadyAddresses: true`, keeps a scaled-down replica in the samples
until it exits, showing when its traffic stopped. A short `-sample-interval`,
such as 5s, resolves how quickly traffic moves. A
fault-free first half shows whether memory, goroutines, files, connections and
latency stay flat; the second half checks outcomes across failover and restarts.
End the window at least 20 minutes before the load so the last recovery
finishes first.
