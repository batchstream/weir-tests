# Database saturation comparison

Backend **mongo**. Started 2026-10-03T22:58:58.520556174Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 90385 | 10179 | 100564 | 0 | 5027.9 | 2.303 | 99.6 | 98.5 | 98.5 | true |
| 1 | 8 | 1 | weir | 56547 | 6363 | 62910 | 0 | 3145.1 | 5.119 | 69.8 | 0.0 | 95.2 | true |
| 1 | 8 | 2 | direct | 89724 | 10094 | 99818 | 0 | 4990.5 | 2.559 | 99.6 | 99.0 | 99.0 | true |
| 1 | 8 | 2 | weir | 64709 | 7265 | 71974 | 0 | 3598.3 | 4.095 | 80.0 | 0.0 | 95.0 | true |
| 1 | 8 | 3 | direct | 89553 | 10090 | 99643 | 0 | 4981.9 | 2.559 | 99.6 | 98.4 | 98.4 | true |
| 1 | 8 | 3 | weir | 64566 | 7233 | 71799 | 0 | 3589.6 | 4.095 | 79.7 | 0.0 | 95.1 | true |
| 1 | 32 | 1 | direct | 94806 | 10605 | 105411 | 0 | 5269.7 | 40.959 | 100.0 | 98.8 | 98.8 | true |
| 1 | 32 | 1 | weir | 78801 | 8812 | 87613 | 0 | 4379.3 | 14.335 | 94.7 | 95.2 | 95.2 | true |
| 1 | 32 | 2 | direct | 94353 | 10553 | 104906 | 0 | 5244.2 | 40.959 | 100.0 | 99.0 | 99.0 | true |
| 1 | 32 | 2 | weir | 78140 | 8743 | 86883 | 0 | 4343.2 | 14.335 | 94.4 | 95.5 | 95.5 | true |
| 1 | 32 | 3 | direct | 95245 | 10645 | 105890 | 0 | 5293.5 | 40.959 | 100.0 | 98.0 | 98.0 | true |
| 1 | 32 | 3 | weir | 77138 | 8623 | 85761 | 0 | 4287.0 | 14.335 | 93.4 | 84.3 | 94.9 | true |
| 1 | 128 | 1 | direct | 89859 | 10208 | 100067 | 0 | 5000.2 | 73.727 | 100.1 | 98.4 | 98.4 | true |
| 1 | 128 | 1 | weir | 81952 | 9294 | 91246 | 0 | 4558.3 | 45.055 | 98.5 | 95.7 | 95.7 | true |
| 1 | 128 | 2 | direct | 88948 | 10113 | 99061 | 0 | 4949.7 | 73.727 | 100.2 | 99.3 | 99.3 | true |
| 1 | 128 | 2 | weir | 80555 | 9143 | 89698 | 0 | 4480.8 | 45.055 | 99.0 | 95.8 | 95.8 | true |
| 1 | 128 | 3 | direct | 87743 | 9954 | 97697 | 0 | 4871.7 | 73.727 | 100.1 | 99.0 | 99.0 | true |
| 1 | 128 | 3 | weir | 81737 | 9267 | 91004 | 0 | 4544.8 | 45.055 | 98.5 | 95.5 | 95.5 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 62910 | 1.000 | 56547/6363 | 0.032 | 1.079 | 0 | 32.000 |
| 1 | 8 | 2 | 71974 | 1.000 | 64709/7265 | 0.030 | 0.989 | 0 | 32.000 |
| 1 | 8 | 3 | 71799 | 1.000 | 64566/7233 | 0.029 | 1.000 | 0 | 32.000 |
| 1 | 32 | 1 | 87613 | 1.000 | 78801/8812 | 0.305 | 2.517 | 0 | 32.000 |
| 1 | 32 | 2 | 86883 | 1.000 | 78140/8743 | 0.313 | 2.521 | 0 | 32.000 |
| 1 | 32 | 3 | 85761 | 1.000 | 77138/8623 | 0.300 | 2.674 | 0 | 32.000 |
| 1 | 128 | 1 | 91246 | 1.000 | 81952/9294 | 14.427 | 4.690 | 0 | 32.000 |
| 1 | 128 | 2 | 89698 | 1.000 | 80555/9143 | 15.104 | 4.756 | 0 | 32.000 |
| 1 | 128 | 3 | 91004 | 1.000 | 81737/9267 | 14.491 | 4.621 | 0 | 32.000 |

**Batch 1 at demonstrated database CPU saturation:** direct 5269.1 logical ops/s (32 workers), Weir 4336.5 logical ops/s (32 workers), Weir/direct 0.8230, change -17.70%.

Best verified business QPS within this ladder: direct 5269.1 (32 workers), Weir 4528.0 (128 workers), observed change -14.07%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 5000.1 | 3444.3 | -31.11% | 0.895 / 2.559 / 40.959 | 2.303 / 4.607 / 6.143 |
| 32 | 5269.1 | 4336.5 | -17.70% | 3.327 / 40.959 / 53.247 | 7.167 / 14.335 / 18.431 |
| 128 | 4940.5 | 4528.0 | -8.35% | 16.383 / 73.727 / 90.111 | 28.671 / 45.055 / 57.343 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
