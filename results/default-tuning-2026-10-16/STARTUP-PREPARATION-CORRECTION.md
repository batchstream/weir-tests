# Prospective startup-preparation correction cohort

The one-eighth-CPU workflow
`https://github.com/batchstream/weir-tests/actions/runs/38797232567` failed before
any warmup or timed measurement. Its Elasticsearch JVM was still initializing
when fixture readiness hit its nested 180s context deadline. No Weir or client
process had started; the owned container was removed with an empty cleanup-error
receipt. Its complete retained evidence at commit
`66e31c8ee46bd90759a7dbff1e75f202bee0acf5` remains mandatory, including its failed
receipt and absence of measured rows. This is a preparation failure, not a
database-capacity observation.

`fixture.Start` already gives the entire startup an eight-minute parent context.
The correction removes only `readySearch`'s additional 180s timeout and uses that
parent context for its existing version probe, retry pause, health request and
template publication. It retains the existing per-request bounds, pinned version,
singleton health and acknowledged-template checks. Startup, cleanup and every
measurement remain bounded. There is no new timeout setting, adaptive extension,
startup under a different CPU quota, or repeated wait until a favorable result.

The preparation-only Go source, Cprep, is
`2220c18eae4cbe5d079753155638f44cc7dc649b`. Compared with original measurement
source C0 `dc7c798a83d2d1700c3ff5b13f678f8af1e55d37`, only these Go inputs differ:

- `internal/fixture/backends.go`: the mechanical removal of `readySearch`'s
  `context.WithTimeout(ctx, 180*time.Second)` and its `defer cancel()`, and use of
  `ctx` in place of `readyCtx` within that function. Everything outside that
  function, including Mongo startup, must remain identical.
- `internal/fixture/backends_deadline_test.go`: one new regression-test file
  proving that Search startup inherits its parent's budget and cancellation.

Every other Go file, `go.mod`, `go.sum`, and all embedded Lua/Painless bytes must
remain identical to C0. In particular, measurement execution, operation timeout,
phase barriers, warmup, sampling, completion buckets, postflight and audits do
not change. The four embedded resources are `internal/benchmark/crawlerdata/`
`common.lua`, `product.lua`, `offer.lua` and `native.painless`.

Register exactly one new case-set `confirm-pure-q0125-startup-fix`, with one case
`default-pure-q0125-startup-fix`. Keep the complete original one-eighth-CPU design:
Search pure full-document writes, observed quota 0.125 CPU throughout startup
and measurement, workers 256/512/1024, four OS clients, three alternating paired
rounds, 60s warmup and 60s timed per path, 8192 seeded records, 1024 payload bytes,
100% writes, no Lua, and fragmented native single-record calls. Weir uses actual
omitted defaults matching 128 operations / 4194304 bytes / 20ms / 16 in flight.
Server W' remains `084572d79b7a53230801297f90ca2363fd864913`; all Search settings
remain fixed. The inventory is still 18 timed stages / 36 minutes of declared
warmup plus timed work on one runner, excluding preparation and postflight.

The new source and plan must be frozen and independently reviewed before this
single new workflow is dispatched. The plan must identify this document's fixed
commit and SHA256 and Cprep's full source commit. Its clean build and executable
hashes are recorded as new identities; they are not claimed to equal C0's binary.
The source audit continues to require every earlier client Go tree to equal C0.
Only this explicitly named new pure confirmation and the conditional pure soak
below may use a client Go tree equal to Cprep, with exact registered
preparation/test file hashes and unchanged remaining Go/module/embedded inputs.
Unknown source changes or missing registration fail closed. This does not permit
a generic fixture-source exception or expand to another workload.

The original C0 auditor remains unchanged: binary SHA256
`77c39f5d863668117c56baf96babdedfee0e172a300e35c073f3d13e30647bdc`,
unmodified source archive SHA256
`cfde0883cb2c26ca4f806c88be271bd7961525b957f8e7fbde7464ef146eeff2`.
Its schema, source receipt, original auditing behavior and all numerical gates
remain mandatory. The selected controls, `POLICY` dictionary, throughput/read
guards, CPU pressure and coverage, host/client headroom, p99 <=5s, stability,
three-round and adjacent-concurrency requirements are unchanged. The derived
soak calculation may admit this exact registered pure parent alongside the
original pure parents; it keeps the existing workers/quota/label/case-ID order,
at most one pure check and at most three checks overall.

Prospectively register one conditional source exception for the already specified
paired pure soak. Only if the unchanged derivation selects a qualified point from
`confirm-pure-q0125-startup-fix` may its `soak-pure` plan propagate this same fixed
document/source/hash registration and Cprep identity. Its metadata must bind the
actual selected confirmation parent: label, case-set, case ID, client commit,
committed plan path and SHA256, and selected worker level. These references must
match the retained confirmation manifest and its frozen plan; a different,
missing or unqualified parent must not grant the source exception. Selecting a
pure parent from an earlier C0 cohort does not propagate the exception.

The admitted derived shape is exactly `phase=soak`, case-set and case ID
`soak-pure`, Search pure writes, observed quota 0.125 CPU, four OS clients, 8192
records, 1024 payload bytes, 100% writes, no Lua, omitted selected defaults, one
round, 60s warmup and 300s timed, with one selected concurrency of 256 or 512.
It retains the native single-record and Weir pair. The last ladder point, 1024,
cannot qualify without a following declared point and is not admitted here.
This condition permits at most two additional timed stages / twelve minutes of
declared warmup plus timed work, and only after qualification and a separately
frozen derived plan. It adds no extra pure soak, favorable retry, quota, workload
or source change. All original five-minute and source/binary gates still apply.

The failed original workflow and every other negative outcome remain visible;
no numerical result is substituted, discarded or spliced across workflows.
There is one prospectively registered correction attempt, with no favorable
performance retry or further automatic startup-budget extension. It may still
fail preparation or any measurement gate. Such a failure remains evidence, and
no scoped maximum may be claimed from it. A passing adjacent pair and later
sustained check support only the declared workload, quota, ladder and observed
host, not a universal or production maximum.
