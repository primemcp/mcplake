---
name: development

description: >
  This skill defines general software-development practices for Go projects.
  Architecture and architectural patterns are defined by the separate `architecture` skill.
  Documentation practices are defined by the separate `documentation` skill.
  Do not introduce architectural decisions through this skill unless explicitly required by the project or another applicable skill.
---

# 1. Go Version

Use **Go 1.27.1** for this project.

Verify the installed version when necessary:

```bash
go version
```

The project should explicitly declare the Go version in its `go.mod` and/or workspace configuration as appropriate.

Use modern Go features when they improve the code.

Go 1.27 includes improvements to generics, including generic methods and improved type inference.

Do not use generics merely because they are available.

Prefer straightforward concrete Go code when it is clearer.

---

# 2. Idiomatic Go

Write idiomatic Go.

Prefer:

* simple code
* explicit control flow
* small functions
* meaningful names
* small interfaces
* explicit error handling
* composition over unnecessary abstraction
* standard-library solutions where practical

Avoid:

* unnecessary abstractions
* excessive interfaces
* deeply nested control flow
* clever one-liners
* premature optimization
* unnecessary reflection
* unnecessary generics
* global mutable state

Follow standard Go conventions unless the project has a documented reason to do otherwise.

---

# 3. TDD

Use **Test-Driven Development** for new functionality.

Follow:

```text
RED
  ↓
Write a failing test
  ↓
GREEN
  ↓
Implement the minimum required behavior
  ↓
REFACTOR
  ↓
Improve the implementation while keeping tests green
```

For new behavior:

1. Define the expected behavior.
2. Write the test first.
3. Run the test and verify that it fails for the expected reason.
4. Implement the minimum required code.
5. Run the test.
6. Refactor if necessary.
7. Run the broader test suite.

Do not routinely implement a large feature first and add tests afterward.

Tests should influence API design and make code easier to use and maintain.

---

# 4. Testify

**Always use Testify for Go tests.**

Use:

```text
github.com/stretchr/testify
```

Prefer Testify assertions such as:

```go
require.NoError(t, err)
assert.Equal(t, expected, actual)
assert.Error(t, err)
assert.NotNil(t, value)
```

Use `require` when the test cannot meaningfully continue after a failure.

Use `assert` when subsequent assertions remain useful.

Use Testify's mocking facilities when mocking is appropriate.

Do not mock everything.

Prefer real implementations, simple fakes, or in-memory implementations when they make tests clearer and more reliable."log/slog"

---

# 5. Test Organization

Keep tests close to the code they test.

Typical structure:

```text
package/
    thing.go
    thing_test.go
```

Use external test packages when testing only the public API is intentional:

```go
package thing_test
```

Otherwise, use the package's normal test package when testing internal behavior is appropriate.

---

# 6. Table-Driven Tests

Prefer table-driven tests when multiple inputs exercise the same behavior.

Example:

```go
func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Value
		wantErr bool
	}{
		{
			name:  "valid value",
			input: "example",
			want:  Value("example"),
		},
		{
			name:    "empty value",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// test
		})
	}
}
```

Use individual tests when they communicate the behavior more clearly.

Do not force unrelated cases into one table.

---

# 7. Test Naming

Test names should describe behavior.

Prefer:

```go
func TestParse_ReturnsErrorForEmptyInput(t *testing.T)
```

over:

```go
func TestParse2(t *testing.T)
```

Use subtests with descriptive names.

Tests should make failures understandable without reading the implementation.

---

# 8. Test Quality

Tests should verify behavior, not implementation details unnecessarily.

Prefer:

```text
given input
    ↓
expected behavior
```

over tests that merely reproduce the implementation's internal algorithm.

Good tests should be:

* deterministic
* isolated
* readable
* fast where possible
* repeatable
* independent of execution order

Avoid tests that depend on:

* arbitrary sleeps
* timing assumptions
* global mutable state
* uncontrolled randomness
* external services unless explicitly an integration test

