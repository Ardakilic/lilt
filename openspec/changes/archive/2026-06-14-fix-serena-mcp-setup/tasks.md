## 1. Update Docker Compose service

- [x] 1.1 Add `profiles: [serena]` to the `serena` service in `compose.yml`.
- [x] 1.2 Change host port mappings in `compose.yml` from `10122:9121` / `34283:24282` to `10123:9121` / `34284:24282`.
- [x] 1.3 Create `Dockerfile.serena` that adds the Go toolchain and `gopls` to the base Serena image.
- [x] 1.4 Update `compose.yml` to build from `Dockerfile.serena` instead of using the stock image.

## 2. Add Serena recipes to Makefile

- [x] 2.1 Add `serena-up` recipe: `docker compose --profile serena up serena -d`.
- [x] 2.2 Add `serena-down` recipe: `docker compose --profile serena down serena`.
- [x] 2.3 Add `serena-build` recipe: `docker compose --profile serena build serena`.
- [x] 2.4 Add `serena-logs` recipe: `docker compose --profile serena logs -f serena`.
- [x] 2.5 Add `serena-index` recipe: `docker compose --profile serena exec serena serena project index /workspace/lilt`.
- [x] 2.6 Add `serena-health` recipe: HTTP status-code check against `http://localhost:10123/sse`.
- [x] 2.7 Update `help` text to show `serena-down` instead of `serena-stop`.
- [x] 2.8 Update `.PHONY` to reference `serena-down` instead of `serena-stop` and include `serena-build`.

## 3. Update client configurations

- [x] 3.1 Update `.mcp.json` SSE URL from `http://localhost:10122/sse` to `http://localhost:10123/sse`.
- [x] 3.2 Update `opencode.jsonc` SSE URL from `http://localhost:10122/sse` to `http://localhost:10123/sse`.

## 4. Update documentation

- [x] 4.1 Update `docs/serena-mcp.md` port table to `10123` / `34284`.
- [x] 4.2 Update `docs/serena-mcp.md` command table and troubleshooting examples to use `serena-down` and `:10123`.
- [x] 4.3 Correct the outdated claim that BrewForm uses `10121/34282`.

## 5. Verify

- [x] 5.1 Run `make serena-up` and confirm the `lilt-serena` container starts.
- [x] 5.2 Run `make serena-health` and confirm it reports healthy.
- [x] 5.3 Run `make serena-index` and confirm it indexes `/workspace/lilt` without errors.
- [x] 5.4 Run `make serena-down` and confirm the container is removed.
