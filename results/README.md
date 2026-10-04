# Benchmark output

Use a fresh directory such as `results/local/read` for each run. Generated output
is ignored by Git. CI uploads reports and fixture receipts as workflow artifacts.

A reproducible comparison needs the immutable test/server/SDK/protocol versions,
exact command, client and server binary identities, database resource limits,
raw samples, per-round results, independent persistence checks and successful
owned-resource cleanup receipts. Compare runs with the same workload and limits.

Observed throughput at a fixed concurrency is distinct from a qualified
maximum-capacity comparison. Both paths must meet the database-saturation and
next-concurrency plateau criteria described in the repository README.
