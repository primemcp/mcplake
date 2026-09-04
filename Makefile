.PHONY: help build test test-race clean lint fmt vet tidy check sync swagger swagger-check

help:
	@echo "mcplake Gateway - Makefile targets"
	@echo ""
	@echo "  make help           - Show this help"
	@echo "  make build          - Build the gateway binary"
	@echo "  make test           - Run all tests (all modules)"
	@echo "  make test-race      - Run tests with race detector"
	@echo "  make lint           - Run go vet (all modules)"
	@echo "  make fmt            - Format code (all modules)"
	@echo "  make vet            - Run go vet (all modules)"
	@echo "  make tidy           - Sync workspace and tidy all modules"
	@echo "  make sync           - Sync Go workspace"
	@echo "  make check          - Run all checks (fmt, vet, test, build)"
	@echo "  make swagger        - Regenerate the control-plane OpenAPI spec (requires swag: go install github.com/swaggo/swag/cmd/swag@latest)"
	@echo "  make swagger-check  - Regenerate the spec and fail if it differs from what's committed (the CI step from ADR-0007)"
	@echo "  make clean          - Remove build artifacts"
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

# swagger regenerates the control-plane OpenAPI spec (ADR-0007) from the
# @-annotations on Gin handlers in gateway/internal/controlplane. Requires
# the swag CLI (go install github.com/swaggo/swag/cmd/swag@latest).
swagger:
	@echo "Regenerating control-plane swagger docs..."
	cd gateway && swag init -g doc.go -d ./internal/controlplane -o ./internal/controlplane/docs --parseInternal
	@echo "✓ Swagger docs regenerated"

# swagger-check is the CI step described in ADR-0007: fail if a handler's
# annotations changed without the committed spec being regenerated to match.
swagger-check: swagger
	@echo "Checking committed swagger docs are up to date..."
	git add -N gateway/internal/controlplane/docs
	git diff --exit-code -- gateway/internal/controlplane/docs || \
		(echo "✗ swagger docs are stale — run 'make swagger' and commit the result" && exit 1)
	@echo "✓ Swagger docs are up to date"

clean:
	@echo "Cleaning build artifacts..."
	rm -f ./cmd/gateway/mcp-gateway
	go clean -cache
	go clean
	@echo "✓ Clean complete"

.PHONY: help build test test-race clean lint fmt vet tidy check sync swagger swagger-check
