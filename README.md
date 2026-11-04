# Weir system tests

[![CI](https://github.com/batchstream/weir-tests/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/batchstream/weir-tests/actions/workflows/ci.yml) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Independent blackbox integration tests and database throughput comparisons for
[Weir](https://github.com/batchstream/weir).

## Overview

Applications do not need this repository. Use it to test Weir across components
or to measure its performance. This module uses the published Go SDK and public
protocol. It imports no server packages.

### Tools

| Tool | Purpose |
| --- | --- |
| `tests/integration` | Real multi-node integration tests against owned MongoDB and Elasticsearch fixtures |
| `cmd/weir-lab` | Paired native-versus-Weir throughput comparisons on owned fixtures |
| `scripts/run_performance_matrix.py`, `scripts/summarize_performance.py` | Run a read/mixed/write/Lua matrix and compare two matrix runs |
| `cmd/weir-crawler` | [Crawler mutation benchmark](docs/crawler-benchmark.md) on an owned Search fixture |
| `cmd/weir-bench` | Fixed and saturation comparisons against independently deployed endpoints |
| `cmd/weir-soak`, `scripts/soak-faults.sh` | Long-running load through deployed Weir replicas, with every outcome checked against the databases, under scheduled faults |

### Make targets

| Target | Action |
| --- | --- |
| `make prepare` | Install the pinned Weir server into `.tools/` |
| `make test` | Run unit and race tests. Starts no databases or Weir processes. |
| `make integration` | Run integration tests against real databases and Weir replicas |
| `make benchmark` | Run the saturation benchmark |
| `make diagnostic` | Run a finite single-operation comparison |

## Requirements

| Item | Requirement |
| --- | --- |
| Go | 1.27.3 |
| Python | Python 3 |
| `jq` | Any version |
| Docker | A local daemon with a Unix socket |
| Benchmarks | Linux with Docker, to enforce database CPU quotas |

Fixtures use ordinary endpoints and processes and work without Kubernetes.

## Quick start

**Step 1.** Get the repository.

```sh
git clone https://github.com/batchstream/weir-tests.git
cd weir-tests
```

**Step 2.** Install the pinned Weir server.

```sh
make prepare
```

**Step 3.** Run the unit tests.

```sh
make test
```

**Step 4.** Run the integration tests. Docker must be running. The run has a
10-minute limit.

```sh
make integration
```

Both test targets print `ok` for each passing package. Generated output belongs
under the ignored `results/` directory.

## Documentation

| Guide | Contents |
| --- | --- |
| [Integration tests](docs/integration.md) | Preparation, the native MongoDB fixture and coverage |
| [Benchmarks](docs/benchmarks.md) | Saturation benchmark, performance matrix and deployed endpoints |
| [Crawler benchmark](docs/crawler-benchmark.md) | Crawler mutation workload |
| [Soak](docs/soak.md) | Long load, outcome checks, fault injection and rollout scenarios |
| [CI and output](docs/ci.md) | Pinned versions, CI jobs and generated output |

## Development

```sh
go test -race ./...
go vet ./...
python3 -m unittest discover -s scripts -p '*_test.py'
```

[versions.json](versions.json) pins the Weir server and database images. `go.mod`
pins the SDK and protocol. See [CI and output](docs/ci.md) before changing a pin.

- Contributing: [CONTRIBUTING.md](CONTRIBUTING.md)
- Security reports: [SECURITY.md](SECURITY.md)

## Related repositories

| Repository | Purpose |
| --- | --- |
| [weir](https://github.com/batchstream/weir) | Server |
| [weir-go](https://github.com/batchstream/weir-go) | Go client SDK |
| [weir-protocol](https://github.com/batchstream/weir-protocol) | Public gRPC schema and validation |
| [weir-charts](https://github.com/batchstream/weir-charts) | Helm chart |
| **[weir-tests](https://github.com/batchstream/weir-tests)** | System tests and benchmarks (this repository) |

## License

MIT. See [LICENSE](LICENSE).
