# CAVI

CAVI is a creative production and content distribution platform. See [`AGENTS.md`](AGENTS.md) for the product vision, architecture and engineering rules.

## Status

Foundation phase. What exists today:

- `apps/api`: Go HTTP API (Gin, Bun, PostgreSQL) with configuration, structured logging, request IDs, a consistent error envelope, an explicit CORS allowlist, graceful shutdown, health endpoints, the early-access waitlist, Google sign-in through WorkOS (server-side sessions), and organisations: personal and team organisations, workspaces, members, roles and permissions, invitations, an audit log, projects (with client sharing), media uploads to S3-compatible object storage, and background jobs (Postgres + NATS JetStream) that the media worker uses to probe files and make thumbnails with FFmpeg.
- `apps/web`: Next.js landing page with a working waitlist signup, an `/account` page (profile, invitations, organisations, team creation) and an organisation page (projects, rename, members and roles, invitations) and a project page (details, editing, media upload and playback, client sharing, deletion). Product visuals on the landing page are labelled illustrations, not screenshots.
- `packages/contracts`: OpenAPI contract, the source of truth for the HTTP API.
- `packages/api-client`: typed TypeScript client generated from the contract.
- `migrations/`: versioned SQL migrations, applied with `make migrate-up`.
- `infrastructure/docker`: local PostgreSQL, SeaweedFS object storage, NATS JetStream and the media worker image.

Not built yet: invitation emails, workspace management, client workspaces (restricting team members to particular clients), transcription and other analysis, live job updates over WebSockets (the page polls), resumable uploads over 5 GB, storage quotas, AI, the editor, and publishing.

## Repository layout

```text
apps/
  api/                 Go HTTP API
    cmd/api/           API entry point
    cmd/migrate/       Migration command (up, down, status)
    cmd/storage-setup/ Creates the media bucket and its CORS rules
    cmd/media-worker/  Media worker (FFmpeg); Dockerfile.media-worker builds it
    internal/config/   Environment configuration
    internal/database/ PostgreSQL connection (Bun)
    internal/httpapi/  Gin routing, middleware, handlers
    internal/identity/ Identity provider boundary (WorkOS)
    internal/account/  Users, organisations, workspaces, members, invitations
    internal/authz/    Roles and permissions
    internal/project/  Projects and client sharing
    internal/media/    Media uploads, file-type checks, storage cleanup
    internal/storage/  Object storage boundary (S3 API: R2, SeaweedFS)
    internal/jobs/     Background jobs: Postgres outbox, NATS relay, worker loop
    internal/ffmpeg/   FFprobe/FFmpeg runner (probe, frame grabs)
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
| `CAVI_NATS_URL` | outside dev | | NATS server for background jobs (`nats://` or `tls://`). Without it in development, jobs are recorded but wait. |

### Web (`apps/web/.env.local`)

| Variable | Required | Description |
|---|---|---|
| `NEXT_PUBLIC_CAVI_API_URL` | yes | Public base URL of the Go API, used by the browser. Inlined at build time, so set it per environment before building. If unset, the waitlist form shows that signups are unavailable. |

