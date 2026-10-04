# Weir throughput comparison

Backend: **search**. Started: 2026-10-04T00:03:38.070877308Z.

fixed identical operation plan; partitioned IDs; no client bulk; alternating AB/BA; setup, warmup, reset and independent postflight excluded; elapsed includes worker start barrier and all joins; latency includes payload validation; throughput counts only successful logical operations.

Each path executes 128 operations (117 reads, 11 writes), 8 workers, 64 pre-created records and 1024 padding bytes per document. Warmup: 32 operations; paired rounds: 2.

| Round | Order | Path | Success | Errors | Indeterminate | Not attempted | Ops/s | p50 upper ms | p95 upper ms | p99 upper ms | Verified |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | direct / weir | direct | 128 | 0 | 0 | 0 | 574.4 | 4.607 | 81.919 | 81.919 | true |
| 1 | direct / weir | weir | 128 | 0 | 0 | 0 | 465.3 | 5.119 | 73.727 | 73.727 | true |
| 2 | weir / direct | direct | 128 | 0 | 0 | 0 | 638.3 | 3.839 | 73.727 | 81.919 | true |
| 2 | weir / direct | weir | 128 | 0 | 0 | 0 | 460.5 | 5.119 | 73.727 | 73.727 | true |

Pooled throughput across all 2 verified pairs: direct **604.7 ops/s**, Weir **462.9 ops/s**; Weir/direct **0.7655**, Weir throughput change **-23.45%**.

Latency includes every attempted operation and payload validation. Quantiles are approximate inclusive upper bounds from a fixed logarithmic histogram. Counts and elapsed wall time include all worker joins. Logical document byte counts exclude wire framing, metadata and mutation acknowledgements; they are not network bandwidth measurements.

The JSON report contains protocol, backend version, write/retry policy, byte counts, bounded error examples, and supplied fixture provenance. Results characterize the recorded workload and environment; they do not establish a general Weir overhead figure.
