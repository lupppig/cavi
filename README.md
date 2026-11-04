# CAVI

CAVI is a creative production and content distribution platform. See [`AGENTS.md`](AGENTS.md) for the product vision, architecture and engineering rules.

## Status

Foundation phase. What exists today:

- `apps/api`: Go HTTP API (Gin, Bun, PostgreSQL) with configuration, structured logging, request IDs, a consistent error envelope, an explicit CORS allowlist, graceful shutdown, health endpoints, the early-access waitlist, and sign-in with WorkOS AuthKit (server-side sessions, users, personal organisations, workspaces and memberships).
- `apps/web`: Next.js landing page with a working waitlist signup, and an `/account` page showing the signed-in user and their organisations. Product visuals on the landing page are labelled illustrations, not screenshots.
- `packages/contracts`: OpenAPI contract, the source of truth for the HTTP API.
- `packages/api-client`: typed TypeScript client generated from the contract.
- `migrations/`: versioned SQL migrations, applied with `make migrate-up`.
- `infrastructure/docker`: local PostgreSQL.

Not built yet: organisation management (invites, roles beyond owner, team organisations), projects, media, jobs, AI, the editor, and publishing.

## Repository layout

```text
apps/
  api/                 Go HTTP API
    cmd/api/           API entry point
    cmd/migrate/       Migration command (up, down, status)
    internal/config/   Environment configuration
    internal/database/ PostgreSQL connection (Bun)
    internal/httpapi/  Gin routing, middleware, handlers
    internal/identity/ Identity provider boundary (WorkOS)
    internal/account/  Users, organisations, workspaces, memberships
    internal/session/  Server-side browser sessions
    internal/waitlist/ Waitlist service and Postgres store
  web/                 Next.js frontend
packages/
  contracts/           OpenAPI spec (source of truth)
  api-client/          Generated TypeScript client
migrations/            Versioned SQL migrations
infrastructure/
  docker/              Local development services
```

## Prerequisites

- Go 1.26+
- Node.js 24+ and pnpm 12+
- Docker with Compose

## Local development

```bash
make setup       # install dependencies, create apps/api/.env and apps/web/.env.local
make db-up       # start PostgreSQL on 127.0.0.1:5432
make migrate-up  # apply database migrations
make api         # API on http://localhost:8080
make web         # web on http://localhost:3000 (separate terminal)
```

Run `make help` for every target. `make check` runs vet, tests, the contract check, lint and type-checking. `make api-test-integration` also runs tests against the local database.

### Changing the API contract

1. Edit `packages/contracts/openapi.yaml`.
2. Run `make contracts` to regenerate `packages/api-client/src/schema.ts`.
3. Implement the change in the Go API. `TestRoutesMatchOpenAPIContract` fails if a route and the spec disagree.

### Database migrations

Migrations are SQL files in `migrations/` named `<timestamp>_<name>.tx.up.sql` and `.tx.down.sql` (`.tx.` runs the file in a transaction). The API never migrates on startup; run `make migrate-up` locally and the migrate command explicitly in deployment. Don't edit a migration that has been applied anywhere shared; add a new one.

## Environment variables

### API (`apps/api/.env`)

| Variable | Required | Default | Description |
|---|---|---|---|
| `CAVI_ENV` | no | `development` | `development`, `preview`, `staging` or `production`. Development uses text logs; other environments use JSON. |
| `CAVI_HTTP_ADDR` | no | `:8080` | Listen address. |
| `CAVI_DATABASE_URL` | yes | | PostgreSQL connection string. |
| `CAVI_ALLOWED_ORIGINS` | no | empty | Comma-separated frontend origins allowed to call the API from a browser. Must include the web app's origin. Wildcards are rejected. |
| `CAVI_TRUSTED_PROXIES` | no | empty | Comma-separated proxy IPs/CIDRs whose `X-Forwarded-For` is trusted for the client IP. Set this when running behind a load balancer or CDN, otherwise rate limits apply to the proxy's IP. |
| `CAVI_SHUTDOWN_TIMEOUT` | no | `15s` | Grace period for in-flight requests on shutdown. |
| `CAVI_PUBLIC_API_URL` | outside dev | `http://localhost:8080` in dev | This API's public base URL; builds the sign-in callback URL. Must be https outside development. |
| `CAVI_WEB_URL` | outside dev | `http://localhost:3000` in dev | Frontend base URL; users land here after signing in or out. Must be https outside development. |
| `WORKOS_API_KEY` | outside dev | | WorkOS secret key. Secret: never commit. |
| `WORKOS_CLIENT_ID` | outside dev | | WorkOS client ID. With both WorkOS values empty, sign-in is disabled (development only). |
| `CAVI_SESSION_TTL` | no | `720h` | Session lifetime (minimum 1h). |

### Web (`apps/web/.env.local`)

| Variable | Required | Description |
|---|---|---|
| `NEXT_PUBLIC_CAVI_API_URL` | yes | Public base URL of the Go API, used by the browser. Inlined at build time, so set it per environment before building. If unset, the waitlist form shows that signups are unavailable. |

## API endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/health` | Liveness: the process is serving HTTP. |
| `GET` | `/api/v1/health/ready` | Readiness: PostgreSQL is reachable. Returns `503` otherwise. |
| `GET` | `/api/v1/auth/login` | Browser navigation: starts sign-in and redirects to WorkOS AuthKit. `?screen=sign-up` opens the sign-up screen. |
| `GET` | `/api/v1/auth/callback` | Browser navigation from WorkOS: creates the session cookie and redirects to `{web}/account` (or `{web}/account?auth_error=…`). |
| `POST` | `/api/v1/auth/logout` | Ends the session; returns `redirect_url` to also end the WorkOS session. Origin must be allowed. |
| `GET` | `/api/v1/me` | The signed-in user and their organisations. `401` when signed out. |
| `POST` | `/api/v1/waitlist` | Join the early-access waitlist. Same `202` response for new and existing addresses. Rate limited to 10 requests per IP per hour. |

The full contract is in `packages/contracts/openapi.yaml`.

The waitlist rate limit is held in API memory: it applies per instance and resets on restart. Move it to Redis before running more than one API instance.

Errors use one envelope:

```json
{ "error": { "code": "not_found", "message": "…", "request_id": "…" } }
```

Every response carries an `X-Request-ID` header. A well-formed incoming `X-Request-ID` is kept; otherwise one is generated.

## Authentication

WorkOS AuthKit verifies who someone is; CAVI's database decides what they can access.

- Sign-in uses the OAuth authorization-code flow with a one-time `state` cookie checked on the callback.
- Sessions are server-side. The browser gets an opaque `cavi_session` cookie (HttpOnly, SameSite=Lax, Secure outside development); the database stores only its SHA-256 hash. Expired sessions are rejected and swept hourly.
- No WorkOS tokens are stored or sent to the browser. The WorkOS session ID is kept only to end the WorkOS session on sign-out.
- The first sign-in creates the user, a personal organisation, a `Main` workspace and an owner membership in one transaction (safe under concurrent sign-ins).
- Cookie-authenticated state changes require an allowed `Origin` header.
- The frontend and API must be on the same site (for example `app.example.com` and `api.example.com`) so the Lax session cookie is sent. Locally, `localhost:3001` and `localhost:8080` are the same site.

WorkOS dashboard (Applications → your app → Redirects) needs the callback URI `{CAVI_PUBLIC_API_URL}/api/v1/auth/callback`, the sign-in endpoint `{CAVI_PUBLIC_API_URL}/api/v1/auth/login` and the sign-out URI `{CAVI_WEB_URL}/`.
