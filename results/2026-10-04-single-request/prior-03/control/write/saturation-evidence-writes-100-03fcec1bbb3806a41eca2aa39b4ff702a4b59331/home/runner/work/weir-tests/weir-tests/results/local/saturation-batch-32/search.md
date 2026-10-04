# Database saturation comparison

Backend **search**. Started 2026-10-03T23:27:21.659384102Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 21017 | 21017 | 0 | 1049.4 | 40.959 | 88.7 | 63.3 | 100.0 | true |
| 1 | 8 | 1 | weir | 0 | 25815 | 25815 | 0 | 1290.5 | 10.239 | 63.6 | 5.2 | 99.3 | true |
| 1 | 8 | 2 | direct | 0 | 28640 | 28640 | 0 | 1431.8 | 5.631 | 47.9 | 0.0 | 98.9 | true |
| 1 | 8 | 2 | weir | 0 | 25826 | 25826 | 0 | 1280.4 | 8.191 | 46.3 | 0.0 | 98.8 | true |
| 1 | 8 | 3 | direct | 0 | 26234 | 26234 | 0 | 1311.5 | 7.167 | 38.6 | 0.0 | 98.8 | true |
| 1 | 8 | 3 | weir | 0 | 31765 | 31765 | 0 | 1587.9 | 6.143 | 56.1 | 5.2 | 99.7 | true |
| 1 | 32 | 1 | direct | 0 | 34251 | 34251 | 0 | 1711.4 | 53.247 | 50.3 | 0.0 | 98.8 | true |
| 1 | 32 | 1 | weir | 0 | 24655 | 24655 | 0 | 1231.8 | 73.727 | 41.2 | 0.0 | 99.1 | true |
| 1 | 32 | 2 | direct | 0 | 36126 | 36126 | 0 | 1804.9 | 36.863 | 51.5 | 0.0 | 98.9 | true |
| 1 | 32 | 2 | weir | 0 | 23773 | 23773 | 0 | 1175.7 | 81.919 | 40.1 | 0.0 | 98.0 | true |
| 1 | 32 | 3 | direct | 0 | 26620 | 26620 | 0 | 1330.1 | 81.919 | 39.0 | 0.0 | 98.7 | true |
| 1 | 32 | 3 | weir | 0 | 23378 | 23378 | 0 | 1168.0 | 81.919 | 38.2 | 0.0 | 98.9 | true |
| 1 | 128 | 1 | direct | 0 | 32225 | 32225 | 0 | 1560.6 | 229.375 | 44.1 | 0.0 | 95.9 | true |
| 1 | 128 | 1 | weir | 0 | 104657 | 104657 | 0 | 5225.7 | 45.055 | 57.6 | 0.0 | 95.1 | true |
| 1 | 128 | 2 | direct | 0 | 26687 | 26687 | 0 | 1322.3 | 294.911 | 38.0 | 0.0 | 97.7 | true |
| 1 | 128 | 2 | weir | 0 | 82224 | 82224 | 0 | 4107.8 | 98.303 | 45.9 | 0.0 | 99.7 | true |
| 1 | 128 | 3 | direct | 0 | 27301 | 27301 | 0 | 1360.9 | 229.375 | 36.3 | 0.0 | 98.5 | true |
| 1 | 128 | 3 | weir | 0 | 91254 | 91254 | 0 | 4473.5 | 73.727 | 47.4 | 0.0 | 97.9 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 25756 | 1.002 | 0/25815 | 0.007 | 5.872 | 0 | 32.000 |
| 1 | 8 | 2 | 25775 | 1.002 | 0/25826 | 0.007 | 5.934 | 0 | 32.000 |
| 1 | 8 | 3 | 31700 | 1.002 | 0/31765 | 0.007 | 4.733 | 0 | 32.000 |
| 1 | 32 | 1 | 24570 | 1.003 | 0/24655 | 0.009 | 25.553 | 0 | 32.000 |
| 1 | 32 | 2 | 23724 | 1.002 | 0/23773 | 0.008 | 26.765 | 0 | 32.000 |
| 1 | 32 | 3 | 23349 | 1.001 | 0/23378 | 0.008 | 27.097 | 0 | 32.000 |
| 1 | 128 | 1 | 28129 | 3.721 | 0/104657 | 0.546 | 22.629 | 0 | 32.000 |
| 1 | 128 | 2 | 22137 | 3.714 | 0/82224 | 0.889 | 28.767 | 0 | 32.000 |
| 1 | 128 | 3 | 24435 | 3.735 | 0/91254 | 0.798 | 26.587 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 1615.5 (32 workers), Weir 4601.7 (128 workers), observed change +184.85%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 1264.2 | 1386.0 | +9.64% | 3.583 / 10.239 / 81.919 | 4.095 / 7.679 / 57.343 |
| 32 | 1615.5 | 1191.8 | -26.23% | 14.335 / 53.247 / 122.879 | 18.431 / 81.919 / 229.375 |
| 128 | 1415.8 | 4601.7 | +225.02% | 61.439 / 245.759 / 491.519 | 20.479 / 73.727 / 131.071 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
