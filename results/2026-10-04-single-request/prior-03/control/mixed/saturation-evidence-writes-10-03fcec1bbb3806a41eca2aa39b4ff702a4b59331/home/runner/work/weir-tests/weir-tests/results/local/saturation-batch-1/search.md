# Database saturation comparison

Backend **search**. Started 2026-10-03T23:08:11.381423513Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 22932 | 2503 | 25435 | 0 | 1271.6 | 61.439 | 99.7 | 98.9 | 98.9 | true |
| 1 | 8 | 1 | weir | 29421 | 3257 | 32678 | 0 | 1633.6 | 11.263 | 99.9 | 98.9 | 98.9 | true |
| 1 | 8 | 2 | direct | 71656 | 8023 | 79679 | 0 | 3983.5 | 2.815 | 99.8 | 98.4 | 98.4 | true |
| 1 | 8 | 2 | weir | 42312 | 4712 | 47024 | 0 | 2350.9 | 5.631 | 96.5 | 99.0 | 99.0 | true |
| 1 | 8 | 3 | direct | 72536 | 8095 | 80631 | 0 | 4031.3 | 2.815 | 99.7 | 98.0 | 98.0 | true |
| 1 | 8 | 3 | weir | 54065 | 6057 | 60122 | 0 | 3005.8 | 4.607 | 90.3 | 39.0 | 99.9 | true |
| 1 | 32 | 1 | direct | 89476 | 10031 | 99507 | 0 | 4974.9 | 36.863 | 99.9 | 98.3 | 98.3 | true |
| 1 | 32 | 1 | weir | 71339 | 7978 | 79317 | 0 | 3964.6 | 15.359 | 97.0 | 94.7 | 94.7 | true |
| 1 | 32 | 2 | direct | 94699 | 10572 | 105271 | 0 | 5262.8 | 36.863 | 99.7 | 98.5 | 98.5 | true |
| 1 | 32 | 2 | weir | 67812 | 7572 | 75384 | 0 | 3768.2 | 15.359 | 97.2 | 95.0 | 95.0 | true |
| 1 | 32 | 3 | direct | 91408 | 10235 | 101643 | 0 | 5080.0 | 36.863 | 99.8 | 98.9 | 98.9 | true |
| 1 | 32 | 3 | weir | 71990 | 8053 | 80043 | 0 | 4000.7 | 15.359 | 97.2 | 94.6 | 94.6 | true |
| 1 | 128 | 1 | direct | 97906 | 11090 | 108996 | 0 | 5447.3 | 61.439 | 100.1 | 98.9 | 98.9 | true |
| 1 | 128 | 1 | weir | 74491 | 8413 | 82904 | 0 | 4139.8 | 53.247 | 98.9 | 95.0 | 95.0 | true |
| 1 | 128 | 2 | direct | 97841 | 11119 | 108960 | 0 | 5445.2 | 61.439 | 100.0 | 99.0 | 99.0 | true |
| 1 | 128 | 2 | weir | 78103 | 8851 | 86954 | 0 | 4343.3 | 45.055 | 98.5 | 95.2 | 95.2 | true |
| 1 | 128 | 3 | direct | 93590 | 10608 | 104198 | 0 | 5206.7 | 65.535 | 100.0 | 98.8 | 98.8 | true |
| 1 | 128 | 3 | weir | 77862 | 8800 | 86662 | 0 | 4328.7 | 45.055 | 98.7 | 95.0 | 95.0 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 32678 | 1.000 | 29421/3257 | 0.024 | 3.641 | 0 | 32.000 |
| 1 | 8 | 2 | 47024 | 1.000 | 42312/4712 | 0.024 | 2.207 | 0 | 32.000 |
| 1 | 8 | 3 | 60122 | 1.000 | 54065/6057 | 0.023 | 1.521 | 0 | 32.000 |
| 1 | 32 | 1 | 79317 | 1.000 | 71339/7978 | 0.201 | 4.053 | 0 | 32.000 |
| 1 | 32 | 2 | 75384 | 1.000 | 67812/7572 | 0.192 | 4.557 | 0 | 32.000 |
| 1 | 32 | 3 | 80043 | 1.000 | 71990/8053 | 0.200 | 4.041 | 0 | 32.000 |
| 1 | 128 | 1 | 82904 | 1.000 | 74491/8413 | 17.790 | 6.203 | 0 | 32.000 |
| 1 | 128 | 2 | 86954 | 1.000 | 78103/8851 | 16.637 | 5.797 | 0 | 32.000 |
| 1 | 128 | 3 | 86662 | 1.000 | 77862/8800 | 16.917 | 5.837 | 0 | 32.000 |

**Batch 1 at demonstrated database CPU saturation:** direct 5105.9 logical ops/s (32 workers), Weir 3911.2 logical ops/s (32 workers), Weir/direct 0.7660, change -23.40%.

Best verified business QPS within this ladder: direct 5366.4 (128 workers), Weir 4270.6 (128 workers), observed change -20.42%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 3095.5 | 2330.1 | -24.73% | 1.279 / 3.839 / 57.343 | 2.559 / 5.631 / 45.055 |
| 32 | 5105.9 | 3911.2 | -23.40% | 3.839 / 36.863 / 49.151 | 7.679 / 15.359 / 36.863 |
| 128 | 5366.4 | 4270.6 | -20.42% | 16.383 / 61.439 / 73.727 | 28.671 / 49.151 / 73.727 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
