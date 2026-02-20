.PHONY: help test test-verbose coverage lint fmt vet vulncheck tidy clean ci doc

## help: show available targets (default)
help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

test: ## Run tests with race detector
	go test -race ./...

test-verbose: ## Run tests with verbose output and race detector
	go test -race -v ./...

coverage: ## Generate coverage report
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

lint: ## Run golangci-lint
	golangci-lint run ./...

fmt: ## Run goimports formatting
	goimports -w -local github.com/fugue-labs/gollem .

vet: ## Run go vet
	go vet ./...

vulncheck: ## Run govulncheck
	govulncheck ./...

tidy: ## Run go mod tidy and verify
	go mod tidy
	go mod verify

clean: ## Remove build artifacts
	rm -f coverage.out coverage.html
	go clean -testcache

ci: lint vet test vulncheck ## Run full CI pipeline locally

doc: ## Start local pkgsite documentation server
	@echo "Starting pkgsite on http://localhost:8080"
	pkgsite -http=:8080
