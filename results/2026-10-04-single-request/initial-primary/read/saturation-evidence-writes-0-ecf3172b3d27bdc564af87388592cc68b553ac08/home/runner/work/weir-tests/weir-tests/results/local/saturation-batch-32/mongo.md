# Database saturation comparison

Backend **mongo**. Started 2026-10-03T22:23:27.865143527Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 324296 | 0 | 324296 | 0 | 16214.2 | 0.831 | 100.0 | 96.1 | 96.1 | true |
| 1 | 8 | 1 | weir | 139189 | 0 | 139189 | 0 | 6959.1 | 2.559 | 65.7 | 0.0 | 96.0 | true |
| 1 | 8 | 2 | direct | 318334 | 0 | 318334 | 0 | 15916.2 | 0.831 | 100.0 | 96.9 | 96.9 | true |
| 1 | 8 | 2 | weir | 138285 | 0 | 138285 | 0 | 6913.7 | 2.559 | 65.2 | 0.0 | 96.5 | true |
| 1 | 8 | 3 | direct | 317283 | 0 | 317283 | 0 | 15861.4 | 0.831 | 100.1 | 97.5 | 97.5 | true |
| 1 | 8 | 3 | weir | 136127 | 0 | 136127 | 0 | 6806.0 | 2.559 | 63.8 | 0.0 | 96.2 | true |
| 1 | 32 | 1 | direct | 387411 | 0 | 387411 | 0 | 19369.7 | 3.583 | 100.0 | 97.7 | 97.7 | true |
| 1 | 32 | 1 | weir | 215227 | 0 | 215227 | 0 | 10759.8 | 6.143 | 69.3 | 0.0 | 96.2 | true |
| 1 | 32 | 2 | direct | 380024 | 0 | 380024 | 0 | 18999.6 | 3.583 | 100.0 | 97.5 | 97.5 | true |
| 1 | 32 | 2 | weir | 201013 | 0 | 201013 | 0 | 10049.6 | 6.655 | 69.1 | 0.0 | 96.6 | true |
| 1 | 32 | 3 | direct | 338098 | 0 | 338098 | 0 | 16903.9 | 4.095 | 100.1 | 98.8 | 98.8 | true |
| 1 | 32 | 3 | weir | 198139 | 0 | 198139 | 0 | 9906.4 | 6.655 | 68.7 | 0.0 | 96.5 | true |
| 1 | 128 | 1 | direct | 300898 | 0 | 300898 | 0 | 15041.5 | 40.959 | 100.1 | 97.0 | 97.0 | true |
| 1 | 128 | 1 | weir | 317925 | 0 | 317925 | 0 | 15890.5 | 16.383 | 56.6 | 0.0 | 96.7 | true |
| 1 | 128 | 2 | direct | 343556 | 0 | 343556 | 0 | 17174.0 | 40.959 | 100.1 | 96.9 | 96.9 | true |
| 1 | 128 | 2 | weir | 332562 | 0 | 332562 | 0 | 16625.7 | 16.383 | 57.5 | 0.0 | 96.4 | true |
| 1 | 128 | 3 | direct | 350122 | 0 | 350122 | 0 | 17502.3 | 36.863 | 100.1 | 97.3 | 97.3 | true |
| 1 | 128 | 3 | weir | 356034 | 0 | 356034 | 0 | 17798.7 | 14.335 | 58.9 | 0.0 | 96.2 | true |
| 1 | 512 | 1 | direct | 288562 | 0 | 288562 | 0 | 14416.2 | 81.919 | 100.2 | 98.8 | 98.8 | true |
| 1 | 512 | 1 | weir | 493970 | 0 | 493970 | 0 | 24683.0 | 40.959 | 40.6 | 0.0 | 96.0 | true |
| 1 | 512 | 2 | direct | 276670 | 0 | 276670 | 0 | 13816.1 | 81.919 | 100.2 | 94.9 | 94.9 | true |
| 1 | 512 | 2 | weir | 492414 | 0 | 492414 | 0 | 24601.2 | 40.959 | 40.0 | 0.0 | 96.4 | true |
| 1 | 512 | 3 | direct | 290666 | 0 | 290666 | 0 | 14516.0 | 81.919 | 100.2 | 98.7 | 98.7 | true |
| 1 | 512 | 3 | weir | 498895 | 0 | 498895 | 0 | 24932.0 | 40.959 | 39.9 | 0.0 | 96.1 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 125233 | 1.111 | 139189/0 | 0.012 | 0.483 | 0 | 32.000 |
| 1 | 8 | 2 | 124721 | 1.109 | 138285/0 | 0.011 | 0.484 | 0 | 32.000 |
| 1 | 8 | 3 | 123176 | 1.105 | 136127/0 | 0.011 | 0.492 | 0 | 32.000 |
| 1 | 32 | 1 | 135801 | 1.585 | 215227/0 | 0.061 | 0.937 | 0 | 32.000 |
| 1 | 32 | 2 | 125088 | 1.607 | 201013/0 | 0.068 | 1.002 | 0 | 32.000 |
| 1 | 32 | 3 | 121378 | 1.632 | 198139/0 | 0.072 | 1.019 | 0 | 32.000 |
| 1 | 128 | 1 | 88342 | 3.599 | 317925/0 | 0.313 | 1.838 | 0 | 32.000 |
| 1 | 128 | 2 | 94676 | 3.513 | 332562/0 | 0.293 | 1.769 | 0 | 32.000 |
| 1 | 128 | 3 | 102478 | 3.474 | 356034/0 | 0.265 | 1.652 | 0 | 32.000 |
| 1 | 512 | 1 | 48301 | 10.227 | 493970/0 | 1.331 | 3.362 | 0 | 32.000 |
| 1 | 512 | 2 | 43466 | 11.329 | 492414/0 | 1.363 | 3.492 | 0 | 32.000 |
| 1 | 512 | 3 | 43434 | 11.486 | 498895/0 | 1.375 | 3.491 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 18424.4 (32 workers), Weir 24738.7 (512 workers), observed change +34.27%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 15997.2 | 6892.9 | -56.91% | 0.351 / 0.831 / 1.791 | 1.023 / 2.559 / 3.583 |
| 32 | 18424.4 | 10238.6 | -44.43% | 0.959 / 3.839 / 32.767 | 2.815 / 6.655 / 9.215 |
| 128 | 16572.6 | 16771.6 | +1.20% | 4.607 / 40.959 / 49.151 | 7.167 / 15.359 / 20.479 |
| 512 | 14249.4 | 24738.7 | +73.61% | 26.623 / 81.919 / 106.495 | 20.479 / 40.959 / 53.247 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
