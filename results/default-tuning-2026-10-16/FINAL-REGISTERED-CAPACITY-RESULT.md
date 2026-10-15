# Final registered capacity attempt: preparation failure

The sole `confirm-pure-extended-q0125` attempt used frozen client
`65f54d7903ae277a739d2b0bb8129feac446f7c8`, Weir
`084572d79b7a53230801297f90ca2363fd864913`, and the exact
[registered plan](../../plans/default-pure-extended-q0125-2026-10-16.json).
The plan declared 1024/2048 workers, four OS clients, three rounds per level,
300 s fixed warmup and 180 s measurement per path: 12 timed stages and
96 minutes of planned warmup plus measurement. **Those stages did not run.**

[Workflow 38815775471](https://github.com/batchstream/weir-tests/actions/runs/38815775471)
failed during fixture startup. The retained execution log reports a GET to
`/_cluster/health?wait_for_status=yellow&timeout=2s` with
`context deadline exceeded (Client.Timeout exceeded while awaiting headers)`.
The version probe had completed, but the subsequent health request failed.
No Weir or client process, warmup or timed stage started. This supplies zero
new throughput observations and cannot establish overload, a capacity upper
bound, stability or a maximum. The healthy-startup counterfactual is unknown.

All 400 archived files and their compressed/uncompressed hashes and lengths
were committed at `e8c439d31f43596f4fc75d1c920f9a14d36b3623` before interpreting
the failure. The [raw inventory](raw/confirm-pure-extended-q0125/files.json),
[source/dispatch preflight](extended-capacity-independent-preflight.json) and
[independent failure review](extended-capacity-independent-failure-review.json)
remain available. All 77 client and 312 server source blobs match their frozen
revisions, the source diff is empty, and binary hashes are unchanged. The
cleanup receipt records the single owned Elasticsearch container removed,
no processes and an empty errors field.

The [registered finite stopping rule](EXTENDED-CONCURRENCY-SUPPLEMENT.md) applies:
there is no replacement run, higher concurrency, changed quota or favorable
retry. Prior failed gates remain failures. No qualified Weir point is available
for a sustained check; no soak is dispatched. The earlier observed same-CPU
throughput gains and comparable-throughput CPU savings remain distinct from
a sustainable maximum.

After preserving this evidence, two test-tool corrections were made without
rerunning performance. `4bcffef` waits for singleton health within the existing
parent and per-request budgets; local HTTP regressions cover transient 503,
red-to-green recovery, strict template acknowledgement and parent cancellation.
`38c64da` preserves an earlier crawler phase/round diagnostic when a later fatal
error occurs. Offline Go tests, fixture/benchmark race checks and vet pass.
These later corrections are not the measured client source, do not modify
archived results, and do not prove that this capacity attempt would pass.

The [final full audit](analysis-final-registered-capacity/analysis.json) retains
591 rows across 25 available workflow archives. The [equivalence receipt](final-analysis-equivalence.json)
confirms all prior rows and decisions are unchanged, and the
[raw-derived final plan](soak-derivation-final-no-qualified-points.json) contains
zero sustained checks.
