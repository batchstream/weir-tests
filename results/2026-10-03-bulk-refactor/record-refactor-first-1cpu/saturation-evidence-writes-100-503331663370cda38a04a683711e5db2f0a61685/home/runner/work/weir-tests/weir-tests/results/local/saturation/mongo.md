# Database saturation comparison

Backend **mongo**. Started 2026-10-03T12:15:14.699602793Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 0 | 270048 | 270048 | 0 | 13500.0 | 90.111 | 86.6 | 51.4 | 97.5 | true |
| 32 | 8 | 1 | weir | 0 | 178144 | 178144 | 0 | 8870.6 | 131.071 | 70.3 | 5.1 | 97.1 | true |
| 32 | 8 | 2 | direct | 0 | 212736 | 212736 | 0 | 10582.0 | 106.495 | 87.0 | 51.6 | 98.0 | true |
| 32 | 8 | 2 | weir | 0 | 161760 | 161760 | 0 | 8072.3 | 122.879 | 76.5 | 30.5 | 97.0 | true |
| 32 | 8 | 3 | direct | 0 | 198528 | 198528 | 0 | 9921.9 | 114.687 | 83.9 | 46.1 | 98.0 | true |
| 32 | 8 | 3 | weir | 0 | 168576 | 168576 | 0 | 8354.3 | 147.455 | 71.6 | 15.2 | 96.2 | true |
| 32 | 32 | 1 | direct | 0 | 224096 | 224096 | 0 | 11185.4 | 327.679 | 96.7 | 93.5 | 98.6 | true |
| 32 | 32 | 1 | weir | 0 | 183616 | 183616 | 0 | 9171.6 | 360.447 | 86.7 | 41.3 | 98.0 | true |
| 32 | 32 | 2 | direct | 0 | 228704 | 228704 | 0 | 11424.6 | 294.911 | 96.0 | 78.2 | 99.0 | true |
| 32 | 32 | 2 | weir | 0 | 204352 | 204352 | 0 | 10115.9 | 327.679 | 94.1 | 72.8 | 99.5 | true |
| 32 | 32 | 3 | direct | 0 | 226080 | 226080 | 0 | 11281.4 | 294.911 | 84.0 | 46.8 | 98.9 | true |
| 32 | 32 | 3 | weir | 0 | 158720 | 158720 | 0 | 7843.6 | 491.519 | 85.3 | 41.5 | 97.6 | true |
| 32 | 64 | 1 | direct | 0 | 232672 | 232672 | 0 | 11627.7 | 589.823 | 99.0 | 90.9 | 96.4 | true |
| 32 | 64 | 1 | weir | 0 | 188992 | 188992 | 0 | 9391.9 | 524.287 | 96.4 | 78.0 | 99.2 | true |
| 32 | 64 | 2 | direct | 0 | 239712 | 239712 | 0 | 11861.8 | 524.287 | 99.8 | 95.5 | 95.5 | true |
| 32 | 64 | 2 | weir | 0 | 208064 | 208064 | 0 | 10350.7 | 524.287 | 96.2 | 88.7 | 99.4 | true |
| 32 | 64 | 3 | direct | 0 | 206080 | 206080 | 0 | 10257.3 | 524.287 | 95.6 | 85.9 | 96.5 | true |
| 32 | 64 | 3 | weir | 0 | 197952 | 197952 | 0 | 9844.8 | 491.519 | 91.3 | 52.2 | 94.6 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 5567 | 32.000 | 0/5567 | 0.019 | 26.605 | 0 | 32.000 |
| 32 | 8 | 2 | 5055 | 32.000 | 0/5055 | 0.018 | 29.534 | 0 | 32.000 |
| 32 | 8 | 3 | 5268 | 32.000 | 0/5268 | 0.021 | 28.477 | 0 | 32.000 |
| 32 | 32 | 1 | 5738 | 32.000 | 0/5738 | 0.104 | 103.471 | 0 | 32.000 |
| 32 | 32 | 2 | 6386 | 32.000 | 0/6386 | 0.069 | 92.996 | 0 | 32.000 |
| 32 | 32 | 3 | 4960 | 32.000 | 0/4960 | 0.106 | 122.765 | 0 | 32.000 |
| 32 | 64 | 1 | 5906 | 32.000 | 0/5906 | 101.155 | 107.922 | 0 | 32.000 |
| 32 | 64 | 2 | 6502 | 32.000 | 0/6502 | 90.171 | 97.684 | 0 | 32.000 |
| 32 | 64 | 3 | 6186 | 32.000 | 0/6186 | 95.538 | 102.847 | 0 | 32.000 |

**Batch 32: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