---

# 9. Testing Levels

Use the appropriate type of test for the behavior.

### Unit tests

Use for isolated logic and components.

### Integration tests

Use when verifying interaction with real dependencies such as:

* databases
* message brokers
* filesystem
* external services
* network components

### End-to-end tests

Use for important complete workflows.

Do not push all testing into E2E tests.

Most behavior should be covered by fast unit tests, with integration and E2E tests covering boundaries and critical workflows.

---

# 10. Race Detection

For code involving concurrency, run:

```bash
go test -race ./...
```

Use race detection regularly for concurrent components.

The race detector should be part of the project's Makefile.

---

# 11. Errors

Handle errors explicitly.

Prefer:

```go
if err != nil {
	return err
}
```

Wrap errors with useful context:

```go
return fmt.Errorf("load configuration: %w", err)
```

Use `%w` when callers need to inspect the underlying error.

Use:

```go
errors.Is(err, target)
```

and:

```go
errors.As(err, &target)
```

instead of comparing error strings.

Define sentinel errors only when callers genuinely need to distinguish them.

Do not use errors as a substitute for normal control flow when a clearer return value is appropriate.

---

# 12. Panic

Do not use `panic` for normal application errors.

Prefer returning errors.

A panic may be appropriate for genuinely unrecoverable programmer errors or impossible states, but it should be uncommon in normal application code.

---

# 13. Context

Use `context.Context` for:

* cancellation
* deadlines
* request-scoped values

Conventionally place it first:

```go
func DoSomething(ctx context.Context, input Input) error
```

Do not store `context.Context` inside long-lived structs.

Do not use context as a general-purpose dependency container.

Propagate cancellation to operations that support it.

---

# 14. Resource Management

Release resources reliably.

Use `defer` where appropriate:

```go
file, err := os.Open(path)
if err != nil {
	return err
}
defer file.Close()
```

Pay particular attention to:

* files
* HTTP response bodies
* database connections
* transactions
* locks
* timers
* goroutines
* subscriptions

Make cleanup behavior obvious.

---

# 15. Concurrency

Use concurrency only when it provides a real benefit.

Do not create goroutines unnecessarily.

Every goroutine should have a clear:

* owner
* lifecycle
* cancellation strategy
* error-handling strategy

Avoid goroutine leaks.

Use channels when communication between concurrent operations is the natural abstraction.

Use mutexes when protecting shared state is simpler.

Do not use channels merely because they are idiomatic Go.

---

# 16. Generics

Use generics when they provide meaningful type-safe reuse.

Good candidates include:

* generic algorithms
* reusable data structures
* generic utility functions
* APIs where the type parameter represents a real invariant

Go 1.27 supports generic methods.

Use them when they make the API clearer.

Avoid replacing simple code with unnecessarily generic abstractions.

Prefer:

```go
func FindUser(id UserID) (User, error)
```

over introducing generic abstractions that provide no meaningful benefit.

Generics should reduce duplication or improve type safety, not increase abstraction for its own sake.

---

# "log/slog"17. Interfaces

Keep interfaces small.

Define interfaces based on the behavior actually required by the consumer.

Prefer:

```go
type Reader interface {
	Read(p []byte) (int, error)
}
```

over large interfaces containing unrelated operations.

Do not create an interface automatically for every struct.

If there is only one implementation and no meaningful abstraction is required, a concrete type may be preferable.

---

# 18. Standard Library First

Prefer the Go standard library when it provides an adequate solution.

Before adding a dependency, consider:

1. Can the standard library solve the problem?
2. Is the dependency maintained?
3. Does it provide substantial value?
4. What transitive dependencies does it introduce?
5. Is the dependency appropriate for the project?

Avoid dependencies for trivial functionality.

---

# 19. Dependencies

Use Go modules for dependency management.

Use standard commands:

```bash
go get
go mod tidy
go mod download
go mod graph
go mod why
```

