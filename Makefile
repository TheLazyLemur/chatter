.PHONY: build run app install setup test/unit test/integration test lint

APP := bin/Chatter.app
VENV := $(HOME)/Library/Application Support/Chatter/.venv

build:
	go build -o bin/chatter ./cmd/chatter
	go build -o bin/chatter-cli ./cmd/chatter-cli

run: build
	./bin/chatter

# Ad-hoc signed bundle; it still links Homebrew's SDL2 and sdl2_ttf.
app: build
	rm -rf $(APP)
	mkdir -p $(APP)/Contents/MacOS
	cp cmd/chatter/Info.plist $(APP)/Contents/
	cp bin/chatter $(APP)/Contents/MacOS/
	codesign --force --sign - $(APP)

install: app
	rm -rf /Applications/Chatter.app
	cp -R $(APP) /Applications/

# Phonon-2 runtime (Python 3.10+, Apple silicon) into the config dir, where Chatter finds bin/fermion itself.
setup:
	python3.12 -m venv "$(VENV)"
	"$(VENV)/bin/pip" install "fermion-research[mlx]"
	"$(VENV)/bin/fermion" transcribe phonon-2 --download-only

test/unit:
	TEST_TIER=unit go test -count=1 ./internal/... ./pkg/...

test/integration:
	TEST_TIER=integration go test -count=1 ./internal/... ./pkg/...

test:
	go test -race -count=1 ./...

lint:
	go vet ./...
	@test -z "$$(gofmt -l cmd internal pkg)" || (gofmt -l cmd internal pkg; exit 1)
