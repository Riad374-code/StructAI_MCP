# ArchitectMCP Runtime

This folder is the MCP-only deployable runtime. It is separated from backend
business logic.

## What This Runtime Does

- Exposes the MCP HTTP/SSE endpoint used by coding agents.
- Accepts the user key through `X-API-Key` or legacy Bearer authorization.
- Validates a user's `sk_mcp_live_...` API key by calling backend
  `POST /v1/license/validate`.
- Stores only a credential fingerprint and backend session JWT in the MCP
  session.
- Forwards `architect_plan` to `POST /v1/mcp/tools/architect-plan`.
- Forwards `architect_check` to `POST /v1/mcp/tools/architect-check`.
- Includes `tool_name`, contract/schema versions, `mcp_version`, and `input`
  in every backend tool request so backend routing is explicit.
- Returns backend's tool output to the coding agent.

## What This Runtime Does Not Do

- It does not run planner business logic locally.
- It does not calculate balance or billing.
- It does not persist specs or dashboard data.
- It does not include the old local planner package.
- It does not include drift checking, graph adaptation, storage, or engine logic.

## Required Backend Endpoints

Backend must provide:

- `POST /v1/license/validate`
- `POST /v1/mcp/tools/architect-plan`
- `POST /v1/mcp/tools/architect-check`

## Configuration

Copy `.env.example` into your deployment platform's secret/configuration
settings. Do not commit the real bearer token.

```powershell
$env:ARCHITECTMCP_BACKEND_URL = "https://api.architectmcp.com"
$env:ARCHITECTMCP_BACKEND_BEARER_TOKEN = "<deployment-secret>"
$env:ARCHITECTMCP_MACHINE_ID = "sha256:<stable-machine-id>"
$env:ARCHITECTMCP_ADDR = ":8080"
go run ./cmd/architectmcp
```

Local development may use:

```powershell
$env:ARCHITECTMCP_BACKEND_URL = "http://localhost:3002"
```

`ARCHITECTMCP_BACKEND_URL` has no default. Startup fails if it is missing, so a
production deployment cannot silently send requests to localhost. Production
backend URLs must use HTTPS; plain HTTP is accepted only for loopback
development.

`ARCHITECTMCP_BACKEND_BEARER_TOKEN` is required for every non-loopback backend.
MCP sends it as `Authorization: Bearer <token>` only to
`POST /v1/license/validate`. The customer key is sent separately as
`X-API-Key`. After validation, backend returns a scoped session JWT, and MCP
uses that JWT as the Bearer credential for tool calls.

There are two independent URLs:

- The public MCP URL, such as `https://mcp.architectmcp.com/mcp/sse`, belongs
  in the user's agent configuration.
- The private product backend base URL, such as
  `https://api.architectmcp.com`, belongs only in the MCP deployment
  environment.

Do not put the backend URL or backend bearer token in the user-facing
onboarding prompt.

## Agent Host Configuration

Backend-generated onboarding prompts should merge this shape into the user's
`mcpServers` object, replacing the example key and public URL:

```json
{
  "architectmcp": {
    "command": "npx",
    "args": [
      "-y",
      "mcp-remote@0.1.38",
      "https://mcp.architectmcp.com/mcp/sse",
      "--header",
      "X-API-Key:${ARCHITECTMCP_API_KEY}"
    ],
    "env": {
      "ARCHITECTMCP_API_KEY": "sk_mcp_live_example"
    }
  }
}
```

`mcp-remote` converts the host's stdio MCP connection to hosted SSE and sends
`X-API-Key` on every MCP HTTP request. The hosted runtime forwards the key to
backend license validation, then uses the returned session JWT for tool calls.
Prompt generation and API-key issuance live in the web/backend app, not in this
standalone MCP runtime.

## Deploy

Build the runtime:

```powershell
go build ./cmd/architectmcp
```

Run tests:

```powershell
go test ./...
```
