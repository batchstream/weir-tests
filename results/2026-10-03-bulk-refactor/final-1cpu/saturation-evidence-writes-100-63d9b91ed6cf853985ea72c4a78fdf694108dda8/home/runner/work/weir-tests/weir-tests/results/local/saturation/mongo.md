# Database saturation comparison

Backend **mongo**. Started 2026-10-03T12:37:46.515376896Z.

duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 32 | 8 | 1 | direct | 0 | 155552 | 155552 | 0 | 7774.7 | 90.111 | 99.6 | 97.4 | 97.4 | true |
| 32 | 8 | 1 | weir | 0 | 134400 | 134400 | 0 | 6689.1 | 98.303 | 99.8 | 95.0 | 95.0 | true |
| 32 | 8 | 2 | direct | 0 | 117216 | 117216 | 0 | 5833.8 | 106.495 | 99.6 | 98.0 | 98.0 | true |
| 32 | 8 | 2 | weir | 0 | 114656 | 114656 | 0 | 5708.9 | 98.303 | 99.7 | 95.0 | 95.0 | true |
| 32 | 8 | 3 | direct | 0 | 119008 | 119008 | 0 | 5948.8 | 98.303 | 99.8 | 97.1 | 97.1 | true |
| 32 | 8 | 3 | weir | 0 | 107488 | 107488 | 0 | 5372.3 | 106.495 | 99.7 | 97.4 | 97.4 | true |
| 32 | 32 | 1 | direct | 0 | 122656 | 122656 | 0 | 6102.2 | 491.519 | 99.9 | 98.9 | 98.9 | true |
| 32 | 32 | 1 | weir | 0 | 108928 | 108928 | 0 | 5422.0 | 458.751 | 99.9 | 98.9 | 98.9 | true |
| 32 | 32 | 2 | direct | 0 | 127584 | 127584 | 0 | 6348.9 | 425.983 | 99.7 | 97.9 | 97.9 | true |
| 32 | 32 | 2 | weir | 0 | 107680 | 107680 | 0 | 5337.7 | 524.287 | 99.6 | 98.0 | 98.0 | true |
| 32 | 32 | 3 | direct | 0 | 127488 | 127488 | 0 | 6340.1 | 327.679 | 100.1 | 98.5 | 98.5 | true |
| 32 | 32 | 3 | weir | 0 | 110528 | 110528 | 0 | 5415.2 | 524.287 | 100.1 | 97.5 | 97.5 | true |
| 32 | 64 | 1 | direct | 0 | 125824 | 125824 | 0 | 6255.2 | 851.967 | 100.3 | 96.5 | 96.5 | true |
| 32 | 64 | 1 | weir | 0 | 114976 | 114976 | 0 | 5688.4 | 655.359 | 99.7 | 99.3 | 99.3 | true |
| 32 | 64 | 2 | direct | 0 | 119904 | 119904 | 0 | 5933.6 | 917.503 | 100.0 | 98.5 | 98.5 | true |
| 32 | 64 | 2 | weir | 0 | 111488 | 111488 | 0 | 5492.3 | 720.895 | 99.8 | 98.4 | 98.4 | true |
| 32 | 64 | 3 | direct | 0 | 128160 | 128160 | 0 | 6346.6 | 851.967 | 100.0 | 94.6 | 94.6 | true |
| 32 | 64 | 3 | weir | 0 | 110144 | 110144 | 0 | 5277.5 | 786.431 | 99.7 | 95.2 | 95.2 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 32 | 8 | 1 | 4200 | 32.000 | 0/4200 | 0.026 | 35.893 | 0 | 32.000 |
| 32 | 8 | 2 | 3583 | 32.000 | 0/3583 | 0.026 | 42.549 | 0 | 32.000 |
| 32 | 8 | 3 | 3359 | 32.000 | 0/3359 | 0.028 | 45.272 | 0 | 32.000 |
| 32 | 32 | 1 | 3404 | 32.000 | 0/3404 | 0.052 | 183.291 | 0 | 32.000 |
| 32 | 32 | 2 | 3365 | 32.000 | 0/3365 | 0.042 | 186.446 | 0 | 32.000 |
| 32 | 32 | 3 | 3454 | 32.000 | 0/3454 | 0.049 | 184.530 | 0 | 32.000 |
| 32 | 64 | 1 | 3593 | 32.000 | 0/3593 | 174.188 | 179.431 | 0 | 32.000 |
| 32 | 64 | 2 | 3484 | 32.000 | 0/3484 | 179.845 | 185.382 | 0 | 32.000 |
| 32 | 64 | 3 | 3442 | 32.000 | 0/3442 | 182.572 | 188.715 | 0 | 32.000 |

**Batch 32 at demonstrated database CPU saturation:** direct 6518.1 logical ops/s (8 workers), Weir 5924.2 logical ops/s (8 workers), Weir/direct 0.9089, change -9.11%.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
