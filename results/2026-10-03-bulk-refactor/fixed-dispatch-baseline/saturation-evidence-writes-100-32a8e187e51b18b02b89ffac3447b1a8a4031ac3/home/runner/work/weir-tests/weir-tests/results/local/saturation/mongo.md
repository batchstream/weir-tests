# Database saturation comparison

Backend **mongo**. Started 2026-10-03T11:14:00.745396304Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 0 | 156736 | 156736 | 0 | 7834.2 | 90.111 | 99.6 | 96.4 | 96.4 | true |
| 32 | 8 | 1 | weir | 0 | 126656 | 126656 | 0 | 6330.8 | 98.303 | 99.9 | 96.4 | 96.4 | true |
| 32 | 8 | 2 | direct | 0 | 113792 | 113792 | 0 | 5687.9 | 106.495 | 99.6 | 97.9 | 97.9 | true |
| 32 | 8 | 2 | weir | 0 | 111232 | 111232 | 0 | 5559.6 | 106.495 | 99.8 | 97.0 | 97.0 | true |
| 32 | 8 | 3 | direct | 0 | 129568 | 129568 | 0 | 6476.0 | 98.303 | 99.5 | 95.9 | 95.9 | true |
| 32 | 8 | 3 | weir | 0 | 113024 | 113024 | 0 | 5596.4 | 98.303 | 99.7 | 95.4 | 95.4 | true |
| 32 | 32 | 1 | direct | 0 | 122688 | 122688 | 0 | 5986.7 | 524.287 | 99.8 | 96.5 | 96.5 | true |
| 32 | 32 | 1 | weir | 0 | 106560 | 106560 | 0 | 5303.2 | 425.983 | 100.1 | 96.6 | 96.6 | true |
| 32 | 32 | 2 | direct | 0 | 129024 | 129024 | 0 | 6323.4 | 425.983 | 100.0 | 97.1 | 97.1 | true |
| 32 | 32 | 2 | weir | 0 | 110240 | 110240 | 0 | 5402.6 | 524.287 | 100.0 | 96.1 | 96.1 | true |
| 32 | 32 | 3 | direct | 0 | 117888 | 117888 | 0 | 5864.8 | 589.823 | 100.0 | 98.0 | 98.0 | true |
| 32 | 32 | 3 | weir | 0 | 106112 | 106112 | 0 | 5254.6 | 425.983 | 99.9 | 97.1 | 97.1 | true |
| 32 | 64 | 1 | direct | 0 | 117408 | 117408 | 0 | 5815.1 | 917.503 | 100.0 | 94.5 | 94.5 | true |
| 32 | 64 | 1 | weir | 0 | 116032 | 116032 | 0 | 5718.8 | 720.895 | 100.0 | 97.1 | 97.1 | true |
| 32 | 64 | 2 | direct | 0 | 125056 | 125056 | 0 | 6218.6 | 786.431 | 100.2 | 95.5 | 95.5 | true |
| 32 | 64 | 2 | weir | 0 | 104672 | 104672 | 0 | 5082.3 | 917.503 | 100.0 | 95.6 | 95.6 | true |
| 32 | 64 | 3 | direct | 0 | 129280 | 129280 | 0 | 6424.9 | 851.967 | 100.0 | 95.3 | 95.3 | true |
| 32 | 64 | 3 | weir | 0 | 104480 | 104480 | 0 | 5145.0 | 917.503 | 99.7 | 96.4 | 96.4 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 3958 | 32.000 | 0/3958 | 0.036 | 37.664 | 0 | 32.000 |
| 32 | 8 | 2 | 3476 | 32.000 | 0/3476 | 0.036 | 43.359 | 0 | 32.000 |
| 32 | 8 | 3 | 3532 | 32.000 | 0/3532 | 0.035 | 43.046 | 0 | 32.000 |
| 32 | 32 | 1 | 3330 | 32.000 | 0/3330 | 91.791 | 96.242 | 0 | 32.000 |
| 32 | 32 | 2 | 3445 | 32.000 | 0/3445 | 89.825 | 94.340 | 0 | 32.000 |
| 32 | 32 | 3 | 3316 | 32.000 | 0/3316 | 92.642 | 97.173 | 0 | 32.000 |
| 32 | 64 | 1 | 3626 | 32.000 | 0/3626 | 261.255 | 89.160 | 0 | 32.000 |
| 32 | 64 | 2 | 3271 | 32.000 | 0/3271 | 291.377 | 100.098 | 0 | 32.000 |
| 32 | 64 | 3 | 3265 | 32.000 | 0/3265 | 291.316 | 99.145 | 0 | 32.000 |

**Batch 32 at demonstrated database CPU saturation:** direct 6666.0 logical ops/s (8 workers), Weir 5828.2 logical ops/s (8 workers), Weir/direct 0.8743, change -12.57%.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
