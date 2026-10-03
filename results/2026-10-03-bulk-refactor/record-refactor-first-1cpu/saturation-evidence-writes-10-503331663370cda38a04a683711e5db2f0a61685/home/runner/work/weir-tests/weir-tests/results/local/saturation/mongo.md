# Database saturation comparison

Backend **mongo**. Started 2026-10-03T12:15:06.258024768Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 1346048 | 151072 | 1497120 | 0 | 74831.0 | 6.143 | 99.6 | 98.0 | 98.0 | true |
| 32 | 8 | 1 | weir | 1087264 | 121344 | 1208608 | 0 | 60425.3 | 8.191 | 95.4 | 92.5 | 97.7 | true |
| 32 | 8 | 2 | direct | 1056160 | 117312 | 1173472 | 0 | 58666.9 | 7.679 | 99.8 | 98.6 | 98.6 | true |
| 32 | 8 | 2 | weir | 1030976 | 114336 | 1145312 | 0 | 57249.3 | 8.191 | 95.3 | 87.3 | 97.7 | true |
| 32 | 8 | 3 | direct | 1134176 | 126624 | 1260800 | 0 | 63031.7 | 7.679 | 99.7 | 98.5 | 98.5 | true |
| 32 | 8 | 3 | weir | 949728 | 105952 | 1055680 | 0 | 52719.9 | 9.215 | 97.0 | 97.4 | 97.4 | true |
| 32 | 32 | 1 | direct | 1100256 | 123104 | 1223360 | 0 | 60991.8 | 73.727 | 100.0 | 95.2 | 95.2 | true |
| 32 | 32 | 1 | weir | 1017696 | 114464 | 1132160 | 0 | 56584.4 | 57.343 | 100.0 | 98.7 | 98.7 | true |
| 32 | 32 | 2 | direct | 1104384 | 124032 | 1228416 | 0 | 61402.8 | 73.727 | 99.9 | 95.0 | 95.0 | true |
| 32 | 32 | 2 | weir | 1075360 | 120704 | 1196064 | 0 | 59769.5 | 45.055 | 99.8 | 98.5 | 98.5 | true |
| 32 | 32 | 3 | direct | 1196768 | 134912 | 1331680 | 0 | 66560.5 | 73.727 | 99.9 | 95.0 | 95.0 | true |
| 32 | 32 | 3 | weir | 995936 | 112288 | 1108224 | 0 | 54697.4 | 49.151 | 100.1 | 97.2 | 97.2 | true |
| 32 | 64 | 1 | direct | 1057760 | 118112 | 1175872 | 0 | 58574.4 | 98.303 | 99.7 | 97.5 | 97.5 | true |
| 32 | 64 | 1 | weir | 1058624 | 116576 | 1175200 | 0 | 58713.1 | 90.111 | 100.0 | 99.6 | 99.6 | true |
| 32 | 64 | 2 | direct | 1132448 | 126208 | 1258656 | 0 | 62884.4 | 98.303 | 99.8 | 98.8 | 98.8 | true |
| 32 | 64 | 2 | weir | 999424 | 110240 | 1109664 | 0 | 55339.5 | 90.111 | 100.0 | 98.8 | 98.8 | true |
| 32 | 64 | 3 | direct | 1105696 | 124096 | 1229792 | 0 | 61439.1 | 98.303 | 99.9 | 97.5 | 97.5 | true |
| 32 | 64 | 3 | weir | 978112 | 108224 | 1086336 | 0 | 53990.0 | 90.111 | 100.0 | 98.3 | 98.3 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 37769 | 32.000 | 33977/3792 | 0.026 | 2.110 | 0 | 32.000 |
| 32 | 8 | 2 | 35791 | 32.000 | 32218/3573 | 0.026 | 2.310 | 0 | 32.000 |
| 32 | 8 | 3 | 32990 | 32.000 | 29679/3311 | 0.025 | 2.722 | 0 | 32.000 |
| 32 | 32 | 1 | 35380 | 32.000 | 31803/3577 | 0.197 | 10.012 | 0 | 32.000 |
| 32 | 32 | 2 | 37377 | 32.000 | 33605/3772 | 0.199 | 9.106 | 0 | 32.000 |
| 32 | 32 | 3 | 34632 | 32.000 | 31123/3509 | 0.214 | 10.641 | 0 | 32.000 |
| 32 | 64 | 1 | 36725 | 32.000 | 33082/3643 | 8.156 | 13.789 | 0 | 32.000 |
| 32 | 64 | 2 | 34677 | 32.000 | 31232/3445 | 9.195 | 14.916 | 0 | 32.000 |
| 32 | 64 | 3 | 33948 | 32.000 | 30566/3382 | 9.440 | 15.209 | 0 | 32.000 |

**Batch 32 at demonstrated database CPU saturation:** direct 65510.5 logical ops/s (8 workers), Weir 57007.5 logical ops/s (32 workers), Weir/direct 0.8702, change -12.98%.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
