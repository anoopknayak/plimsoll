.PHONY: all test lint build tidy cover clean

GO ?= go
BIN_DIR := bin
COVER_PROFILE := coverage.out
COVER_MIN ?= 70

all: lint test build

## test: run the full test suite with race detector and coverage
test:
	$(GO) test ./... -race -cover

## cover: produce a coverage profile and enforce the minimum coverage gate
cover:
	$(GO) test ./... -race -coverprofile=$(COVER_PROFILE) -covermode=atomic
	@total=$$($(GO) tool cover -func=$(COVER_PROFILE) | awk '/^total:/ {gsub("%","",$$3); print $$3}'); \
	echo "total coverage: $$total% (min $(COVER_MIN)%)"; \
	awk "BEGIN { exit !($$total >= $(COVER_MIN)) }" || { echo "coverage below gate"; exit 1; }

## lint: vet the codebase (gofmt check + go vet)
lint:
	@gofmt -l . | tee /tmp/plimsoll-gofmt.out; test ! -s /tmp/plimsoll-gofmt.out
	$(GO) vet ./...

## build: compile the plimsoll and pricing-gen binaries
build:
	$(GO) build -o $(BIN_DIR)/plimsoll ./cmd/plimsoll
	$(GO) build -o $(BIN_DIR)/pricing-gen ./cmd/pricing-gen

## tidy: sync go.mod/go.sum
tidy:
	$(GO) mod tidy

## clean: remove build and coverage artifacts
clean:
	rm -rf $(BIN_DIR) $(COVER_PROFILE) coverage.html
