# Serena MCP Integration

Lilt integrates [Serena](https://github.com/oraios/serena) — a semantic code retrieval MCP server — to provide AI coding tools with deep understanding of the Go codebase.

## What Is Serena

Serena gives AI coding tools (Claude Code, OpenCode, VS Code extensions, etc.) IDE-level understanding of your codebase through:

- **Symbol indexing** — Functions, types, interfaces, and variables are indexed
- **Semantic search** — Find code by meaning, not just text
- **Structured editing** — Make precise changes across the codebase

Serena uses **gopls** (the Go language server) as its backend, providing full Go-specific code intelligence.

## Why Lilt Uses Serena

Lilt is a single-module Go CLI project. Serena's gopls integration provides:

- Accurate symbol navigation across `main.go` and `main_test.go`
- Type-aware search (finding all implementations of an interface, etc.)
- Deep understanding of Go-specific patterns (goroutines, channels, interfaces)

## Architecture

```
┌─────────────┐     ┌─────────────────┐     ┌─────────────┐
│  AI Client  │────▶│  Serena Docker  │────▶│    gopls   │
│ (OpenCode, │     │   Container     │     │  Language  │
│ Claude,    │◀────│  (port 10123)   │◀────│  Server    │
│  Cursor)   │     │                 │     │            │
└─────────────┘     └─────────────────┘     └─────────────┘
```

- **Host**: AI client connects to `localhost:10123`
- **Container**: Serena MCP server runs in Docker (port 9121 internal)
- **gopls**: Go language server indexes the mounted workspace

## Docker Image

Lilt uses a custom Serena image (`Dockerfile.serena`) because the stock `ghcr.io/oraios/serena:latest` image does not include the Go toolchain or `gopls`, both of which are required for Serena's Go language-server support. The custom image is built automatically by Docker Compose on first run:

```dockerfile
FROM golang:1.26.2-trixie AS go-toolchain
FROM ghcr.io/oraios/serena:latest

COPY --from=go-toolchain /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"

RUN go install golang.org/x/tools/gopls@latest
ENV PATH="/root/go/bin:${PATH}"
```

This keeps the Go version aligned with the project's build image and avoids host dependencies.

## Volume Mount

The entire project root is bind-mounted:

```yaml
volumes:
  - .:/workspace/lilt
```

This means Serena indexes `main.go`, `main_test.go`, and all Go source files under a single workspace (`/workspace/lilt`).

## Startup Command

```bash
serena start-mcp-server \
  --transport sse \
  --port 9121 \
  --host 0.0.0.0 \
  --context desktop-app \
  --project /workspace/lilt
```

### Flag Rationale

| Flag | Value | Reason |
|------|-------|--------|
| `--transport sse` | SSE | Required for remote/Docker connections; `stdio` only works for local subprocess |
| `--context desktop-app` | `desktop-app` | Broadest tool set, compatible with all MCP clients. Narrower contexts like `claude-code` remove tools other clients need |
| `--project /workspace/lilt` | Explicit container path | Cannot use `--project-from-cwd` — the container's CWD is `/workspaces/serena`, not the mounted project |

## Port Mapping

| Endpoint | Container Port | Host Port | Purpose |
|----------|-----------------|-----------|----------|
| SSE | 9121 | 10123 | MCP client connections |
| Dashboard | 24282 | 34284 | Serena web UI for inspection |

Non-standard ports are used to avoid conflicts with other projects (e.g., BrewForm uses 10122/34283).

Access the dashboard at http://localhost:34284

## What Is Ignored and Why

### `.gitignore` + `ignored_paths`

The following are excluded from indexing:

| Path | Reason |
|------|--------|
| `dist` | Cross-platform build output directory (see `Makefile` `build-all`) |
| `build` | Alternative build directory |
| `vendor` | Vendored Go dependencies — would pollute symbol search with third-party internals |
| `.cache` | Generic cache directory |

`.gitignore` already covers: `*.exe`, `*.dll`, `*.so`, `*.dylib`, `lilt` binary, `*.test`, `*.out`, `coverage.*`, `dist/`, `build/`.

## Project Configuration

`.serena/project.yml` contains the Serena configuration:

```yaml
project_name: "lilt"
languages:
  - go
encoding: "utf-8"
ignore_all_files_in_gitignore: true
ls_specific_settings: {}
ignored_paths:
  - "dist"
  - "build"
  - "vendor"
  - ".cache"
```

### Key Settings

- **`languages: ["go"]`** — Serena uses gopls as the language server
- **`ignore_all_files_in_gitignore: true`** — Respects `.gitignore` for exclusions
- **`ignored_paths`** — Additional paths specific to Go builds

## Go Language Server (gopls) Notes

Serena uses `gopls` (Go language server) for indexing. By default, it uses sensible settings for most projects.

### Optional: Custom gopls Settings

If needed, you can add `ls_specific_settings` in `.serena/project.yml` for build tags or CGO:

```yaml
ls_specific_settings:
  build_tags:
    - customtag
  cgo: true
```

Most projects don't need these customizations.

## Connecting AI Clients

### OpenCode

`opencode.jsonc` is pre-configured:

```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "serena": {
      "type": "remote",
      "url": "http://localhost:10123/sse",
      "enabled": true
    }
  }
}
```

### Claude Code

```bash
claude mcp add serena --transport sse --url http://localhost:10123/sse
```

### VS Code / Cursor / Windsurf

Create `.vscode/mcp.json`:

```json
{
  "mcpServers": {
    "serena": {
      "type": "sse",
      "url": "http://localhost:10123/sse"
    }
  }
}
```

## Makefile Commands

| Command | Description |
|---------|-------------|
| `make serena-up` | Start Serena MCP service |
| `make serena-down` | Stop Serena MCP service |
| `make serena-build` | Build/rebuild Serena Docker image |
| `make serena-logs` | View Serena logs |
| `make serena-index` | Index the project workspace |
| `make serena-health` | Health check the project workspace |

## Troubleshooting

### Project Not Found

If clients report "project not found", ensure the container is running and the project path matches:

```bash
make serena-up
docker compose --profile serena exec serena serena project index /workspace/lilt
```

### Slow Start

gopls takes time to index on first run. Check logs:

```bash
make serena-logs
```

Wait for "Indexing complete" message.

### Stale Index

If search results seem wrong, re-index:

```bash
make serena-index
```

### Dashboard Unreachable

Ensure the dashboard port (34284) is not in use by another application:

```bash
lsof -i :34284
```
