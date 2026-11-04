# Contributing to the Weir system tests

New scenarios, fixes, documentation improvements and proposals are welcome. Problems
in the product itself belong to the [server](https://github.com/batchstream/weir/issues) or
[Go SDK](https://github.com/batchstream/weir-go/issues) repositories.

## Report a problem

Use the issue forms and include the commit of this repository, the Weir revision
pinned in `versions.json`, the OS and Docker versions, the command and its output.
Remove credentials and private data from logs.

For vulnerabilities, follow [SECURITY.md](SECURITY.md).

## Work locally

Install Go, Python 3, `jq` and a local Docker daemon with a Unix socket.

```sh
git clone https://github.com/batchstream/weir-tests.git
cd weir-tests
make prepare
make test
make integration
```

`make test` launches no databases or Weir processes. See
[Native MongoDB fixture](docs/integration.md#native-mongodb-fixture) for the fixture option.

## Prepare a pull request

- Keep the change focused and explain the observed problem and resulting behavior.
- Add a regression test when fixing behavior.
- Use only the published SDK and protocol; this module imports no server packages.
- Keep generated output under the ignored `results/` directory.
