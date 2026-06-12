# Contributing to ArchitectMCP Runtime

Thanks for your interest in contributing. This is the open-source MCP runtime
that sits between coding agents and a backend licensing/tool service. The
backend business logic lives in a separate private repository.

## Scope

This runtime is intentionally narrow in scope:

- **In scope**: MCP HTTP/SSE transport, API key validation, tool request
  forwarding, session management, credential hashing, configuration loading.
- **Out of scope**: Planning algorithms, billing, persistence, drift detection,
  graph processing, dashboard UIs, or any backend business logic.

Changes that add backend business logic into this repo will not be accepted.
All product logic belongs in the backend service.

## Development Setup

Prerequisites:

- Go 1.25+
- A running backend instance (or a mock)

Clone and build:

```powershell
git clone https://github.com/Riad374-code/architectmcp.git
cd architectmcp
go build ./cmd/architectmcp
```

Run tests:

```powershell
go test ./...
```

## Configuration for Local Development

```powershell
$env:ARCHITECTMCP_BACKEND_URL = "http://localhost:3002"
$env:ARCHITECTMCP_BACKEND_BEARER_TOKEN = "local-development-only"
$env:ARCHITECTMCP_MACHINE_ID = "sha256:dev"
$env:ARCHITECTMCP_ADDR = ":8080"
go run ./cmd/architectmcp
```

The service bearer is optional for loopback backends, but setting it locally
lets contributors test the production authentication flow. Never commit a real
token; use `.env.example` only as a deployment template.

## Making Changes

1. Open an issue first to discuss the change you'd like to make.
2. Fork the repo and create a branch from `main`.
3. Write tests for new functionality.
4. Ensure `go test ./...` passes.
5. Run `go vet ./...` and fix any issues.
6. Keep the diff focused — one change per PR.

## Code Style

- Follow `gofmt` / `go vet` conventions.
- Use existing patterns in the codebase for consistency.
- Avoid adding dependencies unless necessary.
- Do not log API keys, session tokens, or credential hashes.

## PR Checklist

Before submitting:

- [ ] Tests pass (`go test ./...`)
- [ ] No vet warnings (`go vet ./...`)
- [ ] No new dependencies without discussion
- [ ] Commit messages are clear and reference the issue number
- [ ] No secrets, keys, or internal URLs committed

## Reporting Issues

- Bug reports: include the Go version, OS, env var values (redact keys), and
  the full error output.
- Feature requests: explain the use case and why it belongs in this runtime
  rather than the backend service.

## Questions

Open a discussion or issue. Do not email maintainers directly.