After changing dependencies:

```bash
go mod tidy
```

Review changes to `go.mod` and `go.sum`.

Do not manually modify `go.sum`.

Avoid unnecessary dependencies.

---

# 20. Multi-Module Projects

When a project contains multiple independent Go modules, use `go.work`.

The workspace should act as the development workspace for the modules, similar conceptually to a .NET solution containing multiple projects.

Example:

```text
project/
├── go.work
├── module-a/
│   └── go.mod
├── module-b/
│   └── go.mod
└── application/"log/slog"
    └── go.mod
```

Configure:

```go
go 1.27.1

use (
	./module-a
	./module-b
	./application
)
```

When adding a new module:

```bash
go work use ./module-name
```

Keep each module's dependencies in its own `go.mod`.

Do not rely on `go.work` to hide missing dependencies.

`go.work` is for workspace development; `go.mod` remains the authoritative dependency declaration for each module.

---

# 21. Module Independence

A module should remain a valid standalone Go module.

Where appropriate, verify modules independently:

```bash
GOWORK=off go test ./...
```

This helps detect accidental dependencies on the workspace.

Use module boundaries to isolate dependencies when that provides a real benefit.

Do not create a separate module for every package.

---

# 22. go.work Maintenance

When adding or removing modules, update `go.work`.

Prefer:

```bash
go work use ./new-module
```

and:

```bash
go work sync
```

rather than manually maintaining workspace entries when possible.

Keep the workspace clean and representative of the actual development environment.

---

# 23. gopls

`gopls` is installed and available in the development environment.

**Use `gopls extensively.**

Prefer semantic language-server operations over blind text manipulation whenever possible.

Use `gopls` for:

* diagnostics
* symbol lookup
* finding references
* finding implementations
* type information
* navigating definitions
* rename operations
* understanding package relationships
* identifying affected code before API changes
* discovering usages before refactoring

Before changing a public or widely used API:

1. Find references with `gopls`."log/slog"
2. Find implementations.
3. Inspect affected packages.
4. Make the change.
5. Check diagnostics again.
6. Run tests.

Do not blindly modify every textual occurrence of a symbol when a semantic operation is available.

Use `gopls` particularly heavily in multi-module `go.work` workspaces.

---

# 24. Formatting

Go code must be formatted with `gofmt`.

Use:

```bash
gofmt -w .
```

or an equivalent targeted command.

Do not manually format Go code in a style inconsistent with `gofmt`.

---

# 25. Static Analysis

Use Go's standard analysis tools.

At minimum:

```bash
go vet ./...
```

Use additional linters when the project explicitly adopts them.

Do not add a large collection of linters without a reason.

If a linter is configured for the repository, the Makefile should expose it.

---

# 26. go fix

Use `go fix` when appropriate for modernizing Go code.

Before accepting automated changes:

1. Review the changes.
2. Run tests.
3. Run formatting.
4. Run static analysis.

Do not blindly apply automated transformations to unfamiliar code.

---

# 27. Makefile

Every Go project MUST have a `Makefile`.

The Makefile is the standard entry point for common development tasks.

At minimum provide:

```text
make help
make format
make test
make test-race
make vet
make build
make tidy
make check
make clean
```

Additional targets should be added when useful, for example:

```text
make lint
make integration-test
make e2e-test
make benchmark
```

---

# 28. Makefile Design

Make targets should be:

* predictable
* repeatable
* explicit
* safe
* composable

Use `.PHONY` for command targets.

Example:

```makefile
.PHONY: help format test test-race vet build tidy check clean

help:
	@echo "Available targets:"
	@echo "  format      Format Go code"
	@echo "  test        Run tests"
	@echo "  test-race   Run tests with race detector"
	@echo "  vet         Run go vet"
	@echo "  build       Build the project"
	@echo "  tidy        Tidy Go modules"
	@echo "  check       Run validation"
	@echo "  clean       Clean build artifacts"

