## Why

Lilt's Serena MCP integration was scaffolded but never wired up: the `Makefile` lists `serena-up`, `serena-index`, and related targets in `.PHONY` and `help`, but the actual recipes are missing, so `make serena-up` fails with "No rule to make target". Additionally, lilt currently uses the same host ports as the BrewForm reference project (10122 / 34283), preventing both projects' Serena containers from running side-by-side on the same machine.

## What Changes

- Add missing `Makefile` recipes for Serena lifecycle commands (`serena-up`, `serena-down`, `serena-logs`, `serena-index`, `serena-health`).
- Rename `serena-stop` → `serena-down` to align with the BrewForm reference convention.
- Change lilt's exposed Serena host ports to avoid collisions with BrewForm:
  - SSE: `10122` → `10123`
  - Dashboard: `34283` → `34284`
- Add the `serena` Docker Compose profile so the service is only started on demand.
- Update `.mcp.json`, `opencode.jsonc`, and `docs/serena-mcp.md` to reflect the new ports and `serena-down` naming.

## Capabilities

### New Capabilities

None. This change fixes the implementation of an existing capability.

### Modified Capabilities

None. No behavior-level requirements are changing; only port numbers and command wiring.

## Impact

- `Makefile`, `compose.yml`, `.mcp.json`, `opencode.jsonc`, `docs/serena-mcp.md`
- Local developer workflow: `make serena-up` and `make serena-index` will work as expected.
- Running BrewForm and lilt Serena containers simultaneously will no longer conflict on localhost.
