# Weir throughput comparison

Backend: **search**. Started: 2026-10-04T02:58:28.700405604Z.

fixed identical operation plan; partitioned IDs; no client bulk; alternating AB/BA; setup, warmup, reset and independent postflight excluded; elapsed includes worker start barrier and all joins; latency includes payload validation; throughput counts only successful logical operations.

Each path executes 256 operations (0 reads, 256 writes), 16 workers, 64 pre-created records and 1024 padding bytes per document. Warmup: 64 operations; paired rounds: 2.

| Round | Order | Path | Success | Errors | Indeterminate | Not attempted | Ops/s | p50 upper ms | p95 upper ms | p99 upper ms | Verified |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | direct / weir | direct | 256 | 0 | 0 | 0 | 200.0 | 90.111 | 163.839 | 180.223 | true |
| 1 | direct / weir | weir | 256 | 0 | 0 | 0 | 260.9 | 73.727 | 106.495 | 106.495 | true |
| 2 | weir / direct | direct | 256 | 0 | 0 | 0 | 425.7 | 14.335 | 147.455 | 147.455 | true |
| 2 | weir / direct | weir | 256 | 0 | 0 | 0 | 382.5 | 20.479 | 98.303 | 98.303 | true |

Pooled throughput across all 2 verified pairs: direct **272.1 ops/s**, Weir **310.2 ops/s**; Weir/direct **1.1401**, Weir throughput change **+14.01%**.

Latency includes every attempted operation and payload validation. Quantiles are approximate inclusive upper bounds from a fixed logarithmic histogram. Counts and elapsed wall time include all worker joins. Logical document byte counts exclude wire framing, metadata and mutation acknowledgements; they are not network bandwidth measurements.

The JSON report contains protocol, backend version, write/retry policy, byte counts, bounded error examples, and supplied fixture provenance. Results characterize the recorded workload and environment; they do not establish a general Weir overhead figure.
