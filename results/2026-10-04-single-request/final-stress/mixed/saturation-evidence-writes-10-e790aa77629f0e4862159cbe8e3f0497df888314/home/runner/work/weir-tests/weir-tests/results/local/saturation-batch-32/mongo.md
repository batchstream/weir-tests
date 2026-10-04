# Database saturation comparison

Backend **mongo**. Started 2026-10-04T00:12:36.741836442Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 91062 | 10229 | 101291 | 0 | 5064.2 | 2.303 | 99.7 | 98.3 | 98.3 | true |
| 1 | 8 | 1 | weir | 60848 | 6829 | 67677 | 0 | 3383.4 | 4.607 | 68.8 | 0.0 | 94.8 | true |
| 1 | 8 | 2 | direct | 91597 | 10284 | 101881 | 0 | 5093.6 | 2.303 | 99.6 | 99.3 | 99.3 | true |
| 1 | 8 | 2 | weir | 67152 | 7525 | 74677 | 0 | 3733.5 | 4.095 | 76.4 | 0.0 | 99.7 | true |
| 1 | 8 | 3 | direct | 91334 | 10254 | 101588 | 0 | 5079.1 | 2.303 | 99.4 | 98.0 | 98.0 | true |
| 1 | 8 | 3 | weir | 68215 | 7635 | 75850 | 0 | 3792.0 | 3.839 | 77.6 | 0.0 | 99.6 | true |
| 1 | 32 | 1 | direct | 95931 | 10733 | 106664 | 0 | 5332.2 | 40.959 | 100.0 | 99.0 | 99.0 | true |
| 1 | 32 | 1 | weir | 96930 | 10855 | 107785 | 0 | 5388.3 | 12.287 | 84.9 | 5.4 | 94.7 | true |
| 1 | 32 | 2 | direct | 94185 | 10518 | 104703 | 0 | 5234.3 | 40.959 | 99.9 | 99.0 | 99.0 | true |
| 1 | 32 | 2 | weir | 96969 | 10820 | 107789 | 0 | 5388.7 | 12.287 | 84.9 | 0.0 | 95.3 | true |
| 1 | 32 | 3 | direct | 96941 | 10854 | 107795 | 0 | 5388.8 | 40.959 | 100.0 | 97.8 | 97.8 | true |
| 1 | 32 | 3 | weir | 95940 | 10749 | 106689 | 0 | 5333.2 | 12.287 | 84.9 | 0.0 | 95.5 | true |
| 1 | 128 | 1 | direct | 93603 | 10643 | 104246 | 0 | 5208.6 | 65.535 | 100.2 | 99.0 | 99.0 | true |
| 1 | 128 | 1 | weir | 145245 | 16130 | 161375 | 0 | 8065.8 | 32.767 | 77.5 | 0.0 | 95.1 | true |
| 1 | 128 | 2 | direct | 95651 | 10895 | 106546 | 0 | 5323.8 | 65.535 | 100.2 | 99.2 | 99.2 | true |
| 1 | 128 | 2 | weir | 146317 | 16288 | 162605 | 0 | 8126.1 | 30.719 | 78.9 | 0.0 | 94.4 | true |
| 1 | 128 | 3 | direct | 97062 | 11050 | 108112 | 0 | 5401.9 | 65.535 | 100.1 | 99.4 | 99.4 | true |
| 1 | 128 | 3 | weir | 142904 | 15883 | 158787 | 0 | 7933.1 | 32.767 | 78.3 | 0.0 | 94.8 | true |
| 1 | 512 | 1 | direct | 95914 | 10445 | 106359 | 0 | 5291.2 | 196.607 | 100.0 | 95.4 | 95.4 | true |
| 1 | 512 | 1 | weir | 211680 | 23668 | 235348 | 0 | 11752.0 | 90.111 | 66.9 | 0.0 | 94.9 | true |
| 1 | 512 | 2 | direct | 89086 | 9670 | 98756 | 0 | 4914.0 | 212.991 | 100.1 | 95.4 | 95.4 | true |
| 1 | 512 | 2 | weir | 205531 | 22979 | 228510 | 0 | 11411.8 | 90.111 | 67.3 | 0.0 | 95.0 | true |
| 1 | 512 | 3 | direct | 92141 | 10033 | 102174 | 0 | 5084.8 | 196.607 | 100.0 | 95.4 | 95.4 | true |
| 1 | 512 | 3 | weir | 206476 | 23080 | 229556 | 0 | 11467.2 | 90.111 | 69.1 | 0.0 | 94.5 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 61496 | 1.101 | 60848/6829 | 0.026 | 1.019 | 0 | 32.000 |
| 1 | 8 | 2 | 67616 | 1.104 | 67152/7525 | 0.025 | 0.968 | 0 | 32.000 |
| 1 | 8 | 3 | 68276 | 1.111 | 68215/7635 | 0.025 | 0.948 | 0 | 32.000 |
| 1 | 32 | 1 | 65670 | 1.641 | 96930/10855 | 0.139 | 1.929 | 0 | 32.000 |
| 1 | 32 | 2 | 65806 | 1.638 | 96969/10820 | 0.138 | 1.901 | 0 | 32.000 |
| 1 | 32 | 3 | 65285 | 1.634 | 95940/10749 | 0.138 | 1.943 | 0 | 32.000 |
| 1 | 128 | 1 | 47827 | 3.374 | 145245/16130 | 0.573 | 3.598 | 0 | 32.000 |
| 1 | 128 | 2 | 47048 | 3.456 | 146317/16288 | 0.580 | 3.738 | 0 | 32.000 |
| 1 | 128 | 3 | 47634 | 3.333 | 142904/15883 | 0.571 | 3.820 | 0 | 32.000 |
| 1 | 512 | 1 | 24276 | 9.695 | 211680/23668 | 2.622 | 8.151 | 0 | 32.000 |
| 1 | 512 | 2 | 23011 | 9.930 | 205531/22979 | 2.695 | 8.621 | 0 | 32.000 |
| 1 | 512 | 3 | 23584 | 9.734 | 206476/23080 | 2.568 | 8.514 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 5318.4 (32 workers), Weir 11543.7 (512 workers), observed change +117.05%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 5079.0 | 3636.3 | -28.41% | 0.895 / 2.303 / 40.959 | 2.047 / 4.095 / 5.631 |
| 32 | 5318.4 | 5370.1 | +0.97% | 3.327 / 40.959 / 53.247 | 5.631 / 12.287 / 15.359 |
| 128 | 5311.4 | 8041.7 | +51.40% | 15.359 / 65.535 / 81.919 | 15.359 / 32.767 / 45.055 |
| 512 | 5096.7 | 11543.7 | +126.49% | 98.303 / 196.607 / 327.679 | 45.055 / 90.111 / 114.687 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
