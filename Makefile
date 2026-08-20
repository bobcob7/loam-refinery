BIN := bin
GOBIN := $(CURDIR)/$(BIN)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.2.0-dev)

.PHONY: build test lint generate tools clean

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BIN)/loam-refinery ./cmd/loam-refinery

test:
	go test ./...

# lint is what CI enforces: formatting, vet, and a build of everything.
lint: tools
	$(GOBIN)/gofumpt -l -e . | tee /dev/stderr | (! read)
	go vet ./...

# generate is the single entry point for code generation.
generate: tools
	PATH="$(GOBIN):$$PATH" go generate ./...

tools:
	GOBIN=$(GOBIN) go install github.com/matryer/moq
	GOBIN=$(GOBIN) go install mvdan.cc/gofumpt
	GOBIN=$(GOBIN) go install github.com/sqlc-dev/sqlc/cmd/sqlc

clean:
	rm -rf $(BIN)
