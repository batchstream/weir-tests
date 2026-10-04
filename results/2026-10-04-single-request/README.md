# Independent single-request throughput evidence

This corrects the previous bulk-versus-bulk experiment. Four independent business
client OS processes send individual reads and writes; only Weir may aggregate
requests across RPCs. The question is whether that aggregation increases verified
business throughput, and under which workloads. A configured batch limit alone
does not demonstrate aggregation or increased database capacity.

## Reproducible identities

| Component | Fixed identity |
|---|---|
| Test recipe | `e790aa77629f0e4862159cbe8e3f0497df888314` |
| Measured Weir production source | `2305186fffc5d1d9273e223f222e016fc8afe3ed` |
| Published server module | `v0.1.1-0.20261003235319-2305186fffc5` |
| Server module checksum | `h1:6nCxXeaPaILriyyg8B0BJlYBsz6sYnNEaQcDk/CoFW8=` |
| Final reviewed server PR head | `4713a5cf319986ae4270e8ac2760e6ff7b3f0684` |
| SDK / protocol | `v0.4.2` / `v0.2.1` |
| Go | `1.27.1` |

The final server PR head differs from the measured source only in one Search
test fixture: two direct Scan workers plus one Native call require three execution
slots. These direct test calls bypass normal Store admission. All non-test files
are identical to the official measured module. This is independently checked in
[final-source-review.json](final-source-review.json). No measured binary was
silently replaced by a local build. Official download provenance is retained in
[server-module-download.json](server-module-download.json).

## Workload and resource policy

- Four real client processes, each with its own SDK or native driver pool.
  Total concurrency is 8, 32, or 128, with 512 as a separately retained stress
  ladder. Each worker waits for its one-record call before issuing another.
- MongoDB uses native `FindOne`/`ReplaceOne`; Search uses one-ID `POST _mget`
  and one-document `PUT _doc`. Clients do not aggregate. Driver, HTTP and SDK
  write replay is disabled. Fixture setup and reset batches are outside timing.
- 2,048 records with 1 KiB padding, disjoint worker-owned IDs, deterministic
  0%/10%/100% write workloads. Every acknowledged write changes its revision.
  Final values and record counts are independently checked directly in the DB.
- Three alternating AB/BA paired rounds per concurrency: 10 seconds warmup,
  20 seconds measurement. Connections and worker revision state continue from
  warmup. A common phase barrier and child start lag are recorded. In-flight
  calls retain their original deadline after new work stops and are joined.
- Linux runners report four host CPUs. Each owned database container has the
  same inspected **1 CPU** quota for both paths. Client processes and Weir share
  the remaining host capacity; they are not independent dedicated hosts.
  Absolute throughput from different runner jobs is not a controlled comparison.
- Client operations and every benchmark Weir Store have the same **10-second**
  request/execution budget. Queueing does not reset a caller's deadline.
  Connection establishment has a separate two-second guard. Ordinary production
  configuration still defaults to two seconds when `backend_timeout` is omitted.
- Weir backend execution concurrency remains 32. Grouping limit 1 disables
  aggregation; limit 32 permits it. Workspace, data, pools, quota and timeout
  policy remain fixed within a same-job control. Each mode uses fresh owned
  fixtures, in the recorded order **1 then 32**. This order is not randomized;
  the separately repeated native baseline shows time/cache/runner variation.
- Admission budgets of 18 GiB at concurrency 128 and 54 GiB at 512 are declared
  accounting limits, not allocations or OS reservations. Actual RSS is sampled.

## Verified business results

Primary matrix at total concurrency **128**, across four independent client processes:

