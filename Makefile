.PHONY: prepare test integration benchmark diagnostic

prepare:
	python3 scripts/download_modules.py
	python3 scripts/prepare_weir.py
	python3 scripts/prepare_images.py

test:
	GOWORK=off GOPROXY=off GOSUMDB=off go test -race -count=1 ./...
	python3 -m unittest discover -s scripts -p '*_test.py'

integration:
	WEIR_TEST_BINARY="$(CURDIR)/.tools/weir" GOWORK=off go test -race -tags=integration -count=1 -timeout=10m -v ./tests/integration

benchmark:
	GOWORK=off go run ./cmd/weir-lab -mode saturation -weir "$(CURDIR)/.tools/weir" -backend all -database-cpus 1 -store-concurrency 32 -backend-batch-limit 32 -client-processes 4 -concurrency-levels 8,32,128,512 -batch-sizes 1 -records 2048 -write-percent 0 -rounds 3 -warmup-duration 10s -duration 20s -output results/local/benchmark

diagnostic:
	GOWORK=off go run ./cmd/weir-lab -mode fixed -weir "$(CURDIR)/.tools/weir" -output results/local/diagnostic
