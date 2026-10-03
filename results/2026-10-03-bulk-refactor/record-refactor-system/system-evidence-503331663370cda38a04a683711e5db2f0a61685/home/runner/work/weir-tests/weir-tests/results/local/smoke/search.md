# Weir throughput comparison

Backend: **search**. Started: 2026-10-03T12:15:54.266099763Z.

fixed identical operation plan; partitioned IDs; no client bulk; alternating AB/BA; setup, warmup, reset and independent postflight excluded; elapsed includes worker start barrier and all joins; latency includes payload validation; throughput counts only successful logical operations.

Each path executes 128 operations (117 reads, 11 writes), 8 workers, 64 pre-created records and 1024 padding bytes per document. Warmup: 32 operations; paired rounds: 2.

| Round | Order | Path | Success | Errors | Indeterminate | Not attempted | Ops/s | p50 upper ms | p95 upper ms | p99 upper ms | Verified |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | direct / weir | direct | 128 | 0 | 0 | 0 | 422.4 | 6.143 | 81.919 | 90.111 | true |
| 1 | direct / weir | weir | 128 | 0 | 0 | 0 | 587.3 | 5.631 | 73.727 | 73.727 | true |
| 2 | weir / direct | direct | 128 | 0 | 0 | 0 | 1071.7 | 2.815 | 65.535 | 73.727 | true |
| 2 | weir / direct | weir | 128 | 0 | 0 | 0 | 473.5 | 5.631 | 73.727 | 73.727 | true |

Pooled throughput across all 2 verified pairs: direct **606.0 ops/s**, Weir **524.3 ops/s**; Weir/direct **0.8652**, Weir throughput change **-13.48%**.

Latency includes every attempted operation and payload validation. Quantiles are approximate inclusive upper bounds from a fixed logarithmic histogram. Counts and elapsed wall time include all worker joins. Logical document byte counts exclude wire framing, metadata and mutation acknowledgements; they are not network bandwidth measurements.

The JSON report contains protocol, backend version, write/retry policy, byte counts, bounded error examples, and supplied fixture provenance. Results characterize the recorded workload and environment; they do not establish a general Weir overhead figure.