| Database | Write % | Direct req/s | Weir req/s | Change | Direct p99 ms | Weir p99 ms | Actual batch | DB CPU direct / Weir |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| mongo | 0 | 19,829 | 18,010 | -9.2% | 45.05 | 20.48 | 3.40 | 100.2% / 59.8% |
| mongo | 10 | 5,260 | 8,071 | +53.4% | 81.92 | 45.05 | 3.40 | 100.1% / 78.1% |
| mongo | 100 | 2,645 | 3,527 | +33.4% | 327.68 | 180.22 | 2.76 | 99.9% / 99.8% |
| search | 0 | 19,630 | 16,597 | -15.4% | 36.86 | 20.48 | 2.92 | 99.9% / 59.1% |
| search | 10 | 5,642 | 7,172 | +27.1% | 73.73 | 49.15 | 2.95 | 100.0% / 82.4% |
| search | 100 | 2,020 | 5,048 | +149.9% | 114.69 | 65.53 | 3.62 | 72.6% / 74.6% |

Same-job control at concurrency 128 (limit 1 then 32; native baseline variation is reported):

| Database | Write % | Weir limit 1 req/s | Weir limit 32 req/s | Change | Native baseline change | Batch 1 / 32 |
|---|---:|---:|---:|---:|---:|---:|
| mongo | 0 | 8,169 | 14,743 | +80.5% | +1.8% | 1.00 / 3.96 |
| mongo | 10 | 4,701 | 7,936 | +68.8% | -0.8% | 1.00 / 3.41 |
| mongo | 100 | 2,309 | 3,699 | +60.2% | +0.1% | 1.00 / 2.80 |
| search | 0 | 7,862 | 12,699 | +61.5% | +0.7% | 1.00 / 3.31 |
| search | 10 | 4,288 | 7,024 | +63.8% | -3.0% | 1.00 / 2.97 |
| search | 100 | 1,520 | 4,826 | +217.5% | +0.0% | 1.00 / 3.64 |

Results describe this measured workload and hardware. p99 values are pooled
inclusive logarithmic-histogram bucket upper bounds, with rounding error at most
12.5% plus 1 microsecond. They are not averages of client percentiles. Confidence intervals are not
reported from these three rounds. Low-concurrency regressions are
retained in every report and in `business-summary.json`.

The primary run completed all six reports and 108 timed paths with 16,103,395
successful operations and no failed or unknown business outcomes. Both the
strict root audit and separate reviewer recomputation passed. The observed
benefit depends on the workload; it is not a universal recommendation to place
Weir in front of every database read.

## Capacity and stress scope

A database-saturated ratio requires both paths independently to show sustained
CPU use and a next-level throughput plateau. Every round needs at least five
valid CPU intervals, at least 80% measurement coverage, mean CPU at least 90%
of the inspected quota, and at least 80% measured time above that threshold.
The next measured concurrency must add no more than 10% throughput.

The 8/32/128 primary matrix does **not** qualify a database-saturated ratio for
any workload. Observed business peaks are therefore reported separately from
database capacity. A highest tested concurrency without a next level cannot
prove a plateau. CPU alone also cannot establish disk or network saturation.

These stress runs use separate runners from the primary matrix. Compare each row within its paired run; absolute QPS across tables is not controlled.

| Database | Write % | Direct req/s | Weir req/s | Change | Actual batch |
|---|---:|---:|---:|---:|---:|
| mongo | 0 | 10,469 | 18,117 | +73.0% | 12.05 |
| mongo | 10 | 5,097 | 11,544 | +126.5% | 9.78 |
| mongo | 100 | 2,906 | 6,581 | +126.4% | 9.54 |
| search | 0 | 10,365 | 15,429 | +48.9% | 9.88 |
| search | 100 | 1,548 | 9,713 | +527.3% | 12.33 |
| search | 10 | unavailable | unavailable | unavailable | unavailable |

Search mixed-load round 1 at concurrency 512 failed during native warmup: 52 operation timeouts (47 errors and 5 UNKNOWN writes), maximum 10.003122920 seconds. Unknown write effects remain unconfirmed. The earlier Weir timed stage completed 209,844 verified operations, but does not create a paired 512 ratio. All 19 completed Search timed paths remain individually verified; five planned timed paths are absent.

