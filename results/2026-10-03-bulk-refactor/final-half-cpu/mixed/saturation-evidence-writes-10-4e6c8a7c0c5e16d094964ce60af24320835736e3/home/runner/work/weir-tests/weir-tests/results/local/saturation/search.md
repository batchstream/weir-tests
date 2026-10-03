# Database saturation comparison

Backend **search**. Started 2026-10-03T12:47:18.321986058Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 143520 | 16992 | 160512 | 0 | 8024.1 | 98.303 | 99.8 | 99.0 | 99.0 | true |
| 32 | 8 | 1 | weir | 153568 | 18208 | 171776 | 0 | 8586.5 | 90.111 | 99.6 | 98.4 | 98.4 | true |
| 32 | 8 | 2 | direct | 367328 | 40704 | 408032 | 0 | 20399.1 | 81.919 | 99.6 | 97.6 | 97.6 | true |
| 32 | 8 | 2 | weir | 190080 | 22112 | 212192 | 0 | 10608.4 | 90.111 | 99.4 | 97.9 | 97.9 | true |
| 32 | 8 | 3 | direct | 481152 | 51872 | 533024 | 0 | 26648.5 | 73.727 | 99.2 | 96.0 | 96.0 | true |
| 32 | 8 | 3 | weir | 310752 | 34112 | 344864 | 0 | 17194.7 | 81.919 | 99.8 | 95.6 | 95.6 | true |
| 32 | 32 | 1 | direct | 648032 | 74624 | 722656 | 0 | 36121.4 | 90.111 | 99.3 | 97.4 | 97.4 | true |
| 32 | 32 | 1 | weir | 505760 | 57056 | 562816 | 0 | 28020.8 | 98.303 | 99.5 | 99.7 | 99.7 | true |
| 32 | 32 | 2 | direct | 699392 | 80544 | 779936 | 0 | 38843.2 | 90.111 | 99.0 | 97.0 | 97.0 | true |
| 32 | 32 | 2 | weir | 598336 | 68160 | 666496 | 0 | 33229.7 | 81.919 | 99.4 | 95.6 | 95.6 | true |
| 32 | 32 | 3 | direct | 643488 | 73728 | 717216 | 0 | 35851.9 | 90.111 | 99.0 | 98.0 | 98.0 | true |
| 32 | 32 | 3 | weir | 599296 | 68480 | 667776 | 0 | 33377.6 | 81.919 | 99.4 | 94.9 | 94.9 | true |
| 32 | 64 | 1 | direct | 706656 | 76736 | 783392 | 0 | 39148.2 | 106.495 | 99.8 | 98.5 | 98.5 | true |
| 32 | 64 | 1 | weir | 590592 | 62496 | 653088 | 0 | 32626.2 | 106.495 | 99.7 | 94.9 | 94.9 | true |
| 32 | 64 | 2 | direct | 723584 | 78400 | 801984 | 0 | 39948.8 | 106.495 | 99.4 | 98.0 | 98.0 | true |
| 32 | 64 | 2 | weir | 622048 | 65984 | 688032 | 0 | 34363.9 | 106.495 | 99.5 | 95.8 | 95.8 | true |
| 32 | 64 | 3 | direct | 697824 | 75904 | 773728 | 0 | 38527.6 | 106.495 | 99.4 | 98.5 | 98.5 | true |
| 32 | 64 | 3 | weir | 641568 | 68416 | 709984 | 0 | 35471.6 | 106.495 | 99.5 | 94.9 | 94.9 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 5368 | 32.000 | 4799/569 | 0.017 | 27.225 | 0 | 32.000 |
| 32 | 8 | 2 | 6631 | 32.000 | 5940/691 | 0.018 | 21.512 | 0 | 32.000 |
| 32 | 8 | 3 | 10777 | 32.000 | 9711/1066 | 0.017 | 12.571 | 0 | 32.000 |
| 32 | 32 | 1 | 17588 | 32.000 | 15805/1783 | 0.116 | 28.831 | 0 | 32.000 |
| 32 | 32 | 2 | 20828 | 32.000 | 18698/2130 | 0.110 | 23.730 | 0 | 32.000 |
| 32 | 32 | 3 | 20868 | 32.000 | 18728/2140 | 0.109 | 23.469 | 0 | 32.000 |
| 32 | 64 | 1 | 20409 | 32.000 | 18456/1953 | 21.791 | 29.133 | 0 | 32.000 |
| 32 | 64 | 2 | 21501 | 32.000 | 19439/2062 | 20.064 | 27.504 | 0 | 32.000 |
| 32 | 64 | 3 | 22187 | 32.000 | 20049/2138 | 19.225 | 26.495 | 0 | 32.000 |

**Batch 32 at demonstrated database CPU saturation:** direct 36941.2 logical ops/s (32 workers), Weir 31539.5 logical ops/s (32 workers), Weir/direct 0.8538, change -14.62%.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
