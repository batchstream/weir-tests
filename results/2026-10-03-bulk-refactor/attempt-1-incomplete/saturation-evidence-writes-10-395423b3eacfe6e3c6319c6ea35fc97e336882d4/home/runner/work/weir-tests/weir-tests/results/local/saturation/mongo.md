# Database saturation comparison

Backend **mongo**. Started 2026-10-03T10:02:14.523711031Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 1032992 | 115328 | 1148320 | 0 | 57412.3 | 11.263 | 85.0 | 36.4 | 98.5 | true |
| 32 | 8 | 1 | weir | 746336 | 81856 | 828192 | 0 | 41404.3 | 11.263 | 65.9 | 0.0 | 97.8 | true |
| 32 | 8 | 2 | direct | 1004000 | 111584 | 1115584 | 0 | 55651.0 | 16.383 | 94.2 | 77.7 | 98.7 | true |
| 32 | 8 | 2 | weir | 690688 | 76032 | 766720 | 0 | 38304.3 | 12.287 | 67.0 | 0.0 | 97.7 | true |
| 32 | 8 | 3 | direct | 866656 | 97152 | 963808 | 0 | 48089.4 | 20.479 | 88.4 | 57.2 | 98.4 | true |
| 32 | 8 | 3 | weir | 710816 | 78784 | 789600 | 0 | 39277.0 | 12.287 | 75.9 | 10.2 | 98.1 | true |
| 32 | 32 | 1 | direct | 966752 | 108832 | 1075584 | 0 | 53739.1 | 81.919 | 99.3 | 95.0 | 95.0 | true |
| 32 | 32 | 1 | weir | 628384 | 72160 | 700544 | 0 | 34987.2 | 98.303 | 64.4 | 10.3 | 98.0 | true |
| 32 | 32 | 2 | direct | 955776 | 107904 | 1063680 | 0 | 53131.0 | 81.919 | 99.9 | 95.0 | 95.0 | true |
| 32 | 32 | 2 | weir | 342336 | 38976 | 381312 | 0 | 19033.0 | 229.375 | 34.0 | 0.0 | 97.2 | true |
| 32 | 32 | 3 | direct | 946688 | 107040 | 1053728 | 0 | 52218.7 | 81.919 | 99.6 | 99.9 | 99.9 | true |
| 32 | 32 | 3 | weir | 374400 | 41760 | 416160 | 0 | 20643.3 | 180.223 | 33.9 | 0.0 | 96.5 | true |
| 32 | 64 | 1 | direct | 985696 | 109632 | 1095328 | 0 | 53535.2 | 106.495 | 100.1 | 95.9 | 95.9 | true |
| 32 | 64 | 1 | weir | 267168 | 28800 | 295936 | 32 | 29496.3 | 294.911 | 0.0 | 0.0 | 0.0 | false |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backpressure |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 25881 | 32.000 | 23323/2558 | 0.035 | 3.424 | 0 | 0 |
| 32 | 8 | 2 | 23960 | 32.000 | 21584/2376 | 0.033 | 3.934 | 0 | 0 |
| 32 | 8 | 3 | 24675 | 32.000 | 22213/2462 | 0.038 | 3.630 | 0 | 0 |
| 32 | 32 | 1 | 21892 | 32.000 | 19637/2255 | 16.072 | 4.432 | 0 | 28 |
| 32 | 32 | 2 | 11916 | 32.000 | 10698/1218 | 48.843 | 2.348 | 0 | 9 |
| 32 | 32 | 3 | 13005 | 32.000 | 11700/1305 | 45.846 | 1.808 | 0 | 10 |
| 32 | 64 | 1 | unavailable | unavailable | unavailable | unavailable | unavailable | unavailable | unavailable |

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.

Incomplete: weir warmup failed: [rpc error: code = ResourceExhausted desc = session limit rpc error: code = ResourceExhausted desc = session limit rpc error: code = ResourceExhausted desc = session limit]
