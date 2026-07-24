# 0014. Single-node container deploy (Docker)

## Status

Accepted

## Context

v1 is a private, low-cost deployment of a Go/Gin API, React SPA, SQLite, and server-side NIM calls. Operator chose local/single-machine containers over "dev only" or cloud-first.

## Decision

- Ship **Docker** (preferably **Compose**): run the API and serve the built SPA (either from the API static file server or a tiny nginx sidecar — implementation detail).
- **SQLite** file on a **named volume** (or bind mount) so data survives container recreate.
- **Secrets** (NIM API key, session secret) via env / Compose `env_file`, never baked into the image.
- Public VPS is optional later with the same compose file; not required for v1.

## Consequences

- Reproducible run: `docker compose up` as the happy path.
- Backup = copy SQLite volume + know the env file.
- Need a multi-stage Dockerfile (build SPA, build Go, runtime image).
