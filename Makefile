.PHONY: prepare prepare-previous test integration integration-previous-server integration-previous-sdk benchmark diagnostic

prepare:
	revision=$$(jq -er .weir_source versions.json) && \
		GOBIN="$(CURDIR)/.tools" go install -ldflags "-X main.revision=$$revision" github.com/batchstream/weir/cmd/weir@$$revision

prepare-previous:
	revision=$$(jq -er .weir_previous_source versions.json) && \
		GOBIN="$(CURDIR)/.tools/previous" go install -ldflags "-X main.revision=$$revision" github.com/batchstream/weir/cmd/weir@$$revision

test:
	go test -race -count=1 ./...
	python3 -m unittest discover -s scripts -p '*_test.py'

integration:
	WEIR_TEST_BINARY="$(CURDIR)/.tools/weir" go test -race -tags=integration -count=1 -timeout=10m -v ./tests/integration

integration-previous-server:
	WEIR_TEST_BINARY="$(CURDIR)/.tools/previous/weir" go test -race -tags=integration,previous -count=1 -timeout=10m -v ./tests/integration

# GOFLAGS, not a flag of go test, so that any binary a test builds uses the same SDK.
# The SDK brings the protocol version it was released with, as it does for a caller.
integration-previous-sdk:
	mkdir -p .tools
	cp go.mod .tools/previous-sdk.mod
	cp go.sum .tools/previous-sdk.sum
	version=$$(jq -er .weir_go_previous versions.json) && \
		protocol=$$(go mod download -json github.com/batchstream/weir-go@$$version | jq -er .GoMod | xargs awk '$$1 == "github.com/batchstream/weir-protocol" {print $$2}') && \
		go get -modfile=.tools/previous-sdk.mod github.com/batchstream/weir-go@$$version github.com/batchstream/weir-protocol@$$protocol
	GOFLAGS=-modfile="$(CURDIR)/.tools/previous-sdk.mod" WEIR_TEST_BINARY="$(CURDIR)/.tools/weir" go test -race -tags=integration,previous,previous_sdk -count=1 -timeout=10m -v ./tests/integration

benchmark:
	go run ./cmd/weir-lab -mode saturation -weir "$(CURDIR)/.tools/weir" -backend all -database-cpus 1 -client-processes 4 -concurrency-levels 4,8,32,128,256 -batch-sizes 1 -records 8192 -write-percent 0 -rounds 3 -warmup-duration 10s -duration 20s -output results/local/benchmark

diagnostic:
	go run ./cmd/weir-lab -mode fixed -weir "$(CURDIR)/.tools/weir" -output results/local/diagnostic
