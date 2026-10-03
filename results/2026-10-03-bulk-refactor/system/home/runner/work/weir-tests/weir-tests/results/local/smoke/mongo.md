# Weir throughput comparison

Backend: **mongo**. Started: 2026-10-03T10:03:23.962471617Z.

fixed identical operation plan; partitioned IDs; no client bulk; alternating AB/BA; setup, warmup, reset and independent postflight excluded; elapsed includes worker start barrier and all joins; latency includes payload validation; throughput counts only successful logical operations.

Each path executes 128 operations (117 reads, 11 writes), 8 workers, 64 pre-created records and 1024 padding bytes per document. Warmup: 32 operations; paired rounds: 2.

| Round | Order | Path | Success | Errors | Indeterminate | Not attempted | Ops/s | p50 upper ms | p95 upper ms | p99 upper ms | Verified |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | direct / weir | direct | 128 | 0 | 0 | 0 | 6821.2 | 0.959 | 2.047 | 2.303 | true |
| 1 | direct / weir | weir | 128 | 0 | 0 | 0 | 2236.9 | 3.071 | 7.167 | 8.191 | true |
| 2 | weir / direct | direct | 128 | 0 | 0 | 0 | 4718.3 | 1.279 | 4.607 | 5.119 | true |
| 2 | weir / direct | weir | 128 | 0 | 0 | 0 | 2608.4 | 2.815 | 6.143 | 6.655 | true |

Pooled throughput across all 2 verified pairs: direct **5578.2 ops/s**, Weir **2408.4 ops/s**; Weir/direct **0.4318**, Weir throughput change **-56.82%**.

Latency includes every attempted operation and payload validation. Quantiles are approximate inclusive upper bounds from a fixed logarithmic histogram. Counts and elapsed wall time include all worker joins. Logical document byte counts exclude wire framing, metadata and mutation acknowledgements; they are not network bandwidth measurements.

The JSON report contains protocol, backend version, write/retry policy, byte counts, bounded error examples, and supplied fixture provenance. Results characterize the recorded workload and environment; they do not establish a general Weir overhead figure.
