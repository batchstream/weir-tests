# Weir throughput comparison

Backend: **mongo**. Started: 2026-10-04T02:57:50.038260509Z.

fixed identical operation plan; partitioned IDs; no client bulk; alternating AB/BA; setup, warmup, reset and independent postflight excluded; elapsed includes worker start barrier and all joins; latency includes payload validation; throughput counts only successful logical operations.

Each path executes 128 operations (117 reads, 11 writes), 8 workers, 64 pre-created records and 1024 padding bytes per document. Warmup: 32 operations; paired rounds: 2.

| Round | Order | Path | Success | Errors | Indeterminate | Not attempted | Ops/s | p50 upper ms | p95 upper ms | p99 upper ms | Verified |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | direct / weir | direct | 128 | 0 | 0 | 0 | 11715.4 | 0.575 | 1.407 | 1.535 | true |
| 1 | direct / weir | weir | 128 | 0 | 0 | 0 | 999.8 | 1.151 | 9.215 | 90.111 | true |
| 2 | weir / direct | direct | 128 | 0 | 0 | 0 | 8849.5 | 0.703 | 1.919 | 2.303 | true |
| 2 | weir / direct | weir | 128 | 0 | 0 | 0 | 6036.1 | 1.151 | 2.559 | 3.327 | true |

Pooled throughput across all 2 verified pairs: direct **10082.7 ops/s**, Weir **1715.5 ops/s**; Weir/direct **0.1701**, Weir throughput change **-82.99%**.

Latency includes every attempted operation and payload validation. Quantiles are approximate inclusive upper bounds from a fixed logarithmic histogram. Counts and elapsed wall time include all worker joins. Logical document byte counts exclude wire framing, metadata and mutation acknowledgements; they are not network bandwidth measurements.

The JSON report contains protocol, backend version, write/retry policy, byte counts, bounded error examples, and supplied fixture provenance. Results characterize the recorded workload and environment; they do not establish a general Weir overhead figure.
