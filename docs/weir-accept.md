# Finite external acceptance runner

`cmd/weir-accept` is a small current-SDK functional runner for explicitly supplied,
externally orchestrated Weir, MongoDB and Elasticsearch services. It starts no
servers, fixtures or subprocesses. It is not a benchmark, formal qualification,
24-hour observer, node-failure test or HA certificate. Multiple seed opens do not
prove failover. Existing normal/full formal qualification gates remain separate.

## Exact invocation and input

Build from the companion checkout without changing its dependency pins:

```sh
GOENV=off GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
  GOFLAGS=-mod=readonly go build -trimpath \
  -ldflags "-X main.sourceRevision=FULL_COMPANION_COMMIT_SHA" \
  -o .tools/weir-accept ./cmd/weir-accept
```

Build with the native target/toolchain selected by the orchestrator. Accepted
execution targets are Linux `amd64` and `arm64`; `expected_arch` must equal the
binary's actual Go runtime target. A runtime/build target check is not independent
kernel ISA evidence and cannot prove that an orchestrator did not use emulation.
An injected source revision is explicitly an assertion; available Go VCS build
metadata is reported separately. `-buildvcs=false` makes actual VCS metadata
unavailable, not a verified clean source identity.

The runner reads only the explicit regular `.json` input file, at most 16 KiB.
Input symlinks are rejected; use a regular dedicated file or regular ConfigMap
subPath mount rather than a projected-directory symlink.
Do not supply a Secret, `.env`, credential-bearing URI, key path or other private
file. No environment discovery, in-cluster service-account token, Kubernetes
client, kubeconfig, RBAC API call or PVC is used. The orchestrator should disable
automatic service-account token mounting. It must independently check namespace
UID, Pod UID, source-to-image provenance and the actual deployed image digest.
The runner validates their syntax, not their truth.

```json
{
  "expected_arch": "arm64",
  "allow_remote": true,
  "server_source": "0c41296e5c6585480249f34d9d714fbda962c466",
  "server_oci": "sha256:e559ba975b05369853ff7f7528a487044ad1ff0674cacd17867f7c7669aa438b",
  "namespace": "weir-accept-example",
  "namespace_uid": "11111111-1111-1111-1111-111111111111",
  "pod_name": "weir-0",
  "pod_uid": "22222222-2222-2222-2222-222222222222",
  "run_owner": "run12345",
  "application_seeds": [
    "weir-0.weir-headless.weir-accept-example.svc.cluster.local:7777"
  ],
  "mongo_uri": "mongodb://mongo.weir-accept-example.svc.cluster.local:27017/?replicaSet=rs0",
  "search_url": "http://search.weir-accept-example.svc.cluster.local:9200",
  "mongo_store": "mongo",
  "search_store": "search",
  "owned_dataset_prefix": "weiraccept_run12345_"
}
```

The source and arm64 OCI manifest digest above identify an independently prepared
nonstable candidate, not a production default or a runner-verified image. Its
multi-platform index is
`ghcr.io/batchstream/weir@sha256:21caf0607dc325a11e4394de3a10b214ec3caea80a92770df1cdd3bc2f1bb134`;
the amd64 manifest is
`sha256:96dcbbced96c6178361cfdc16ce780ed2664832e7cc88c9f05d9b935dde72373`.
The orchestrator must check the deployed platform manifest/image identity against
its selected candidate. Namespace, Pod, owner and endpoint examples are placeholders,
not evidence of an executed deployment. No stable release or final pin is advanced.
All fields are required. Unknown and duplicate JSON keys, trailing JSON and oversized input are
rejected. `run_owner` is a lowercase letter followed by 7–23 lowercase letters or
digits. `owned_dataset_prefix` must be exactly `weiraccept_<run_owner>_`.
Namespace/Pod names are lowercase DNS labels of at most 63 characters; their UIDs
are canonical lowercase UUIDs. Store names are distinct lowercase identifiers
matching `[a-z][a-z0-9_-]{0,62}`.

Supply one through three distinct canonical `host:port` application seeds. The
pinned SDK uses plaintext application gRPC; this entry does not claim server TLS
coverage. Search is a credential-free HTTP root URL with an explicit port, no
path/query/fragment. Mongo reuses the existing bounded credential-free
`mongodb://` profile: explicit ports, at most seven seeds, optional `replicaSet`
and `directConnection` only. `allow_remote: true` is mandatory explicit opt-in.
Namespace-owned service DNS and ClusterIP addresses are permitted. Syntax checks
are not network isolation: SDK and Mongo discovery can advertise additional
peers. The orchestrator owns endpoint authorization and network boundaries.

```sh
.tools/weir-accept -input /run/weir-accept/config.json
.tools/weir-accept -input /run/weir-accept/config.json \
  -output /reports/weiraccept_run12345_report.json
```

