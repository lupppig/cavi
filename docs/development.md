# Developing CAVI

How to run CAVI locally, change its contracts and configure it. For how each part behaves, see [the reference](reference.md).

## Prerequisites

- Go 1.26+
- Node.js 24+ and pnpm 12+
- Docker with Compose

## Local development

```bash
make setup       # install dependencies, create apps/api/.env and apps/web/.env.local
make db-up          # start PostgreSQL (:5432), object storage (:8333) and NATS (:4222) on 127.0.0.1
make migrate-up     # apply database migrations
make storage-setup  # once: create the media bucket and allow browser uploads
make media-worker   # build and start the media worker (FFmpeg) in Docker
make api         # API on http://localhost:8080
make web         # web on http://localhost:3000 (separate terminal)
```

Run `make help` for every target. `make check` runs vet, tests, the contract check, lint and type-checking. `make api-test-integration` also runs tests against the local database, object storage and NATS.

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
| `CAVI_STORAGE_ENDPOINT` | outside dev | | S3 API endpoint for media. Locally `http://localhost:8333`; for R2 `https://<account-id>.r2.cloudflarestorage.com`. Empty disables media (development only). |
| `CAVI_STORAGE_PUBLIC_ENDPOINT` | no | the endpoint | How browsers reach storage, if different from the API. Signed URLs use it. |
| `CAVI_STORAGE_REGION` | no | `auto` | `auto` for R2; `us-east-1` locally. |
| `CAVI_STORAGE_BUCKET` | with storage | | Bucket name. |
| `CAVI_STORAGE_ACCESS_KEY_ID` | with storage | | Storage access key. Secret outside local development. |
| `CAVI_STORAGE_SECRET_ACCESS_KEY` | with storage | | Storage secret key. Secret: never commit (the local SeaweedFS ones in `.env.example` are dev-only). |
| `CAVI_STORAGE_PATH_STYLE` | no | `false` | `true` for SeaweedFS; `false` for R2. |
| `GEMINI_API_KEY` | for AI features | | Gemini API key, used by the media worker. Secret: never commit. Without it, transcription and highlight jobs wait. |
| `CAVI_NATS_URL` | outside dev | | NATS server for background jobs (`nats://` or `tls://`). Without it in development, jobs are recorded but wait. |

### Web (`apps/web/.env.local`)

| Variable | Required | Description |
|---|---|---|
| `NEXT_PUBLIC_CAVI_API_URL` | yes | Public base URL of the Go API, used by the browser. Inlined at build time, so set it per environment before building. If unset, the waitlist form shows that signups are unavailable. |
