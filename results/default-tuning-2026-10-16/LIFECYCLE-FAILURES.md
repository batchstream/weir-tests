# Retained incomplete paired experiments

The source review of the original twelve C2 confirmation workflows verified all
5,111 compressed/uncompressed file hashes and lengths, twelve download receipts,
34 case receipts, frozen source identities and observed runtime controls. All
34 owned containers and processes were removed/stopped. Six cases nevertheless
have process exit errors in `cleanup.json` and remain ineligible under the
original cleanup and whole-case rules. No threshold or qualification was relaxed.

| Case | Owned fixture |
| --- | --- |
| `default-partial-q025-r1024` | `weir-tests-2921333718` |
| `default-partial-q025-r2048` | `weir-tests-3103689741` |
| `default-partial-q025-r4096` | `weir-tests-1312594022` |
| `default-crawler-q025-r136` | `weir-tests-1674464681` |
| `default-crawler-q025-r272` | `weir-tests-3001380342` |
| `default-crawler-q025-r544` | `weir-tests-2322707788` |

The raw files are retained under the corresponding `raw/confirm-partial-q025`
and `raw/confirm-crawler-q025` inventories. Each of these six reports contains
only a direct-path first-round stage, warmup clients, no timed clients and
`Verified=false`. Native warmup error examples include `context deadline exceeded`.
The parent leaves that stage before timing and closes client stdin; children
waiting for the timed protocol message consequently record EOF and exit errors.

The C2 runner then attempts the optimized path. `ReplaceServer` normally closes
the previous Weir instance, writes the omitted-batching configuration and starts
the replacement. The new instance fails backend startup qualification before
listening. Thus the fixture manifest still describes the previous successful
legacy configuration while the final routes contain omitted batching. Cleanup
records the already-failed process exit and still reclaims all owned resources.

All six process logs show about five seconds from the previous normal close to
the final startup qualification failure, consistent with the unchanged default
`lifecycle.startup_timeout=5s`. The error text `search product identification
failed` does not distinguish a deadline from a non-200 or otherwise invalid
response. A startup-budget explanation is an inference; the original native
warmup timeout and subsequent qualification failure are directly observed.

The terminal defer in the frozen C2 runner overwrites its earlier `Incomplete`
diagnostic with the later startup/cleanup error. The earlier causes remain in
warmup worker `error_examples` and client exits. This diagnostic limitation must
be described rather than interpreting the final error as the first failure.
The frozen clients, original raw reports and auditor remain unchanged for all
measurements. Any subsequent diagnostic fix must be committed after the final
measurement client source is frozen and identified separately.

These six cases establish neither a qualified paired comparison nor a valid
service-overload upper bound. The prospectively registered independent C3
ladders address the path-coupling limitation without changing these outcomes.

Source traces: C2 `682e725df1a81e29f4d4983d8711df1989c4adad`,
`internal/benchmark/crawler.go` (terminal report defer, path replacement and
warmup early return), `internal/fixture/fixture.go` (replacement and cleanup);
W′ `084572d79b7a53230801297f90ca2363fd864913`, `cmd/weir/main.go` startup
budget and `internal/backend/search/adapter.go` qualification.
