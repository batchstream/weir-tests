# Database saturation comparison

Backend **search**. Started 2026-10-03T22:36:01.541666558Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 15128 | 15128 | 0 | 756.2 | 57.343 | 100.0 | 98.9 | 98.9 | true |
| 1 | 8 | 1 | weir | 0 | 21786 | 21786 | 0 | 1089.0 | 45.055 | 98.2 | 92.6 | 97.9 | true |
| 1 | 8 | 2 | direct | 0 | 39164 | 39164 | 0 | 1957.9 | 4.607 | 86.0 | 10.5 | 95.5 | true |
| 1 | 8 | 2 | weir | 0 | 36454 | 36454 | 0 | 1822.4 | 5.631 | 85.2 | 27.0 | 96.6 | true |
| 1 | 8 | 3 | direct | 0 | 42139 | 42139 | 0 | 2106.6 | 4.095 | 83.9 | 10.6 | 95.4 | true |
| 1 | 8 | 3 | weir | 0 | 38080 | 38080 | 0 | 1903.7 | 5.119 | 81.1 | 5.3 | 96.4 | true |
| 1 | 32 | 1 | direct | 0 | 43298 | 43298 | 0 | 2163.5 | 16.383 | 83.6 | 0.0 | 95.1 | true |
| 1 | 32 | 1 | weir | 0 | 39216 | 39216 | 0 | 1959.5 | 20.479 | 81.1 | 0.0 | 97.1 | true |
| 1 | 32 | 2 | direct | 0 | 44240 | 44240 | 0 | 2210.6 | 16.383 | 84.1 | 10.6 | 95.2 | true |
| 1 | 32 | 2 | weir | 0 | 42287 | 42287 | 0 | 2113.0 | 18.431 | 82.9 | 5.4 | 97.9 | true |
| 1 | 32 | 3 | direct | 0 | 44406 | 44406 | 0 | 2218.9 | 16.383 | 82.4 | 5.3 | 95.2 | true |
| 1 | 32 | 3 | weir | 0 | 39682 | 39682 | 0 | 1982.9 | 18.431 | 80.4 | 0.0 | 96.7 | true |
| 1 | 128 | 1 | direct | 0 | 47088 | 47088 | 0 | 2348.7 | 65.535 | 83.4 | 0.0 | 95.1 | true |
| 1 | 128 | 1 | weir | 0 | 115152 | 115152 | 0 | 5752.9 | 36.863 | 83.6 | 11.1 | 99.4 | true |
| 1 | 128 | 2 | direct | 0 | 43472 | 43472 | 0 | 2168.0 | 98.303 | 85.7 | 15.9 | 95.2 | true |
| 1 | 128 | 2 | weir | 0 | 120809 | 120809 | 0 | 6035.6 | 30.719 | 82.3 | 10.8 | 98.8 | true |
| 1 | 128 | 3 | direct | 0 | 48211 | 48211 | 0 | 2404.9 | 61.439 | 0.0 | 0.0 | 0.0 | true |
| 1 | 128 | 3 | weir | 0 | 119861 | 119861 | 0 | 5988.6 | 28.671 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 1 | direct | 0 | 48642 | 48642 | 0 | 2409.6 | 294.911 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 1 | weir | 0 | 248751 | 248751 | 0 | 12418.5 | 73.727 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 2 | direct | 0 | 46792 | 46792 | 0 | 2318.0 | 327.679 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 2 | weir | 0 | 250422 | 250422 | 0 | 12502.1 | 73.727 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 3 | direct | 0 | 49464 | 49464 | 0 | 2450.7 | 245.759 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 3 | weir | 0 | 239388 | 239388 | 0 | 11959.5 | 81.919 | 0.0 | 0.0 | 0.0 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 21403 | 1.018 | 0/21786 | 0.012 | 6.699 | 0 | 32.000 |
| 1 | 8 | 2 | 35973 | 1.013 | 0/36454 | 0.010 | 3.856 | 0 | 32.000 |
| 1 | 8 | 3 | 37739 | 1.009 | 0/38080 | 0.009 | 3.726 | 0 | 32.000 |
| 1 | 32 | 1 | 38679 | 1.014 | 0/39216 | 0.012 | 15.823 | 0 | 32.000 |
| 1 | 32 | 2 | 41471 | 1.020 | 0/42287 | 0.013 | 14.582 | 0 | 32.000 |
| 1 | 32 | 3 | 39269 | 1.011 | 0/39682 | 0.012 | 15.644 | 0 | 32.000 |
| 1 | 128 | 1 | 33009 | 3.489 | 0/115152 | 0.566 | 18.694 | 0 | 32.000 |
| 1 | 128 | 2 | 34524 | 3.499 | 0/120809 | 0.429 | 18.000 | 0 | 32.000 |
| 1 | 128 | 3 | 34053 | 3.520 | 0/119861 | 0.470 | 18.308 | 0 | 32.000 |
| 1 | 512 | 1 | 23514 | 10.579 | 0/248751 | 2.411 | 18.436 | 0 | 32.000 |
| 1 | 512 | 2 | 23875 | 10.489 | 0/250422 | 2.486 | 16.965 | 0 | 32.000 |
| 1 | 512 | 3 | 22066 | 10.849 | 0/239388 | 2.706 | 19.836 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 2392.8 (512 workers), Weir 12293.4 (512 workers), observed change +413.77%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 1606.9 | 1605.0 | -0.12% | 3.839 / 6.143 / 57.343 | 4.095 / 6.143 / 49.151 |
| 32 | 2197.7 | 2018.5 | -8.15% | 14.335 / 16.383 / 32.767 | 16.383 / 18.431 / 36.863 |
| 128 | 2307.2 | 5925.7 | +156.83% | 53.247 / 73.727 / 114.687 | 20.479 / 30.719 / 65.535 |
| 512 | 2392.8 | 12293.4 | +413.77% | 212.991 / 294.911 / 425.983 | 40.959 / 73.727 / 98.303 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
