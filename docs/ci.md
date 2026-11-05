# CI and output

What CI runs, which versions it pins and where output goes.

## Pinned versions

The Weir server commit and the database images are pinned in
[versions.json](../versions.json) by full commit; `go.mod` pins released SDK and
protocol versions without replacements. `versions.json` also names the previous
server and SDK, which CI tests against the current other side. The tests depend
on no server module or package.

## CI

CI runs unit, race, vet and module-boundary checks, real multi-node integration,
and verified ordinary and Lua comparison smoke tests on both databases, and
uploads results and fixture logs as a workflow artifact. It also runs the
integration tests with each side one version behind, since a server and its
clients are not upgraded together. After `make prepare prepare-previous`, `make
integration-previous-server` uses the current SDK with the `weir_previous_source`
server and `make integration-previous-sdk` uses the current server with the
`weir_go_previous` SDK and the protocol version it was released with. When pinning
a new server or SDK, move that side's previous pin to the version it replaces; CI
checks that each differs from the current one and that the SDK's is the release
right before it. Both runs set the `previous` build tag: a test or helper that
only the current versions can pass goes in a file with a `!previous` build
constraint so that the rest keeps running, until the previous pin has that
behaviour too.

The SDK's API changed at v0.24.0, so `integration-previous-sdk` also sets the
`previous_sdk` tag. `sdk_current_test.go` and `sdk_previous_test.go` hold the calls
that differ. The soak and benchmark tests use packages built on the current API
only, so they do not run on the previous SDK. When `weir_go_previous` reaches
v0.24.0, delete the tag, `sdk_previous_test.go` and the constraints on those tests.

## Output

All generated output belongs under the ignored `results/` directory, conventionally
`results/local/`. Nothing under `results/` is committed.
