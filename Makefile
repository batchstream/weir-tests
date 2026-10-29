.PHONY: prepare test integration benchmark diagnostic

WEIR_SOURCE := $(shell jq -r .weir_source versions.json)

prepare:
	GOBIN="$(CURDIR)/.tools" go install -ldflags "-X main.revision=$(WEIR_SOURCE)" github.com/batchstream/weir/cmd/weir@$(WEIR_SOURCE)

test:
	go test -race -count=1 ./...
	python3 -m unittest discover -s scripts -p '*_test.py'

integration:
	WEIR_TEST_BINARY="$(CURDIR)/.tools/weir" go test -race -tags=integration -count=1 -timeout=10m -v ./tests/integration

benchmark:
	go run ./cmd/weir-lab -mode saturation -weir "$(CURDIR)/.tools/weir" -backend all -database-cpus 1 -client-processes 4 -concurrency-levels 4,8,32,128,256 -batch-sizes 1 -records 8192 -write-percent 0 -rounds 3 -warmup-duration 10s -duration 20s -output results/local/benchmark

diagnostic:
	go run ./cmd/weir-lab -mode fixed -weir "$(CURDIR)/.tools/weir" -output results/local/diagnostic