Overall stress evidence contains five complete backend reports plus one incomplete report, 139 individually successful timed paths and 17,575,059 successful timed operations. It is not a complete six-report PASS. No stress dataset establishes a paired database-saturated capacity ratio.

## Raw evidence and independent review

- [Run index](run-index.json) records the exact GitHub runs, recipe and pins.
- `final-primary/` retains six complete reports, before/after public Prometheus
  text, every client PID and process exit, request histograms, resource samples,
  fixture configuration, inspected DB quota, cleanup and complete run logs.
- `final-control/` retains the same-job grouping-limit control. The independent
  audit checks that only grouping mode and fresh fixture identities change.
- `final-stress/` preserves the complete 512 ladder, including failed warmups
  or timed requests. Failed stages are not silently excluded from the record.
- [Primary strict audit](final-primary-audit.json),
  [independent primary audit](reviewer-primary-audit.json) and the combined
  reviewer audit retain recomputed throughput, pooled latency, CPU qualification,
  actual batch histograms, peak selection and unavailable capacity conclusions.
- [System review](final-system-review.json): 10 passing tests/subtests, zero
  skips; 1,024 smoke operations; real MongoDB and Search, multiprocess clients,
  direct owner discovery, batched SDK operations and independent persistence.
  All owned fixtures are cleaned. A separate instrumented CPU/mutex diagnostic
  is identified explicitly and excluded from capacity measurements.
- `server-validation/` preserves source-level deadline, cancellation, resource
  ownership and real-backend checks. External M10 and TLS opt-ins were not run.
  Old combined-command failures and missing original stdout are explicitly
  marked; the fresh real Search replay has its own complete stdout and receipt.
  `server-ci/` retains the successful final Linux CI. `local-functional/` is
  macOS functional coverage with native Mongo and does not imply a CPU quota.

MongoDB physical-command counters are global `serverStatus` values. They include
commands outside the business namespace; excess counters are preserved without
inventing an attribution. An adapter invocation can also issue separate read
and write commands. Search does not provide a physical HTTP request counter here.
Optional filesystem statistics have their stated scope and unavailable values;
none of these counters prove device saturation.

`initial-*` and `linux-*` preserve earlier observations with the MongoDB counter
parser / Search monitor defect. `prior-03/` preserves runs with a 10-second
client budget but a hidden two-second Weir backend cap, including failed 512
warmups and nine unknown Search write acknowledgements. These are superseded
observations, not the final equal-budget comparison. The historical
`../2026-10-03-bulk-refactor/` experiment measures proxy overhead with bulk clients.

## Recompute

From the repository root, with Python 3 and no running database:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 results/2026-10-04-single-request/audit_single_requests.py \
  results/2026-10-04-single-request/final-primary \
  --head e790aa77629f0e4862159cbe8e3f0497df888314 \
  --server 2305186fffc5d1d9273e223f222e016fc8afe3ed \
  --require-equal-timeouts --output /tmp/weir-primary-audit.json

PYTHONDONTWRITEBYTECODE=1 python3 results/2026-10-04-single-request/reviewer_audit.py \
  --primary results/2026-10-04-single-request/final-primary \
  --control results/2026-10-04-single-request/final-control \
  --head e790aa77629f0e4862159cbe8e3f0497df888314 \
  --server 2305186fffc5d1d9273e223f222e016fc8afe3ed \
  --version v0.1.1-0.20261003235319-2305186fffc5 \
  --sum 'h1:6nCxXeaPaILriyyg8B0BJlYBsz6sYnNEaQcDk/CoFW8=' \
  --primary-levels 8,32,128 --control-levels 8,32,128 \
  --output /tmp/weir-reviewer-audit.json \
  --control-output /tmp/weir-reviewer-control-audit.json

cd results/2026-10-04-single-request
shasum -a 256 -c SHA256SUMS
```

To repeat a measured run, use the immutable recipe checkout, prepared public
tools/images, and the exact command in each measurement receipt. The workflow
inputs are recorded in the run index; changing an SLO, quota, client count,
pool limit, dataset, or source creates a different experiment.
