# Native bulk refactor evidence

Production source is `b73c83a47791ac97bb5533b5b290c96d966a2e43`; final test recipe is `4e6c8a7c0c5e16d094964ce60af24320835736e3`. The public modules are Go SDK v0.4.2 and protocol v0.2.1. `versions.json` locks the full source revision, immutable pseudo-version and official module checksum. Profile binaries have a separate identity and are not used for capacity conclusions.

## Measurement and interpretation

Both paths use the same owned database, deterministic 2,048 records, 1,024-byte payload, 32 items per call and real revision changes. Three AB/BA paired rounds warm up for 10 seconds and measure for 20 seconds at 8, 32 and 64 client workers. Reset, independent postflight and persistence validation are outside timing; all in-flight completion is inside timing. MongoDB uses majority acknowledgement; Elasticsearch batch writes use refresh=false and wait_for_active_shards=1; independent postflight refresh is outside timing. A native MongoDB collection bulk command and Weir's database-level per-item acknowledgement command have different physical acknowledgement costs despite matched logical operations.

Capacity requires every round of a stage to have a time-weighted database CPU mean >=90%, >=80% measurement time above that threshold, >=80% sampling coverage and >=5 valid intervals, followed by <=10% throughput improvement at the next concurrency level. Read and write rates are logical records per second, not RPC/s. An underloaded database is reported as a throughput shortfall, without a database-capacity percentage. CPU quota is checked against the owned container's actual ID and HostConfig.NanoCpus before timing. The 1 CPU and 0.5 CPU experiments are separate capacity envelopes; the latter cannot establish 1 CPU capacity.

The Linux runner, client, Weir and databases share the runner's CPU resources. Per-process CPU samples and process memory declarations are evidence, not claims of dedicated host resources. `working_memory` and the process memory budget are admission limits, not preallocated RAM or OS reservations. The fixture computes them from the same 2MiB per-read worst case used by native backend preparation. At 32 configured backend permits, the two-Store fixture declares 1,312MiB MongoDB workspace, 3,072MiB Elasticsearch workspace and 12GiB process memory. Transport/output ownership bounds remain enforced.

## Root causes and final design

The client batch previously reached a streamed operation pipeline and was collected again by a backend scheduler. Native unary ReadBatch and MutateBatch now carry the whole same-Store batch as one admission/scheduling unit. Backends may group records by namespace, byte bounds and duplicate mutation order. Arbitrary 512-item and fixture 32-concurrency ceilings were removed. Scan and Native retain their single-command stream semantics.

The old latency-only adaptive scheduler reduced a configured window from 32 to 1 while queue wait dominated database execution. It could throttle an idle database. Fixed configured permits, event-driven queue wakeups and releasing RPC admission at handler completion replace that behavior.

A Linux CPU profile of the fixed-dispatch baseline found PrepareBatch/PrepareOperation at 22.29% cumulative sampled CPU. Overlapping protocol and resource-path call trees must not be added to that number. The same relative URI was validated, rebuilt as a full URI, validated again, and parsed again by the adapter. The final immutable internal Record boundary validates the complete public batch once, retains decoded path segments once and sends them directly to backend-specific preparation. Full-URI reconstruction, redundant public validation and namespace regex parsing are gone. Late invalid records still cause zero backend effects.

The baseline's fixed 384MiB workspace admitted about 9 MongoDB or 4 Elasticsearch read batches at the configured worst-case read size, despite a configured concurrency of 32. Per-Store working_memory is now explicit and participates in checked process-budget accumulation; the fixture sizes it for actual declared concurrency. A two-Store MaxInt regression verifies oversized budgets cannot wrap and bypass admission. Deep-path preparation metadata is checked against the existing request/Store byte budgets before retaining batch records, rather than adding an arbitrary segment count.

## Historical evidence

`../2026-10-03-saturation/` is the pre-refactor baseline. It shows approximately 6,000 Weir read records/s with an underloaded database, so it does not establish a database-capacity ratio.

`attempt-1-incomplete/` preserves an interrupted intermediate attempt that hit RPC admission during the 64-worker warmup. It is incomplete and excluded from capacity conclusions. `system/` and `system-final/` are successful historical system checks for earlier immutable production revisions, not the final Record refactor.

