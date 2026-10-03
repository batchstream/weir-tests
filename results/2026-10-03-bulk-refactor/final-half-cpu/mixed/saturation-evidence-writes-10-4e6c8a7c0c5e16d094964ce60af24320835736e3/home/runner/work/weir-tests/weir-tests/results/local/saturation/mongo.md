# Database saturation comparison

Backend **mongo**. Started 2026-10-03T12:38:12.754237636Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 925504 | 103008 | 1028512 | 0 | 51423.9 | 10.239 | 98.9 | 95.1 | 95.1 | true |
| 32 | 8 | 1 | weir | 723808 | 80160 | 803968 | 0 | 40053.3 | 49.151 | 99.8 | 99.0 | 99.0 | true |
| 32 | 8 | 2 | direct | 735648 | 81472 | 817120 | 0 | 40669.0 | 81.919 | 98.6 | 95.0 | 95.0 | true |
| 32 | 8 | 2 | weir | 663840 | 73344 | 737184 | 0 | 36854.7 | 49.151 | 99.8 | 100.0 | 100.0 | true |
| 32 | 8 | 3 | direct | 660000 | 72480 | 732480 | 0 | 36623.1 | 81.919 | 99.3 | 95.6 | 95.6 | true |
| 32 | 8 | 3 | weir | 632032 | 68576 | 700608 | 0 | 35026.8 | 49.151 | 99.6 | 94.9 | 94.9 | true |
| 32 | 32 | 1 | direct | 707328 | 81760 | 789088 | 0 | 39446.6 | 98.303 | 99.7 | 98.5 | 98.5 | true |
| 32 | 32 | 1 | weir | 649632 | 74304 | 723936 | 0 | 36183.4 | 90.111 | 99.7 | 95.8 | 95.8 | true |
| 32 | 32 | 2 | direct | 702720 | 81600 | 784320 | 0 | 38626.8 | 98.303 | 99.9 | 97.5 | 97.5 | true |
| 32 | 32 | 2 | weir | 638720 | 73376 | 712096 | 0 | 35593.9 | 90.111 | 100.2 | 96.1 | 96.1 | true |
| 32 | 32 | 3 | direct | 699616 | 80640 | 780256 | 0 | 38999.3 | 98.303 | 99.9 | 98.4 | 98.4 | true |
| 32 | 32 | 3 | weir | 574880 | 65792 | 640672 | 0 | 32023.9 | 98.303 | 99.9 | 96.9 | 96.9 | true |
| 32 | 64 | 1 | direct | 676480 | 72736 | 749216 | 0 | 37446.0 | 114.687 | 100.0 | 95.0 | 95.0 | true |
| 32 | 64 | 1 | weir | 662112 | 70688 | 732800 | 0 | 36616.0 | 106.495 | 100.0 | 96.5 | 96.5 | true |
| 32 | 64 | 2 | direct | 703680 | 76096 | 779776 | 0 | 38963.5 | 114.687 | 100.0 | 98.0 | 98.0 | true |
| 32 | 64 | 2 | weir | 665248 | 71104 | 736352 | 0 | 36655.8 | 106.495 | 99.8 | 97.7 | 97.7 | true |
| 32 | 64 | 3 | direct | 653728 | 70336 | 724064 | 0 | 36027.1 | 114.687 | 99.4 | 95.4 | 95.4 | true |
| 32 | 64 | 3 | weir | 640704 | 67936 | 708640 | 0 | 35412.8 | 106.495 | 100.1 | 96.5 | 96.5 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 25124 | 32.000 | 22619/2505 | 0.020 | 4.723 | 0 | 32.000 |
| 32 | 8 | 2 | 23037 | 32.000 | 20745/2292 | 0.019 | 5.261 | 0 | 32.000 |
| 32 | 8 | 3 | 21894 | 32.000 | 19751/2143 | 0.018 | 5.709 | 0 | 32.000 |
| 32 | 32 | 1 | 22623 | 32.000 | 20301/2322 | 0.140 | 22.456 | 0 | 32.000 |
| 32 | 32 | 2 | 22253 | 32.000 | 19960/2293 | 0.132 | 22.961 | 0 | 32.000 |
| 32 | 32 | 3 | 20021 | 32.000 | 17965/2056 | 0.130 | 26.030 | 0 | 32.000 |
| 32 | 64 | 1 | 22900 | 32.000 | 20691/2209 | 20.846 | 25.486 | 0 | 32.000 |
| 32 | 64 | 2 | 23011 | 32.000 | 20789/2222 | 20.510 | 25.453 | 0 | 32.000 |
| 32 | 64 | 3 | 22145 | 32.000 | 20022/2123 | 21.258 | 26.340 | 0 | 32.000 |

**Batch 32 at demonstrated database CPU saturation:** direct 42901.9 logical ops/s (8 workers), Weir 37314.8 logical ops/s (8 workers), Weir/direct 0.8698, change -13.02%.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
