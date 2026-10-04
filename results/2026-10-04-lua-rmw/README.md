# Single-request Lua read-modify-write aggregation

[Server PR #32](https://github.com/batchstream/weir/pull/32) batches independent
Lua mutations as R → individual M → W. MongoDB uses a shared short snapshot
transaction and majority commit; Elasticsearch uses its native observed
sequence number and primary term. Weir does not add version fields to business
documents or require cooperation from direct database writers. Lua stays in the
main Weir process. Ambiguous writes are not replayed.

The [successful control run](https://github.com/batchstream/weir-tests/actions/runs/37172507455)
compares four independent client processes, one URI per business request, 100%
read-modify-write, 2,048 disjoint records and 1,024 padding bytes. Native MongoDB
owns one snapshot transaction per request (FindOne → compute → ReplaceOne →
majority commit); native Search does a single-ID realtime read followed by a
conditional single-document write. The SDK sends one Lua AtomicTransform per
RPC, computing the same toggle of the fixture's pre-existing `revision` field.
This fixture field is workload data, not metadata added by Weir. Neither timed
client uses a bulk API. Untimed preparation/reset and postflight can use bulk
operations and are outside measurement counters.

Each path has three alternating AB/BA rounds, 10 seconds warmup and 20 seconds
measurement, at 8, 32 and 128 concurrent workers. Database containers have a
verified 1 CPU quota each. Both clients, one Weir process and the containers
share a four-CPU Linux amd64 runner. Store concurrency is 32, collection wait is
zero, and the only changed control setting is adapter batch limit 1 versus 32.
Controls execute sequentially with fresh owned fixtures on the same job runner;
they are not randomized or simultaneous trials. This hot indexed, loopback,
uncontended-record workload does not establish hot-key or remote-network gains.

At **128 concurrent single-request workers**, pooled successful business mutations/s:

| Backend | Direct, limit-1 fixture | Weir limit 1 | Direct, limit-32 fixture | Weir limit 32 | Weir 32 / matched direct | Actual Weir batch, limit 32 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| MongoDB | 1,251.6 | 1,086.9 | 1,236.2 | 2,202.4 | 1.782× (+78.2%) | 2.527 |
| Elasticsearch | 1,520.1 | 1,471.2 | 1,455.0 | 3,061.3 | 2.104× (+110.4%) | 2.582 |

Changing Weir limit 1 → 32 increases its measured throughput by 102.6% for MongoDB
and 108.1% for Search; rerun native baselines change by -1.2% and -4.3%
respectively. The aggregation histogram and MongoDB's physical
find/bulkWrite/commit counters demonstrate grouping of independent callers.
MongoDB's three 128-worker limit-32 rounds contain 132,310 mutations in 52,368 committed
transactions, with zero aborts. Search statistics do not expose physical HTTP
command counts; its batch evidence is explicitly adapter invocation metrics.

Database CPU at 128 workers is 99.93% direct / 99.99% Weir for MongoDB and
100.02% direct / 92.52% Weir for Search, relative to each database's 1-CPU quota.
Every round meets the sustained CPU criterion. Values slightly above 100% are
raw sampling observations. **A maximum-capacity ratio for limit 32 is still
unavailable:** throughput is increasing at the highest measured concurrency,
which has no following level to verify a plateau. These are matched-concurrency
observations with demonstrated CPU load, not a general guarantee of peak
capacity. Limit 1 has qualified lower-concurrency plateaus; raw reports retain
those separate comparisons. Search's first 8-worker native round is much slower
than later rounds, so the pooled low-concurrency observation should be read
alongside individual rounds.

All **72 timed Lua stages**, totaling **1,963,653 acknowledged mutations**, pass
independent persisted-data validation with zero business errors, UNKNOWN,
timeouts, not-attempted or applied-with-error results. Each stage preserves all
four child PIDs, disjoint final record ranges, joined completion, latency
histograms, raw Docker CPU counters and raw Prometheus observations. MongoDB's
global update counter has one extra update at 32 workers round 2 in each native
fixture; this is retained as unattributed counter excess rather than assigned
to a business request. Native find and commit counts match acknowledged
mutations. Both control fixtures removed their four containers and stopped
their two Weir processes without cleanup errors.

[Linux System tests](https://github.com/batchstream/weir-tests/actions/runs/37172465131)
pass real-database public SDK, discovery, recovery, cancellation, native-reset
and Lua smoke checks. All eight finite Lua smoke stages acknowledge and verify
256 mutations each; these are functional checks, not capacity evidence. The
[ordinary read/write regression](https://github.com/batchstream/weir-tests/actions/runs/37172465003)
also passes all 108 timed stages across 0%, 10% and 100% writes. Its aggregate
review record is retained; it is a separate workload from Lua RMW.

The measured clean benchmark source is
`9d9e4ce1610881330049b69c4a0de7d872003af5`; the official immutable server source is
`514deab214dd801dd4fc95c5184435b6a9a17a3d`, version
`v0.1.1-0.20261004023249-514deab214dd`, checksum
`h1:VxY5kf4L5t2jDEidQl8iORSxvkkszL+Z/OSfup+hcrg=`. Server PR32's squash merge has
the identical tree. SDK **v0.4.2**, protocol **v0.2.1**, Go **1.27.1**, MongoDB
**8.0.32** and Elasticsearch **8.19.22** are pinned. Artifact names from PR CI
contain its synthetic merge SHA; their reports identify the clean measured
benchmark head explicitly. The production Linux server SHA256 is
`6611a3b65b934917a41e99ede00a97036fef9d523036aa865f650dbd450f25d0`.

`control/` retains all final raw reports, exact executed command receipts,
fixture manifests/configuration, process logs and cleanup. `linux-system/`
retains real integration logs, functional reports and diagnostic profiles.
The CPU diagnostic is a separately built binary and is not used in the Lua
throughput table. The uploaded placeholder `weir-profile` executable is omitted;
its diagnostic receipt remains. `server-tests/` contains local real-backend
race and abort-confirmation logs. Root and independent reviewer audits, module
metadata and run/PR identities accompany the evidence. `SHA256SUMS` covers every
archived file except itself. Earlier failed or cancelled benchmark attempts are
not used in these tables.

Recompute the raw timed-stage CPU, latency, child ownership, adapter metrics and
MongoDB command accounting using the retained prior auditor helpers:

```sh
python3 results/2026-10-04-lua-rmw/audit_control.py
```

The exact reproducible benchmark commands are in
`control/measurement-batch-1.receipt.json` and
`control/measurement-batch-32.receipt.json`; run them from the recorded clean
benchmark revision on a compatible Linux Docker host after `make prepare`.
Use a new output directory. Any change in host isolation, quotas, concurrency,
payload, write semantics, database or source version needs a new measurement.