`fixed-dispatch-baseline/` and `fixed-dispatch-baseline-run.json` preserve the complete unprofiled ed43aa2/2b3a67d baseline after native bulk and fixed dispatch but before the Record/workspace refactor: 54 pairs / 108 timed paths, zero reported operation errors, independently verified state and owned cleanup. Its raw Docker quota inspection was verified by code but was not serialized into reports; the final version records that inspection explicitly. Pure-read Weir MongoDB/Elasticsearch remained underloaded at approximately 59,050/36,151 records/s at 64 workers. MongoDB's eligible saturated mixed and write capacities were 33,421/5,828 records/s versus native 39,488/6,666, respectively (-15.37%/-12.57%). Elasticsearch did not satisfy both-path saturation in those runs.

`record-refactor-first-1cpu/` preserves the first complete final-production run at test recipe 7d49196. Its 108 timed paths passed independent recalculation, persistence checks, byte budgets and actual owned Docker NanoCpus inspection. It establishes eligible mixed capacity differences of -12.98% for MongoDB and -17.49% for Elasticsearch. Pure-read Weir at 64 workers measured 65,935/39,630 records/s with database CPU 66.73%/83.85%; those are underloaded throughput observations, not capacity ratios. Pure-write cases did not establish both-path full-load capacity.

These historical runs used separate CI runners. Mixed direct-native throughput itself changed substantially and Elasticsearch write rates varied with persistent I/O, so cross-run absolute throughput changes cannot be attributed entirely to the Record refactor. Same-run AB/BA pairs are the basis for capacity comparisons.

`half-cpu-startup-failed/` preserves three setup-only failures at 7d49196, before any timed workloads. Each failed Elasticsearch index-template publication at the fixture's 3s HTTP deadline and cleaned up its two owned containers. No complete fixture manifest or timed workload exists for these attempts; `status.json` records those limits. The fixture-only 4e6c8a7 follow-up retains short version/health probes but gives template publication a separate 60s request bound within the 180s startup/caller context. The regression test waits beyond 3s for a successful publication and cancels a second call after the actual PUT has entered, verifying prompt cancellation.

`record-refactor-system/` and its independent audit apply to 7d49196; `final-system/` applies to the final 4e6c8a7 recipe. Both use the same immutable b73c83a production source, with profile helpers kept separate from the production binary. PrepareBatch cumulative CPU is 0.81s/2.04% versus 9.68s/22.29% in the fixed-dispatch profile, but some preparation moved into NewReadRecords (3.74s/9.41%). Their mutually exclusive preparation call trees together account for 4.55s/11.44% in that diagnostic and 4.46s/11.04% in final-system; preparation did not fall to 2% overall. Total sampled mutex delay increased from 14.88s to 17.41s in that diagnostic, so the evidence does not support a claim that every lock improved. Driver/adapter work, allocations, response validation and encoding remain visible costs. Profiles include startup and warmup; overlapping cumulative stacks are not additive and instrumented throughput is not production capacity evidence.


## Final unprofiled results

The final two envelopes each contain 54 paired rounds / 108 timed paths: **148,456,832 successful logical operations**, zero errors/UNKNOWN/APPLIED-with-error/not-attempted outcomes, independent verified persistence, exactly 32 records per adapter invocation and matching unary RPC counts. Actual owned Docker quotas are 1,000,000,000 / 500,000,000 NanoCpus; raw sampling coverage is >=94.64% / >=93.98%, respectively. Each envelope stopped its 3 owned processes and removed its 6 owned containers without cleanup errors.

The raw evidence policy strings come from the shared single-record path API description (for example PUT_doc or a one-ID _mget); they describe logical operation policy and are not wire-command traces for the bulk workload. The actual native bulk code uses _bulk / multi-ID _mget and the metrics prove adapter batch cardinality.

The following capacity deltas use each path's best eligible point under the report's 90% CPU definition. Each next-level throughput gain must be <=10%. All quantities are logical records/s, averaged across 3 paired rounds.

