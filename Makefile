.PHONY: prepare test integration benchmark

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
	GOWORK=off go run ./cmd/weir-lab -weir "$(CURDIR)/.tools/weir" -output results/local/benchmark
