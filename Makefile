.PHONY: all test lint build tidy cover clean refresh-pricing

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

## refresh-pricing: regenerate the embedded snapshots from upstream pricing
## sources. Requires source URLs (and a GCP API key) via environment variables;
## intended to run in the scheduled CI refresh workflow.
##   GCP_CATALOG_URL, AWS_PRICELIST_URL, AZURE_RETAIL_URL
##   GCP_REGION, AWS_REGION, AZURE_REGION
refresh-pricing: build
	$(BIN_DIR)/pricing-gen --cloud gcp   --region "$(GCP_REGION)"   --url "$(GCP_CATALOG_URL)"  --out internal/pricing/data
	$(BIN_DIR)/pricing-gen --cloud aws   --region "$(AWS_REGION)"   --url "$(AWS_PRICELIST_URL)" --out internal/pricing/data
	$(BIN_DIR)/pricing-gen --cloud azure --region "$(AZURE_REGION)" --url "$(AZURE_RETAIL_URL)"  --out internal/pricing/data

## clean: remove build and coverage artifacts
clean:
	rm -rf $(BIN_DIR) $(COVER_PROFILE) coverage.html