The optional output's existing parent directory must be caller-owned and writable.
The basename must start with the exact owned prefix, contain only lowercase
letters/digits/underscores before `.json`, and be at most 96 characters. Creation
uses exclusive mode `0600`; existing files and output symlinks are never replaced.
The report is always emitted as one bounded JSON line on stdout. There is no
arbitrary-command option. `-version` emits a separate `mode: "build"` report
without opening services; it is not a functional pass.

## Work and lifecycle contract

Budgets are fixed, not input-adjustable: 120 seconds overall, at most 75 seconds
of functional work, and a fresh 3-second context for every ordinary operation,
preparation, cancellation-admission check and cleanup/driver-close phase. Cleanup
uses the remaining overall budget even after caller cancellation. Calls and
callbacks are synchronous; no detached worker is used to fake a successful join.
SDK `Close` joins its actual refresh worker. A failed/unconfirmed close prevents
data deletion and leaves the named owned data for orchestrator remediation.

The runner generates an unpredictable suffix and uses only two new dedicated
names: `<owned_dataset_base>_m` for a Mongo database with collection `records`,
and `<owned_dataset_base>_s` for one Search index. It first checks exact-name
absence. It never adopts existing data. The owner must exclusively reserve the
prefix for this run. An acknowledged or ambiguous create attempt after absence
checking remains an owned cleanup obligation; mutation failures are not retried.

For each explicit seed, the runner initializes the current published SDK with
both Stores, which performs ResolveStore and readies direct business connections.
For each backend it then performs:

1. A typed missing-document read.
2. One `Put` of a small deterministic record with query-visible completion.
3. One Lua atomic increment, followed by a typed read of the full expected record.
4. A deliberately rejected Lua transform, followed by a read proving its attempted
   document change was not published.
5. One read-only Native count, bounded to 16 KiB and requiring a complete response,
   successful backend status and exactly one record. No cursor is created.
6. An already-canceled read checking cancellation **admission**, not an in-flight
   cancellation/drain or server-credit-conservation proof.
7. Independent native-driver/HTTP full-field and exact-count oracles.

Only one application record per backend is used. Each seed gets a fresh SDK
client; no node is removed or failed. Business requests are issued once. SDK
business replay, Mongo retryReads/retryWrites/adaptive retries and Search mutation
retries are disabled; an unknown/unconfirmed mutation fails the run, is retained
as such, and is never replayed. Discovery is control-plane behavior, not business
request replay. Expected Lua rejection is a confirmed non-applied outcome, not
successful throughput. Unexpected failure stops further functional work.

All SDK clients are closed/joined and the original native clients closed before
any deletion. Fresh bounded native cleanup connections delete only the owned
Mongo database/Search index and check their exact absence, then close themselves.
`cleanup_verified` means those actual deletion/absence/close operations passed.
`cleanup_not_owned` means no ownership was acquired; it is not a leftovers or
server-ledger-zero claim. Failure/skipped cleanup remains failure. No diagnostic
or server-ledger evidence is fabricated or inferred.

## Report and deployment boundary

A functional pass requires exit code zero **and** `schema: 1`,
`mode: "functional"`, `passed: true`, all expected per-seed/backend checks and
actual cleanup checks. Every other functional outcome exits nonzero. Reports
are at most 64 KiB, retain bounded check timings, protocol numeric failure/outcome/
completion enums and unknown flags, and never include endpoints, credentials,
request payloads, raw errors or server messages. `error_code` is 0 (no transport
error), 1 (failure), 2 (deadline), 3 (canceled), or 4 (input rejection); a failed
assertion can have error code 0 while `passed` is false. Expected cancellation
has code 3 while its check passes.

`caller_asserted_provenance` is intentionally distinguished from `runner_build`.
The former is supplied source/digest/namespace/Pod/owner information, not
Kubernetes or image inspection. The latter contains actual Go runtime target,
linked SDK/protocol versions and available VCS metadata; `injected_source` is
separately labeled. Missing build metadata remains `unknown`/absent.

For the first single-node functional job, use small external resource bounds:
runner requests no more than `250m` CPU and `256Mi` memory, with corresponding
owned job limits as selected by the chart author. The binary uses one Go
scheduler processor, a 64 MiB soft Go memory limit, native pool size two and at
most two HTTP connections. These bounds are not a measured RSS/CPU certification;
actual resource enforcement belongs to the orchestrator. Allow the 120-second
runner budget plus Pod startup/report collection time in the external Job
lifecycle. External dependency provisioning, Pod deletion, namespace cleanup,
image/source proof and any future multi-node failure tests are not this runner's
responsibility. No quotas, deadlines or production qualification gates are relaxed.

Offline tests use fake SDK/backend boundaries and HTTP transports, not localhost
servers or live databases. Run normal unit/race tests and native/cross-target
compilation using the cached pinned toolchain. Live functional acceptance still
requires the separately orchestrated owned environment and actual evidence.
