# Benchmark output

Use a fresh directory such as `results/local/read` for each run. Generated output
is ignored by Git. CI uploads reports and fixture receipts as workflow artifacts.
The minimal `go.mod` in this directory gives evidence data a module boundary, so
historical generated Go files are excluded from root unit tests and dependency
checks. It has no code or dependencies; retain existing measurements in place.

A reproducible comparison needs the immutable test/server/SDK/protocol versions,
exact command, client and server binary identities, database resource limits,
raw samples, per-round results, independent persistence checks and successful
owned-resource cleanup receipts. Compare runs with the same workload and limits.

Observed throughput at a fixed concurrency is distinct from a qualified
maximum-capacity comparison. Both paths must meet the database-saturation and
next-concurrency plateau criteria described in the repository README.

Local refactor runs must include the checksummed binary receipt and retained
`*.source` snapshot produced by `scripts/build_local_server.py`. The tree
manifest covers tracked and untracked Go inputs. A source HEAD alone does not
identify an unpublished build. Preserve the exact command and source snapshot
for the benchmark client as well when its checkout is dirty.
