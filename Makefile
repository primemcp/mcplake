.PHONY: help build test test-race clean lint fmt vet tidy check sync

help:
	@echo "mcplake Gateway - Makefile targets"
	@echo ""
	@echo "  make help        - Show this help"
	@echo "  make build       - Build the gateway binary"
	@echo "  make test        - Run all tests (all modules)"
	@echo "  make test-race   - Run tests with race detector"
	@echo "  make lint        - Run go vet (all modules)"
	@echo "  make fmt         - Format code (all modules)"
	@echo "  make vet         - Run go vet (all modules)"
	@echo "  make tidy        - Sync workspace and tidy all modules"
	@echo "  make sync        - Sync Go workspace"
	@echo "  make check       - Run all checks (fmt, vet, test, build)"
	@echo "  make clean       - Remove build artifacts"
	@echo ""
	@echo "Multi-module workspace managed by go.work"
	@echo "To add a module: go work use ./path && make tidy"

build:
	@echo "Building mcplake gateway..."
	go build -o ./cmd/gateway/mcp-gateway ./cmd/gateway
	@echo "✓ Built: ./cmd/gateway/mcp-gateway"

test:
	@echo "Running tests (all modules)..."
	go test -v -cover ./auth/... ./router/... ./cache/... ./filter/... ./mcp/... ./config/... ./persistence/... ./gateway/... ./cmd/gateway/...

test-race:
	@echo "Running tests with race detector..."
	go test -race ./auth/... ./router/... ./cache/... ./filter/... ./mcp/... ./config/... ./persistence/... ./gateway/... ./cmd/gateway/...

lint:
	@echo "Running go vet..."
	go vet ./auth/... ./router/... ./cache/... ./filter/... ./mcp/... ./config/... ./persistence/... ./gateway/... ./cmd/gateway/...

fmt:
	@echo "Formatting code..."
	gofmt -w auth/ router/ cache/ filter/ mcp/ config/ persistence/ gateway/ cmd/

vet:
	@echo "Running go vet..."
	go vet ./auth/... ./router/... ./cache/... ./filter/... ./mcp/... ./config/... ./persistence/... ./gateway/... ./cmd/gateway/...

tidy:
	@echo "Syncing workspace..."
	go work sync
	@echo "✓ Workspace synced"

sync:
	@echo "Syncing Go workspace..."
	go work sync
	@echo "✓ Workspace synced"

check: fmt vet test build
	@echo "✓ All checks passed"

clean:
	@echo "Cleaning build artifacts..."
	rm -f ./cmd/gateway/mcp-gateway
	go clean -cache
	go clean
	@echo "✓ Clean complete"

.PHONY: help build test test-race clean lint fmt vet tidy check sync
