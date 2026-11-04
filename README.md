# CAVI

CAVI is a creative production and content distribution platform. See [`AGENTS.md`](AGENTS.md) for the product vision, architecture and engineering rules.

## Status

Foundation phase. What exists today:

- `apps/api`: Go HTTP API (Gin, Bun, PostgreSQL) with configuration, structured logging, request IDs, a consistent error envelope, an explicit CORS allowlist, graceful shutdown, and health endpoints.
- `apps/web`: Next.js app with a holding page that reports the API's readiness.
- `infrastructure/docker`: local PostgreSQL.

Not built yet: authentication (WorkOS AuthKit), organisations and tenancy, projects, media, jobs, AI, the editor, and publishing.

## Repository layout

```text
apps/
  api/                 Go HTTP API
    cmd/api/           Entry point
    internal/config/   Environment configuration
    internal/database/ PostgreSQL connection (Bun)
    internal/httpapi/  Gin routing, middleware, handlers
  web/                 Next.js frontend
infrastructure/
  docker/              Local development services
```

## Prerequisites

- Go 1.26+
- Node.js 24+ and pnpm 12+
- Docker with Compose

## Local development

```bash
make setup     # install dependencies, create apps/api/.env and apps/web/.env.local
make db-up     # start PostgreSQL on 127.0.0.1:5432
make api       # API on http://localhost:8080
make web       # web on http://localhost:3000 (separate terminal)
```

Run `make help` for every target. `make check` runs vet, tests, lint and type-checking.

## Environment variables

### API (`apps/api/.env`)

| Variable | Required | Default | Description |
|---|---|---|---|
| `CAVI_ENV` | no | `development` | `development`, `preview`, `staging` or `production`. Development uses text logs; other environments use JSON. |
| `CAVI_HTTP_ADDR` | no | `:8080` | Listen address. |
| `CAVI_DATABASE_URL` | yes | | PostgreSQL connection string. |
| `CAVI_ALLOWED_ORIGINS` | no | empty | Comma-separated frontend origins allowed to make credentialed requests. Wildcards are rejected. |
| `CAVI_SHUTDOWN_TIMEOUT` | no | `15s` | Grace period for in-flight requests on shutdown. |

### Web (`apps/web/.env.local`)

| Variable | Required | Description |
|---|---|---|
| `CAVI_API_URL` | yes | Base URL of the Go API. Server-side only. |

## API endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/health` | Liveness: the process is serving HTTP. |
| `GET` | `/api/v1/health/ready` | Readiness: PostgreSQL is reachable. Returns `503` otherwise. |

Errors use one envelope:

```json
{ "error": { "code": "not_found", "message": "…", "request_id": "…" } }
```

Every response carries an `X-Request-ID` header. A well-formed incoming `X-Request-ID` is kept; otherwise one is generated.