## API endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/health` | Liveness: the process is serving HTTP. |
| `GET` | `/api/v1/health/ready` | Readiness: PostgreSQL is reachable. Returns `503` otherwise. |
| `GET` | `/api/v1/auth/login` | Browser navigation: starts Google sign-in (via WorkOS). Sign-in and sign-up are the same flow. |
| `GET` | `/api/v1/auth/callback` | Browser navigation from WorkOS: creates the session cookie and redirects to `{web}/account` (or `{web}/account?auth_error=…`). |
| `POST` | `/api/v1/auth/logout` | Ends the session; returns `redirect_url` to also end the WorkOS session. Origin must be allowed. |
| `GET` | `/api/v1/me` | The signed-in user and their organisations. `401` when signed out. |
| `POST` | `/api/v1/organisations` | Create a team organisation (caller becomes owner). |
| `GET`, `PATCH` | `/api/v1/organisations/{orgID}` | View or rename an organisation. |
| `GET` | `/api/v1/organisations/{orgID}/members` | List members. |
| `PATCH`, `DELETE` | `/api/v1/organisations/{orgID}/members/{userID}` | Change a role; remove a member or leave. |
| `GET`, `POST` | `/api/v1/organisations/{orgID}/invitations` | List or create invitations. |
| `DELETE` | `/api/v1/organisations/{orgID}/invitations/{invitationID}` | Revoke an invitation. |
| `GET` | `/api/v1/invitations` | The caller's pending invitations. |
| `POST` | `/api/v1/invitations/{invitationID}/accept`, `/decline` | Respond to an invitation. |
| `GET`, `POST` | `/api/v1/organisations/{orgID}/projects` | List projects (newest first, cursor-paged, optional `workspace_id`) or create one. |
| `GET`, `PATCH`, `DELETE` | `/api/v1/organisations/{orgID}/projects/{projectID}` | View, update or delete a project. |
| `GET`, `POST` | `/api/v1/organisations/{orgID}/projects/{projectID}/access` | List or add the clients a project is shared with. |
| `DELETE` | `/api/v1/organisations/{orgID}/projects/{projectID}/access/{userID}` | Stop sharing a project with a client. |
| `GET` | `/api/v1/organisations/{orgID}/projects/{projectID}/media` | List uploaded media with short-lived signed URLs. |
| `POST` | `/api/v1/organisations/{orgID}/projects/{projectID}/media/uploads` | Start an upload; returns a signed instruction for the browser. |
| `POST` | `/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/complete` | Check the uploaded file and make it available. |
| `GET`, `DELETE` | `/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}` | Get one file (fresh signed URLs) or delete it. |
| `POST` | `/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/process` | Retry processing that failed. |
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

- Google is the only sign-in method. The login URL goes straight to Google, and the callback refuses any other method (password, magic link, other OAuth providers), so the AuthKit hosted page can't be used as a side door even if other methods are left enabled in WorkOS.
- Sign-in uses the OAuth authorization-code flow with a one-time `state` cookie checked on the callback.
- Sessions are server-side. The browser gets an opaque `cavi_session` cookie (HttpOnly, SameSite=Lax, Secure outside development); the database stores only its SHA-256 hash. Expired sessions are rejected and swept hourly.
- No WorkOS tokens are stored or sent to the browser. The WorkOS session ID is kept only to end the WorkOS session on sign-out.
- The first sign-in creates the user, a personal organisation, a `Main` workspace and an owner membership in one transaction (safe under concurrent sign-ins).
- Cookie-authenticated state changes require an allowed `Origin` header.
- The frontend and API must be on the same site (for example `app.example.com` and `api.example.com`) so the Lax session cookie is sent. Locally, `localhost:3001` and `localhost:8080` are the same site.

WorkOS dashboard (Applications → your app → Redirects) needs the callback URI `{CAVI_PUBLIC_API_URL}/api/v1/auth/callback`, the sign-in endpoint `{CAVI_PUBLIC_API_URL}/api/v1/auth/login` and the sign-out URI `{CAVI_WEB_URL}/`.

## Organisations, roles and permissions

