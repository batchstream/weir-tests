# Database saturation comparison

Backend **mongo**. Started 2026-10-03T22:22:01.929029986Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 197258 | 21812 | 219070 | 0 | 10953.0 | 1.407 | 98.8 | 96.0 | 96.0 | true |
| 1 | 8 | 1 | weir | 113127 | 12653 | 125780 | 0 | 6288.3 | 2.815 | 74.1 | 0.0 | 95.8 | true |
| 1 | 8 | 2 | direct | 193175 | 21344 | 214519 | 0 | 10725.6 | 1.407 | 99.0 | 94.9 | 94.9 | true |
| 1 | 8 | 2 | weir | 115297 | 12874 | 128171 | 0 | 6408.1 | 2.815 | 76.4 | 0.0 | 95.9 | true |
| 1 | 8 | 3 | direct | 197829 | 21870 | 219699 | 0 | 10984.5 | 1.407 | 99.0 | 95.9 | 95.9 | true |
| 1 | 8 | 3 | weir | 114699 | 12792 | 127491 | 0 | 6374.2 | 2.815 | 76.5 | 0.0 | 95.8 | true |
| 1 | 32 | 1 | direct | 229171 | 25709 | 254880 | 0 | 12742.6 | 4.607 | 100.2 | 95.5 | 95.5 | true |
| 1 | 32 | 1 | weir | 171296 | 19240 | 190536 | 0 | 9525.6 | 7.167 | 87.1 | 5.2 | 96.3 | true |
| 1 | 32 | 2 | direct | 231288 | 25877 | 257165 | 0 | 12857.6 | 4.607 | 100.0 | 95.4 | 95.4 | true |
| 1 | 32 | 2 | weir | 166281 | 18671 | 184952 | 0 | 9245.9 | 7.167 | 87.9 | 21.3 | 96.4 | true |
| 1 | 32 | 3 | direct | 219369 | 24474 | 243843 | 0 | 12191.0 | 4.607 | 99.8 | 96.5 | 96.5 | true |
| 1 | 32 | 3 | weir | 161311 | 18095 | 179406 | 0 | 8969.1 | 7.679 | 88.6 | 27.1 | 96.8 | true |
| 1 | 128 | 1 | direct | 217315 | 23983 | 241298 | 0 | 12062.2 | 53.247 | 100.0 | 95.4 | 95.4 | true |
| 1 | 128 | 1 | weir | 244896 | 26897 | 271793 | 0 | 13583.0 | 20.479 | 81.9 | 0.0 | 96.5 | true |
| 1 | 128 | 2 | direct | 223684 | 24686 | 248370 | 0 | 12414.9 | 53.247 | 99.9 | 96.5 | 96.5 | true |
| 1 | 128 | 2 | weir | 247361 | 27171 | 274532 | 0 | 13722.8 | 18.431 | 83.0 | 0.0 | 96.9 | true |
| 1 | 128 | 3 | direct | 215774 | 23769 | 239543 | 0 | 11970.5 | 53.247 | 99.9 | 96.4 | 96.4 | true |
| 1 | 128 | 3 | weir | 260164 | 28570 | 288734 | 0 | 14434.0 | 18.431 | 84.5 | 10.5 | 96.4 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 115258 | 1.091 | 113127/12653 | 0.010 | 0.607 | 0 | 32.000 |
| 1 | 8 | 2 | 117517 | 1.091 | 115297/12874 | 0.010 | 0.594 | 0 | 32.000 |
| 1 | 8 | 3 | 117171 | 1.088 | 114699/12792 | 0.010 | 0.593 | 0 | 32.000 |
| 1 | 32 | 1 | 130548 | 1.460 | 171296/19240 | 0.054 | 1.148 | 0 | 32.000 |
| 1 | 32 | 2 | 124683 | 1.483 | 166281/18671 | 0.057 | 1.207 | 0 | 32.000 |
| 1 | 32 | 3 | 120844 | 1.485 | 161311/18095 | 0.057 | 1.268 | 0 | 32.000 |
| 1 | 128 | 1 | 91324 | 2.976 | 244896/26897 | 0.300 | 2.274 | 0 | 32.000 |
| 1 | 128 | 2 | 94440 | 2.907 | 247361/27171 | 0.267 | 2.250 | 0 | 32.000 |
| 1 | 128 | 3 | 98534 | 2.930 | 260164/28570 | 0.283 | 2.216 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 12597.1 (32 workers), Weir 13913.2 (128 workers), observed change +10.45%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 10887.7 | 6356.9 | -41.61% | 0.383 / 1.407 / 3.583 | 1.151 / 2.815 / 4.095 |
| 32 | 12597.1 | 9246.9 | -26.60% | 1.279 / 4.607 / 45.055 | 3.071 / 7.167 / 10.239 |
| 128 | 12149.2 | 13913.2 | +14.52% | 5.631 / 53.247 / 65.535 | 9.215 / 18.431 / 26.623 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
