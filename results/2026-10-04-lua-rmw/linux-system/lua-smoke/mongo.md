# Weir throughput comparison

Backend: **mongo**. Started: 2026-10-04T02:58:26.911656865Z.

fixed identical operation plan; partitioned IDs; no client bulk; alternating AB/BA; setup, warmup, reset and independent postflight excluded; elapsed includes worker start barrier and all joins; latency includes payload validation; throughput counts only successful logical operations.

Each path executes 256 operations (0 reads, 256 writes), 16 workers, 64 pre-created records and 1024 padding bytes per document. Warmup: 64 operations; paired rounds: 2.

| Round | Order | Path | Success | Errors | Indeterminate | Not attempted | Ops/s | p50 upper ms | p95 upper ms | p99 upper ms | Verified |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | direct / weir | direct | 256 | 0 | 0 | 0 | 2750.1 | 4.607 | 12.287 | 15.359 | true |
| 1 | direct / weir | weir | 256 | 0 | 0 | 0 | 1929.4 | 8.191 | 13.311 | 18.431 | true |
| 2 | weir / direct | direct | 256 | 0 | 0 | 0 | 2203.7 | 4.607 | 36.863 | 36.863 | true |
| 2 | weir / direct | weir | 256 | 0 | 0 | 0 | 1811.4 | 8.191 | 16.383 | 22.527 | true |

Pooled throughput across all 2 verified pairs: direct **2446.7 ops/s**, Weir **1868.5 ops/s**; Weir/direct **0.7637**, Weir throughput change **-23.63%**.

Latency includes every attempted operation and payload validation. Quantiles are approximate inclusive upper bounds from a fixed logarithmic histogram. Counts and elapsed wall time include all worker joins. Logical document byte counts exclude wire framing, metadata and mutation acknowledgements; they are not network bandwidth measurements.

The JSON report contains protocol, backend version, write/retry policy, byte counts, bounded error examples, and supplied fixture provenance. Results characterize the recorded workload and environment; they do not establish a general Weir overhead figure.
