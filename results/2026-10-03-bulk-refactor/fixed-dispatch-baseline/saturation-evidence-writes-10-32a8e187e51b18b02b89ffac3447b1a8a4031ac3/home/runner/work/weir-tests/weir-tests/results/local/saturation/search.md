# Database saturation comparison

Backend **search**. Started 2026-10-03T11:23:06.350762834Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 174720 | 20992 | 195712 | 0 | 9779.6 | 81.919 | 99.8 | 98.4 | 98.4 | true |
| 32 | 8 | 1 | weir | 212064 | 24256 | 236320 | 0 | 11812.9 | 61.439 | 99.9 | 95.3 | 95.3 | true |
| 32 | 8 | 2 | direct | 549440 | 59168 | 608608 | 0 | 30425.3 | 45.055 | 99.6 | 99.9 | 99.9 | true |
| 32 | 8 | 2 | weir | 374816 | 41984 | 416800 | 0 | 20831.8 | 49.151 | 94.6 | 73.7 | 99.8 | true |
| 32 | 8 | 3 | direct | 624864 | 68032 | 692896 | 0 | 34638.1 | 40.959 | 99.6 | 99.8 | 99.8 | true |
| 32 | 8 | 3 | weir | 481344 | 51488 | 532832 | 0 | 26634.1 | 16.383 | 87.4 | 25.9 | 98.6 | true |
| 32 | 32 | 1 | direct | 749440 | 86240 | 835680 | 0 | 41766.1 | 65.535 | 99.8 | 95.4 | 95.4 | true |
| 32 | 32 | 1 | weir | 567488 | 64000 | 631488 | 0 | 31533.2 | 45.055 | 86.6 | 15.6 | 98.9 | true |
| 32 | 32 | 2 | direct | 759264 | 87360 | 846624 | 0 | 42308.3 | 65.535 | 99.8 | 96.3 | 96.3 | true |
| 32 | 32 | 2 | weir | 563552 | 63392 | 626944 | 0 | 31307.9 | 45.055 | 87.3 | 20.8 | 98.9 | true |
| 32 | 32 | 3 | direct | 731808 | 84288 | 816096 | 0 | 40786.1 | 73.727 | 99.6 | 96.5 | 96.5 | true |
| 32 | 32 | 3 | weir | 582208 | 66048 | 648256 | 0 | 32374.0 | 40.959 | 86.1 | 10.4 | 99.1 | true |
| 32 | 64 | 1 | direct | 790048 | 85824 | 875872 | 0 | 43658.5 | 98.303 | 99.8 | 97.1 | 97.1 | true |
| 32 | 64 | 1 | weir | 585408 | 61984 | 647392 | 0 | 32281.5 | 90.111 | 87.6 | 20.9 | 99.2 | true |
| 32 | 64 | 2 | direct | 762592 | 82208 | 844800 | 0 | 42209.0 | 98.303 | 99.7 | 97.3 | 97.3 | true |
| 32 | 64 | 2 | weir | 612448 | 64928 | 677376 | 0 | 33787.8 | 73.727 | 86.4 | 10.4 | 98.7 | true |
| 32 | 64 | 3 | direct | 770624 | 83488 | 854112 | 0 | 42677.9 | 98.303 | 100.0 | 96.6 | 96.6 | true |
| 32 | 64 | 3 | weir | 602880 | 63936 | 666816 | 0 | 33259.3 | 81.919 | 86.3 | 5.2 | 98.8 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 7385 | 32.000 | 6627/758 | 6.898 | 10.433 | 0 | 32.000 |
| 32 | 8 | 2 | 13025 | 32.000 | 11713/1312 | 2.510 | 5.604 | 0 | 32.000 |
| 32 | 8 | 3 | 16651 | 32.000 | 15042/1609 | 1.355 | 4.280 | 0 | 32.000 |
| 32 | 32 | 1 | 19734 | 32.000 | 17734/2000 | 23.592 | 3.954 | 0 | 32.000 |
| 32 | 32 | 2 | 19592 | 32.000 | 17611/1981 | 23.956 | 3.991 | 0 | 32.000 |
| 32 | 32 | 3 | 20258 | 32.000 | 18194/2064 | 23.042 | 3.856 | 0 | 32.000 |
| 32 | 64 | 1 | 20231 | 32.000 | 18294/1937 | 54.582 | 3.857 | 0 | 32.000 |
| 32 | 64 | 2 | 21168 | 32.000 | 19139/2029 | 51.957 | 3.686 | 0 | 32.000 |
| 32 | 64 | 3 | 20838 | 32.000 | 18840/1998 | 52.916 | 3.746 | 0 | 32.000 |

**Batch 32: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
