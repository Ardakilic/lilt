## Context

Lilt already has the Serena MCP scaffolding in place from a previous commit (`98a347f`):

- `compose.yml` defines a `serena` service using the `ghcr.io/oraios/serena:latest` image, bind-mounting the repo at `/workspace/lilt`, and exposing host ports `10122`/`34283`.
- `.serena/project.yml` configures the project as `lilt` with `languages: [go]`.
- `.mcp.json` and `opencode.jsonc` point AI clients at `http://localhost:10122/sse`.
- `docs/serena-mcp.md` documents the integration.

However, the `Makefile` only lists the Serena targets in `help` and `.PHONY`; the recipes themselves are missing, so the commands do not run. Also, the chosen host ports collide with BrewForm’s Serena instance, making it impossible to run both reference projects at the same time.

## Goals / Non-Goals

**Goals:**
- Make `make serena-up` start the Serena container reliably.
- Make `make serena-index` re-index the lilt workspace inside the container.
- Provide `serena-down`, `serena-logs`, and `serena-health` targets with consistent behavior.
- Use host ports that do not conflict with BrewForm’s Serena setup.
- Keep all client configs and docs in sync with the chosen ports and commands.

**Non-Goals:**
- Changing the Go language server backend or `.serena/project.yml` language settings.
- Adding new infrastructure services (postgres, mailpit, etc.) to `compose.yml`.
- Changing container-internal ports (Serena expects 9121 / 24282 inside the container).

## Decisions

1. **Port selection**
   - BrewForm uses `10122` (SSE) and `34283` (dashboard).
   - Lilt will use `10123` (SSE) and `34284` (dashboard) — adjacent, unoccupied ports that keep the internal mapping simple.

2. **Command naming**
   - Rename `serena-stop` to `serena-down` to match BrewForm and the more common Docker Compose idiom (`docker compose down`).

3. **Docker Compose profile**
   - Add `profiles: [serena]` to the `serena` service so it is only started explicitly via `make serena-up` and never accidentally brought up by a plain `docker compose up`.
   - All `docker compose` commands in the `Makefile` will include `--profile serena` for consistency.

4. **Custom Serena image**
   - The stock `ghcr.io/oraios/serena:latest` image does not include Go or `gopls`, which are required for Serena's Go language-server support.
   - Create `Dockerfile.serena` as a multi-stage build that copies the Go toolchain from `golang:1.26.4-trixie` (the same image used by the project's build targets) and then installs `gopls`.
   - Update `compose.yml` to build from `Dockerfile.serena` instead of using the stock image directly.

5. **Makefile recipes**
   - Use the same patterns as BrewForm, adapted for lilt paths and ports:
     - `serena-up`: `docker compose --profile serena up serena -d` (builds automatically on first run)
     - `serena-down`: `docker compose --profile serena down serena`
     - `serena-build`: `docker compose --profile serena build serena` (explicit rebuild when `Dockerfile.serena` changes)
     - `serena-logs`: `docker compose --profile serena logs -f serena`
     - `serena-index`: `docker compose --profile serena exec serena serena project index /workspace/lilt`
     - `serena-health`: HTTP status-code check against `http://localhost:10123/sse` (avoids the SSE stream keeping `curl` open past `--max-time`).

6. **Client configs**
   - Update `.mcp.json` and `opencode.jsonc` to use `http://localhost:10123/sse`.
   - Update `docs/serena-mcp.md` port table, sample commands, and add a note about the custom image.

## Risks / Trade-offs

- [Risk] Changing host ports breaks any local client already configured to `10122`.  
  → Mitigation: Update all project-local configs (`.mcp.json`, `opencode.jsonc`, `docs/serena-mcp.md`) at the same time so the new ports are discoverable.

- [Risk] Adding a Docker Compose profile means plain `docker compose up` no longer starts Serena.  
  → Mitigation: This is the intended behavior; `make serena-up` is the documented entry point.

- [Risk] `gopls` inside Serena may take time to index on first run.  
  → Mitigation: Documented in `docs/serena-mcp.md`; users can monitor with `make serena-logs`.

## Migration Plan

Not applicable for local development tooling. After the change, developers should:

1. Stop any running lilt Serena container with the old config.
2. Run `make serena-up`.
3. Verify with `make serena-health`.
4. Re-index if needed with `make serena-index`.
