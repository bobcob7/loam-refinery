BIN := bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.2.0-dev)

.PHONY: build test lint generate clean

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BIN)/loam-refinery ./cmd/loam-refinery

test:
	go test ./...

# lint is the gate a contributor is expected to run: formatting and vet.
lint:
	go tool gofumpt -l -e . | tee /dev/stderr | (! read)
	go vet ./...

# generate is the single entry point for code generation.
generate:
	go generate ./...

clean:
	rm -rf $(BIN)