format:
	gofmt -w .

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...
"log/slog"
build:
	go build ./...

tidy:
	go mod tidy
	go work sync

check: format vet test build

clean:
	go clean
```

Adapt the commands for multi-module repositories.

Do not assume that `go test ./...` from the repository root tests every module when multiple independent modules are present.

---

# 29. Makefile as Developer Interface

For common operations, use the Makefile.

Prefer:

```bash
make test
```

over requiring developers to remember the exact underlying command.

Prefer:

```bash
make build
```

over undocumented build commands.

The underlying Go commands should still remain straightforward and discoverable.

---

# 30. CI Compatibility

Make targets should work in CI.

Avoid targets that depend on:

* a developer's shell aliases
* interactive input
* local editor configuration
* undocumented environment state

CI should be able to run the same commands developers run locally.

A good CI pipeline should be able to invoke:

```bash
make check
```

plus project-specific integration tests.

---

# 31. Build

Use standard Go builds.

Examples:

```bash
go build ./...
```

For applications, provide explicit Makefile targets where useful.

Build failures must propagate to `make`.

Do not hide compiler errors or convert failed commands into successful Make targets.

---

# 32. Code Quality Workflow

For normal development, use:

```text
write test
    ↓
implement
    ↓
gofmt
    ↓
run focused tests
    ↓
run broader tests
    ↓
go vet
    ↓
race test when relevant
    ↓
make check
```

Do not wait until the end of a large feature to discover basic formatting, compilation, or test failures.

---

# 33. Refactoring

Refactor continuously when tests provide sufficient safety.

Prefer small changes.

Examples:

* simplify functions
* improve names
* remove duplication
* reduce unnecessary dependencies
* improve error handling
* simplify APIs
* eliminate dead code

Use `gopls` when performing semantic refactoring.

Do not perform large rewrites without a concrete reason.

---

# 34. Performance

Do not optimize without evidence.

When performance matters, use Go's profiling and benchmarking tools:

```bash
go test -bench=.
go test -benchmem
```

Use CPU and memory profiling when appropriate.

Prefer algorithmic improvements over micro-optimizations.

Keep performance-related tests or benchmarks when they protect an important requirement.

---

# 35. Security

Follow secure coding practices.

Never log:

* passwords
* authentication tokens
* private keys
* credentials"log/slog"
* secrets

Validate untrusted input.

Avoid:

* command injection
* SQL injection
* path traversal
* unsafe deserialization
* insecure randomness
* accidental secret exposure

Use established libraries and the standard library's cryptographic primitives rather than implementing cryptography yourself.

---

# 36. Development Completion Checklist

Before considering a code change complete:

### Code

* Code is idiomatic Go.
* Code is formatted.
* Errors are handled correctly.
* Context is used appropriately.
* Resources are cleaned up.
* No unnecessary abstraction was introduced.
* Generics are used only where beneficial.

### Tests

* New behavior has tests.
* TDD was followed where practical.
* Tests use Testify.
* Tests are deterministic.
* Relevant integration tests exist.
* Race testing is performed when relevant.

### Dependencies

* Dependencies are justified.
* `go.mod` is correct.
* `go.sum` is correct.
* `go mod tidy` succeeds.
* `go.work` is updated when applicable.

### Tooling

* `gopls` diagnostics are clean or understood.
* `go vet` passes.
* Formatting passes.
* Build succeeds.
* Makefile targets work.

### Final validation

Run:

```bash
make check
```

and any additional project-specific test targets.

---

# 37. General Rule

Prefer:

```text
idiomatic Go
+
simple code
+
TDD
+
Testify
+
gofmt
+
gopls
+
go vet
+
Go modules
+
go.work for multi-module development
+
Makefile
+
repeatable development workflows
```

Avoid:

```text
unnecessary abstractions
+
unnecessary generics
+
unnecessary dependencies
+
large interfaces
+
blind text-based refactoring
+
tests added only after implementation
+
manual repetitive development commands
```
