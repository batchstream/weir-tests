# Weir throughput comparison

Backend: **search**. Started: 2026-10-03T03:06:22.814657Z.

fixed identical operation plan; partitioned IDs; no client bulk; alternating AB/BA; setup, warmup, reset and independent postflight excluded; elapsed includes worker start barrier and all joins; latency includes payload validation; throughput counts only successful logical operations.

Each path executes 10000 operations (10000 reads, 0 writes), 8 workers, 1024 pre-created records and 1024 padding bytes per document. Warmup: 1000 operations; paired rounds: 3.

| Round | Order | Path | Success | Errors | Indeterminate | Not attempted | Ops/s | p50 upper ms | p95 upper ms | p99 upper ms | Verified |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | direct / weir | direct | 10000 | 0 | 0 | 0 | 6295.9 | 0.831 | 1.407 | 24.575 | true |
| 1 | direct / weir | weir | 10000 | 0 | 0 | 0 | 3272.9 | 2.303 | 3.583 | 10.239 | true |
| 2 | weir / direct | direct | 10000 | 0 | 0 | 0 | 11644.9 | 0.639 | 1.023 | 1.535 | true |
| 2 | weir / direct | weir | 10000 | 0 | 0 | 0 | 3928.3 | 2.047 | 2.815 | 4.607 | true |
| 3 | direct / weir | direct | 10000 | 0 | 0 | 0 | 11315.4 | 0.703 | 1.151 | 1.791 | true |
| 3 | direct / weir | weir | 10000 | 0 | 0 | 0 | 3960.2 | 2.047 | 2.815 | 4.607 | true |

Pooled throughput across all 3 verified pairs: direct **9006.7 ops/s**, Weir **3691.8 ops/s**; Weir/direct **0.4099**, Weir throughput change **-59.01%**.

Latency includes every attempted operation and payload validation. Quantiles are approximate inclusive upper bounds from a fixed logarithmic histogram. Counts and elapsed wall time include all worker joins. Logical document byte counts exclude wire framing, metadata and mutation acknowledgements; they are not network bandwidth measurements.

The JSON report contains protocol, backend version, write/retry policy, byte counts, bounded error examples, and supplied fixture provenance. Results characterize the recorded workload and environment; they do not establish a general Weir overhead figure.
