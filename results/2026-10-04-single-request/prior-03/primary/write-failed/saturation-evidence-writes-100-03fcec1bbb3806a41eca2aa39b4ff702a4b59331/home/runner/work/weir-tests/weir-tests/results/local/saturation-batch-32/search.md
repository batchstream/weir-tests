# Database saturation comparison

Backend **search**. Started 2026-10-03T23:10:38.51179695Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 26418 | 26418 | 0 | 1320.6 | 16.383 | 82.0 | 52.3 | 99.2 | true |
| 1 | 8 | 1 | weir | 0 | 24830 | 24830 | 0 | 1241.3 | 12.287 | 52.1 | 5.2 | 99.0 | true |
| 1 | 8 | 2 | direct | 0 | 38803 | 38803 | 0 | 1939.8 | 4.607 | 49.3 | 0.0 | 99.3 | true |
| 1 | 8 | 2 | weir | 0 | 33096 | 33096 | 0 | 1654.5 | 5.119 | 53.2 | 0.0 | 94.8 | true |
| 1 | 8 | 3 | direct | 0 | 38487 | 38487 | 0 | 1924.0 | 4.607 | 43.6 | 0.0 | 98.8 | true |
| 1 | 8 | 3 | weir | 0 | 35837 | 35837 | 0 | 1791.5 | 5.119 | 51.4 | 0.0 | 99.4 | true |
| 1 | 32 | 1 | direct | 0 | 42056 | 42056 | 0 | 2101.4 | 18.431 | 46.6 | 0.0 | 98.8 | true |
| 1 | 32 | 1 | weir | 0 | 36099 | 36099 | 0 | 1803.6 | 22.527 | 53.5 | 0.0 | 99.6 | true |
| 1 | 32 | 2 | direct | 0 | 39164 | 39164 | 0 | 1956.8 | 18.431 | 42.4 | 0.0 | 98.7 | true |
| 1 | 32 | 2 | weir | 0 | 27975 | 27975 | 0 | 1397.4 | 36.863 | 42.9 | 0.0 | 99.3 | true |
| 1 | 32 | 3 |  | 0 | 0 | 0 | 0 | 0.0 | 0.000 | 0.0 | 0.0 | 0.0 | false |
| 1 | 32 | 3 | weir | 0 | 16987 | 16978 | 9 | 835.0 | 106.495 | 24.8 | 0.0 | 97.0 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 24795 | 1.001 | 0/24830 | 0.006 | 6.177 | 0 | 32.000 |
| 1 | 8 | 2 | 33063 | 1.001 | 0/33096 | 0.006 | 4.565 | 0 | 32.000 |
| 1 | 8 | 3 | 35796 | 1.001 | 0/35837 | 0.006 | 4.206 | 0 | 32.000 |
| 1 | 32 | 1 | 36036 | 1.002 | 0/36099 | 0.008 | 17.479 | 0 | 32.000 |
| 1 | 32 | 2 | 27903 | 1.003 | 0/27975 | 0.008 | 22.610 | 0 | 32.000 |
| 1 | 32 | 3 | 16940 | 1.003 | 0/16987 | 0.007 | 34.321 | 0 | 32.000 |

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.

Incomplete: weir single-request stage failed: exit status 1 exit status 1 exit status 1 exit status 1 