- Every user has a **personal organisation** (just them) and can create **team organisations**.
- Roles: `owner`, `admin`, `manager`, `editor`, `creator`, `reviewer`, `viewer`, `client`. Each maps to a set of permissions in `internal/authz`; handlers and future workers check permissions, not role names.
- Authorisation happens in the account service for every operation, from the caller's membership in the database. Non-members get `404`, never `403`, so organisation IDs can't be probed.
- Only owners can grant, remove or invite owners. An organisation always keeps at least one owner (enforced with row locks, so concurrent changes can't remove the last one).
- Invitations are matched to email: only a signed-in user whose **verified** email matches can see or accept one. CAVI doesn't send invitation emails yet; the inviter shares the sign-in link.
- Organisation changes, role changes, removals and invitation events are written to `audit_events`.
- Signed-in requests that change data must carry an allowed `Origin`.

## Projects

- A project belongs to an organisation and is filed in one of its workspaces (the first workspace by default). A composite foreign key makes it impossible to file a project in another organisation's workspace.
- Team members with `project.read` see every project in the organisation. **Clients see only projects explicitly shared with them**; everything else is `404` to them, as are projects in organisations the caller doesn't belong to.
- `project.update` lets editors and above change any project; **creators can change only projects they created**. Deleting needs `project.delete` (owner, admin, manager).
- Sharing with clients needs `project.access.manage` (owner, admin, manager), and only members with the `client` role can be granted access. Grants hang off the membership, so removing someone from the organisation also removes their project access.
- Each project response lists the caller's project-level `permissions`, so the UI shows only actions that will succeed. The API enforces them regardless.
- Project creation, updates, deletion and sharing changes are written to `audit_events`.
- Deleting a project is permanent and removes its media, including the stored files (see below).

## Background jobs

Work that shouldn't run inside a request (media processing now; transcription, analysis and rendering later) runs as jobs.

- **Postgres is the source of truth.** Each job is a row in `jobs` with status (`queued`, `running`, `succeeded`, `failed`), attempts, last error, lease and timestamps. The row is written in the same transaction as the change that needs it, so a job can't be lost or created for a change that rolled back.
- **NATS JetStream delivers jobs.** A relay in the API publishes queued rows to a work-queue stream (`CAVI_JOBS`, subjects `cavi.jobs.<kind>`) and republishes any that stay queued too long. Messages carry only IDs (job, organisation, media) and a correlation ID (the request ID), never file contents.
- **Workers claim before running.** A worker claims the job in Postgres under a lease, and only if the message's organisation and resource match the job record. Duplicate deliveries run a job once at a time, forged messages are discarded, and a crashed worker's job is taken over when its lease expires.
- **Failures retry with backoff** (15 s doubling, capped at 5 min) up to the job's limit (5 by default). Failures that retrying can't fix, such as an undecodable file, fail at once. The user-visible resource (the media file) records the outcome.
- **The media worker** (`cmd/media-worker`) is deployed separately from the API because it needs FFmpeg and CPU. Scale it by running more copies; `CAVI_WORKER_CONCURRENCY` sets jobs per copy (default 2). It needs `CAVI_DATABASE_URL`, `CAVI_NATS_URL` and the `CAVI_STORAGE_*` variables. Locally it runs in Docker (`make media-worker`, logs with `make media-worker-logs`), so FFmpeg doesn't need installing.

## Media

Files never pass through the API:

1. The browser asks to start an upload (`media.upload` permission, file name, type and size up to 5 GB).
2. The API records the file as `uploading` and returns a signed `PUT` URL valid for 15 minutes. The signature pins the content type and the download filename.
3. The browser uploads straight to object storage, showing progress.
4. The browser calls `complete`. The API checks the stored file: its size must match, and its first bytes must be a supported format (MP4/MOV/WebM/MKV/AVI video; MP3/M4A/WAV/AAC/FLAC/Ogg audio; PNG/JPEG/WebP images). File names and the browser's claimed type are never trusted. Files that fail are deleted and the user is told why.

Access and storage:

- Media follows project visibility: anyone who can see a project can see and download its media, including clients it's shared with. Uploading needs `media.upload`; deleting needs `media.delete`, but people can always delete their own uploads.
- Objects are private. Browsers get signed URLs that expire after an hour; storage refuses unsigned requests. Object keys are `organisations/{org}/projects/{project}/assets/{asset}/original`, which makes ownership explicit but is never used for authorisation.
- Deleting media, a project or an organisation queues the stored objects (a database trigger on `media_assets`), and a sweep in the API deletes them every minute, retrying failures with backoff. Uploads never completed are removed after 24 hours.
- Uploads and deletions are written to `audit_events`, and each file's size is recorded for future usage accounting.

After a file is uploaded, the media worker processes it in the background: it reads the file through a short-lived signed URL with ffprobe (duration, display dimensions after rotation, frame rate, codecs, channels, sample rate) and stores a JPEG thumbnail for video and images. The project page shows "Processing…" and polls until it's done. Files FFmpeg can't decode fail at once with a clear reason; other failures retry, and anyone who can upload can retry a failed file.

Locally, `make db-up` runs SeaweedFS, which speaks the same S3 API as Cloudflare R2. For staging and production, create an R2 bucket and an API token with object read/write, set the `CAVI_STORAGE_*` variables, and run `go run ./cmd/storage-setup` once with credentials that can manage the bucket (or set the bucket's CORS rules in the Cloudflare dashboard: allow `PUT`, `GET` and `HEAD` from the web origin with headers `content-type` and `content-disposition`).
