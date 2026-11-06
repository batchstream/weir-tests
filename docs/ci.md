# CI and output

What CI runs, which versions it pins and where output goes.

## Pinned versions

The Weir server commit and the database images are pinned in
[versions.json](../versions.json) by full commit; `go.mod` pins released SDK and
protocol versions without replacements. The tests depend on no server module or
package.

## CI

CI runs unit, race, vet and module-boundary checks, real multi-node integration,
and verified ordinary and Lua comparison smoke tests on both databases, and
uploads results and fixture logs as a workflow artifact.

## Output

All generated output belongs under the ignored `results/` directory, conventionally
`results/local/`. Nothing under `results/` is committed.
