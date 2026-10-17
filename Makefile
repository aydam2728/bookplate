# BookPlate development tasks.
# `make check` runs exactly what CI runs: nothing more, nothing less.

GO   ?= go
BIN  := bin/bookplate
PKGS := ./...

.PHONY: help
help: ## List the available targets
	@grep -hE "^[a-z-]+:.*?## " $(MAKEFILE_LIST) | sort | \
		awk "BEGIN{FS=\":.*?## \"}{printf \"  %-12s %s\n\", \$$1, \$$2}"

.PHONY: check
check: fmt-check vet test ## Everything CI runs

.PHONY: fmt
fmt: ## Format the Go sources
	$(GO) fmt $(PKGS)

.PHONY: fmt-check
fmt-check: ## Fail if any Go source is not formatted
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "not formatted:"; echo "$$unformatted"; exit 1; \
	fi

.PHONY: vet
vet: ## Run the Go vetter
	$(GO) vet $(PKGS)

.PHONY: test
test: ## Run the tests
	$(GO) test $(PKGS)

.PHONY: build
build: ## Build the server binary
	$(GO) build -o $(BIN) ./cmd/bookplate

.PHONY: run
run: ## Run a local instance
	$(GO) run ./cmd/bookplate

.PHONY: clean
clean: ## Remove build output
	rm -rf bin

IMAGE   ?= ghcr.io/aydam2728/bookplate
VERSION ?= dev

.PHONY: image
image: ## Build the container image for this machine architecture
	docker build -t $(IMAGE):$(VERSION) --build-arg VERSION=$(VERSION) .

.PHONY: image-multiarch
image-multiarch: ## Build for amd64 and arm64 without publishing
	docker buildx build --platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) .

.PHONY: dev
dev: ## Build from these sources and run the container locally
	docker compose -f compose.dev.yaml up --build -d

.PHONY: dev-down
dev-down: ## Stop the local container and drop its volume
	docker compose -f compose.dev.yaml down -v
