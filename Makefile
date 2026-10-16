.PHONY: build run setup test/unit test/integration test lint

build:
	go build -o bin/chatter ./cmd/chatter
	go build -o bin/chatter-cli ./cmd/chatter-cli

run: build
	./bin/chatter

# Phonon-2 runtime (Python 3.10+, Apple silicon) into .venv; Chatter finds .venv/bin/fermion itself.
setup:
	python3.12 -m venv .venv
	.venv/bin/pip install "fermion-research[mlx]"
	.venv/bin/fermion transcribe phonon-2 --download-only

test/unit:
	TEST_TIER=unit go test -count=1 ./internal/... ./pkg/...

test/integration:
	TEST_TIER=integration go test -count=1 ./internal/... ./pkg/...

test:
	go test -race -count=1 ./...

lint:
	go vet ./...
	@test -z "$$(gofmt -l cmd internal pkg)" || (gofmt -l cmd internal pkg; exit 1)