| Database CPUs | Backend | Writes | Direct records/s | Weir records/s | Weir vs Direct |
|---:|---|---:|---:|---:|---:|
| 1 | mongo | 10% | 58,246 | 54,875 | -5.79% |
| 1 | mongo | 100% | 6,518 | 5,924 | -9.11% |
| 1 | search | 10% | 60,312 | 51,434 | -14.72% |
| 0.5 | mongo | 0% | 54,041 | 46,804 | -13.39% |
| 0.5 | mongo | 10% | 42,902 | 37,315 | -13.02% |
| 0.5 | mongo | 100% | 3,398 | 3,034 | -10.71% |
| 0.5 | search | 0% | 25,190 | 23,419 | -7.03% |
| 0.5 | search | 10% | 36,941 | 31,539 | -14.62% |

The 1 CPU pure-read paths still lack Weir database saturation. At 64 client workers, actual measured values are:

| Backend | Direct records/s | Weir records/s | Weir DB CPU |
|---|---:|---:|---:|
| mongo | 106,897 | 67,309 | 66.83% |
| search | 53,066 | 40,913 | 82.80% |

The 0.5 CPU pure-read experiment, at the **same 32-worker concurrency** on both paths, independently passes the stricter 95% CPU criterion and has a next-level plateau. Both paths' database means are approximately 100%:

| Backend | Direct records/s | Weir records/s | Weir vs Direct |
|---|---:|---:|---:|
| mongo | 51,905 | 46,804 | -9.83% |
| search | 25,190 | 23,419 | -7.03% |

This smaller database envelope does not establish 1 CPU capacity. At 95%, 1 CPU Elasticsearch mixed capacity becomes unavailable. MongoDB mixed changes from -5.79% to +1.20% because the stricter predicate excludes the faster native 8-worker point and chooses 32 workers; this sign change is **selection sensitivity, not evidence of a stable Weir speedup**. The full per-interval recalculation and chosen points remain in `final-95percent-audit.json`. Elasticsearch write-only is unavailable: the 1 CPU envelope lacks both-path full-load proof; the 0.5 CPU envelope reaches CPU full load but gains 14–15% between 32 and 64 workers and has no measured plateau.

## Final verification and reproducibility

- Server CI: [37122074340](https://github.com/batchstream/weir/actions/runs/37122074340), exact b73c83a source; independent code review and offline full race/vet/tagged compilation passed. Real native MongoDB 8.0.32 and Elasticsearch 8.19.22 backend/server integration passed.
- Final System: [37123392364](https://github.com/batchstream/weir-tests/actions/runs/37123392364), exact 4e6c8a7 recipe. Six real blackbox subtests pass without skips; 513-record batches, result/duplicate order, whole-input rejection without effects, direct-owner discovery, cancellation, scan and graceful owner withdrawal/recovery are covered. Matched two-backend smoke and owned cleanup pass; the independent audit has 69 PASS / zero FAIL. The instrumented helper has its own identity/hash and cannot supply capacity evidence.
- Final 1 CPU matrix: [37123392352](https://github.com/batchstream/weir-tests/actions/runs/37123392352), all read/mixed/write jobs pass.
- Final 0.5 CPU matrix: [read 37123390740](https://github.com/batchstream/weir-tests/actions/runs/37123390740), [mixed 37123392919](https://github.com/batchstream/weir-tests/actions/runs/37123392919), [write 37123395573](https://github.com/batchstream/weir-tests/actions/runs/37123395573), all pass.

`final-1cpu/`, `final-half-cpu/` and `final-system/` preserve raw reports, metrics, resource samples, configuration, binary receipts and owned cleanup. Artifact-name suffixes may be GitHub's synthetic PR merge SHA; the measurement receipts identify the actual checked-out 4e6c8a7 source. The results-only archival commit does not change the measured code or version locks. `SHA256SUMS` covers every archived file except itself.

Run `sha256sum -c SHA256SUMS` from this directory. The standalone `audit_saturation.py` recalculates CPU intervals, workload outcomes, metrics, version receipts, actual quota and workspace from the raw files. Example:

```sh
python3 audit_saturation.py final-1cpu --head 4e6c8a7c0c5e16d094964ce60af24320835736e3 --server b73c83a47791ac97bb5533b5b290c96d966a2e43 --quota 1 --output /tmp/rechecked-1cpu.json
python3 audit_saturation.py final-half-cpu --head 4e6c8a7c0c5e16d094964ce60af24320835736e3 --server b73c83a47791ac97bb5533b5b290c96d966a2e43 --quota 0.5 --output /tmp/rechecked-half-cpu.json
```
