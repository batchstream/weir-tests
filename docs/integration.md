# Integration tests

What `make prepare`, `make test` and `make integration` do and cover.

## Preparation

`make prepare` runs `go install` for `github.com/batchstream/weir/cmd/weir` at
the pinned commit into `.tools/weir` and stamps that commit as its `revision`.
Default Go tests, including race tests, launch no databases or Weir processes.
They validate request accounting, mutation outcomes without replay, scheduling,
resource sampling, saturation checks and fixture ownership.

## Native MongoDB fixture

On macOS (arm64/amd64) or Ubuntu 24.04 amd64, an explicitly selected native
MongoDB fixture is also available:

```sh
mongod=$(scripts/mongod.sh) && export WEIR_TEST_MONGODB_BINARY="$mongod"
```

`scripts/mongod.sh` downloads the MongoDB 8.0.32 archive for the host (the Linux
build targets Ubuntu 24.04), checks
its pinned SHA-256 and prints the installed `mongod` path. Each test owns a new
replica set, data directory and loopback port. Direct and Weir paths use the
same database mode. The blackbox CI job exports `WEIR_TEST_MONGODB_BINARY` for
integration and both comparison smokes; Search uses its Docker fixture. Without
that explicit selection the Mongo fixture is Docker; there is no runtime
fallback. The native fixture has no database cgroup limits.

## Run

```sh
make integration
```

## Coverage

The opt-in `integration` build tag starts MongoDB, Elasticsearch and two
independent Weir replicas hosting the same Stores. Tagged tests require a
prepared `WEIR_TEST_BINARY`. Coverage includes SDK reads through every replica,
NotFound for an unhosted Store, typed CRUD and Lua transformations, independent
persistence checks, same-Store calls containing many records sent on one stream,
slice preflight and per-record validation, Native MongoDB
find/getMore and Search scroll pagination through alternating replicas,
backend-native filters and include/exclude projections, Native response metadata,
cancellation, calls recovering within keepalive from a connection that goes
silent, the soak's default profiles without faults, and a replica stop and
restart that the other replica and the original client survive.

## Lua, Native and pagination

Lua source returns one function receiving the current and incoming documents as
ordinary tables. Tests cover object replacements, explicit keep/delete/reject
actions, first creation, and concurrent read-modify-write through both replicas.
Nil, missing or multiple callback results return INVALID_ARGUMENT, including a
callback that returns a missing current document. Missing records and missing
backend targets have distinct typed outcomes. Native keeps backend semantics:
MongoDB find on a missing collection succeeds with an empty cursor; Search returns
a complete HTTP 404 with index/record absence in its body. Pagination uses one
record per page, at most eight pages, ten Native calls, 1 MiB per response and
10 MiB total per paginator (including cleanup/probes). Callers own backend cursor
IDs, track rotating scroll IDs, and explicitly reclaim live cursors on early stop
or errors using a separate five-second cleanup deadline. Exhausted Search scrolls
are also cleared. Closed continuations retain complete Native transport metadata
alongside MongoDB CursorNotFound or Search HTTP 404 business errors. There is no
Weir continuation/checkpoint or projection-token binding contract. Search scroll
requests use the API common to Elasticsearch and OpenSearch; the fixture uses the
pinned Elasticsearch version. Separate multi-process tests verify independent
single-record callers.

## Large reads and interrupted streams

One-RPC reads deliver 64 MiB and 256 MiB across 1024 and 4096 distinct documents
while checking that queued records stay within the stream window and sampling
replica RSS. Interrupted streams retain confirmed record outcomes; unconfirmed
mutations remain indeterminate.

## Records and document formats

Each Execute request carries one record and its consecutive index. The client
streams records as they are produced; server aggregation stays within the Store.
Later invalid records stop the stream while preserving earlier acknowledged writes.
Documents use explicit `ContentType` fields. Native carries an opaque request
Document and optional response metadata; adapters own the formats. HTTP fixtures
use standard messages through SDK helpers. Tests verify unknown content types
reach adapter classification. Projection syntax and business errors belong to the
backend, not a portable Weir projection/token layer. Native transfer completion
is checked separately from MongoDB `ok` and HTTP status/body, including complete
backend error responses.

## Fixture ownership

Services bind to loopback and own exclusive namespaces. Cleanup verifies exact
process and container ownership. Fixture logs, manifests and `cleanup.json` are
kept in the printed output directory, including after startup failures.
