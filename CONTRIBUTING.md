# Contributing to MCP Gateway

We welcome contributions to the MCP Gateway project! This document provides guidelines and instructions for contributing.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/your-username/mcp-gateway.git`
3. Add upstream: `git remote add upstream https://github.com/mcp-gateway/mcp-gateway.git`
4. Create a feature branch: `git checkout -b feature/your-feature`

## Development Setup

### Prerequisites

- Go 1.22 or later
- Make

### Building

```bash
go build -o mcp-gateway ./cmd/gateway
```

### Running Tests

```bash
go test ./...
```

## Contribution Guidelines

### Code Style

- Follow Go conventions (use `gofmt`, `golint`)
- Keep functions small and focused
- Use clear, descriptive names
- Add comments for non-obvious logic

### Commit Messages

- Use clear, descriptive commit messages
- Start with a verb (Add, Fix, Implement, Refactor)
- Reference issues when applicable: "Fixes #123"

### Pull Requests

1. Create a descriptive PR title
2. Include a summary of changes
3. Reference any related issues
4. Ensure all tests pass
5. Keep PRs focused (one feature/fix per PR when possible)

## Testing

- Add tests for new features
- Ensure existing tests pass
- Aim for >80% code coverage

## Documentation

- Update README if behavior changes
- Add doc comments to public APIs
- Update architecture docs if design changes

## Community

- Be respectful and constructive
- Help review other contributors' work
- Share ideas and feedback

## Questions?

- Open an issue for questions or clarifications
- Check existing issues first
- Use discussions for non-bug topics

Thank you for contributing!
