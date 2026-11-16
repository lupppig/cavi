# CAVI reference

How the API, jobs, AI, rendering and media behave. To run CAVI, see [Developing CAVI](development.md).

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
| `POST` | `/api/v1/organisations/{orgID}/projects/{projectID}/media/imports` | Import a video from a YouTube, Twitch or Kick link (the caller confirms they have the rights). |
| `POST` | `/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/complete` | Check the uploaded file and make it available. |
| `GET`, `DELETE` | `/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}` | Get one file (fresh signed URLs) or delete it. |
| `POST` | `/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/process` | Retry processing that failed. |
| `POST` | `/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/framing` | Find the people in a video for auto framing (older files, or a retry). |
| `GET` | `/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/framing/path` | The camera path auto framing follows for a stretch and shape, for previews. |
| `GET`, `POST` | `/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/transcript` | Read a transcript, or request one (sends audio to the organisation's AI provider). |
| `GET`, `POST` | `/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/highlights` | Read a file's candidate clips, or find them in its transcript (sends transcript text to the organisation's AI provider). |
| `GET`, `PUT`, `PATCH`, `DELETE` | `/api/v1/organisations/{orgID}/projects/{projectID}/brief` | Read the project's creator brief; submit one to be read into rules (sends the brief's text to the organisation's AI provider); save a reviewed version; delete it. |
| `GET`, `POST` | `/api/v1/organisations/{orgID}/projects/{projectID}/plans` | Read the project's edit plans, or have the AI propose montages from its transcripts and brief. |
| `GET`, `POST` | `/api/v1/organisations/{orgID}/projects/{projectID}/voiceovers` | List a project's voiceovers, or have a script spoken by an AI voice. |
| `GET`, `DELETE` | `/api/v1/organisations/{orgID}/projects/{projectID}/voiceovers/{voiceoverID}` | Read a voiceover (fresh signed URL) or delete it. |
| `GET` | `/api/v1/voices` | The voices voiceovers can use. |
| `GET`, `POST` | `/api/v1/organisations/{orgID}/projects/{projectID}/renders` | List a project's rendered clips (optionally for one source file), or render one from a composition. |
| `GET`, `DELETE` | `/api/v1/organisations/{orgID}/projects/{projectID}/renders/{renderID}` | Read or delete a rendered clip. |
| `GET` | `/api/v1/organisations/{orgID}/ai-models` | The organisation's AI model for each task, with the supported options. |
| `PUT` | `/api/v1/organisations/{orgID}/ai-models/{task}` | Choose the model for a task (`organisation.manage`). |
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

## Creator briefs

A project can hold the creator's brief for its campaign: paste it, choose the video its timestamps refer to, and AI reads it into a campaign message, guidance and timed rules that people then review and edit.

- Rule kinds: `priority` (moments to feature), `exclude` (must never appear), `blur` (something on screen to blur or crop out), `mute` (audio not to use, such as uncleared music), `caution` (sensitive topics, third-party branding) and `review` (a person must check it). A single timestamp is a rule with the same start and end.
- The model can't invent moments: every rule time must be written in the brief and lie within the video, or the output is rejected and the read retried. Models for the `briefs` task must pass `TestEvalBriefs` (`make ai-eval`).
- Only the brief's text goes to the AI provider, never the media. The provider, model and prompt version are stored with the brief; usage is recorded per organisation.
- Highlights follow the brief: when highlights are requested for the brief's video, its campaign message, guidance and rules are copied onto the run. Each clip must say how it connects to the campaign (`campaign_link`), clips over excluded moments are dropped, and clips featuring a priority moment are listed first.
- Reading needs `project.update` (creators: only on their own projects). Anyone who can see the project can read the brief.

## Montages

A render's composition can be a montage (version 3): segments from one or more files in the project, played one after another and reframed to one shape. Each segment can mute stretches of its audio (e.g. uncleared music) and blur regions of its frame (e.g. licence plates; boxes are fractions of the source frame, so they hold for any output shape). A hook shows text at the top of the opening. Captions come from each file's transcript and are placed on the montage's timeline. A segment can also be a **split screen**: up to two more scenes (`tiles`) play at the same time, stacked top to bottom in equal panels with thin dividers, and only the segment's own sound plays. A montage can end with a **sign-off** (`sign_off: {text, voiceover_id?}`): a call to action such as "Like and subscribe for more!" on a yellow card in the middle, popping in over the last 2.5 s, or, when a ready voiceover says it, over its length plus half a second with the clip's own sound lowered. Captions stop where the card starts, and a `voiceover` must finish before it. The composer turns a sign-off on for every new montage. Every segment and tile is checked against its file's creator brief, when the render is requested and again before it runs. Sources of montages with more than one segment are downloaded to the worker before rendering (streamed, a later segment's idle connection could be dropped and its picture end early), and a render whose video is shorter than the clip fails. Montages render at 30 fps; `make api-test-ffmpeg` checks blur, mute and voiceover mixing with real FFmpeg.

## Edit plans

`POST …/projects/{projectID}/plans` has the AI propose up to ten montages from the transcripts of up to five files in the project and its creator brief. Plans are written for retention (`plans-v3`): each opens cold on its most gripping line (a short teaser from later in the footage if need be, shown again in context), keeps the setup to the minimum, and ends on the payoff; the hook is on-screen text that opens a question the video answers, and the title is written for Shorts and TikTok. Each plan has segments (whole transcript lines from one or more files, in order), a reason and its campaign link, and a voiceover script only when the opening needs context the footage doesn't give. Times come from the transcripts, never the model; plans over excluded moments, of the wrong length, or (with a brief) without a campaign link are dropped. The brief and the files are copied onto the run. When the brief says a person must be featured, plans centre on them and name them in the hook or title. Plans are suggestions: people open one in the montage composer, change it, and render it.

## Voiceovers

A voiceover is a script spoken by an AI voice (`POST …/voiceovers`; voices from `GET /api/v1/voices`). The script goes to the organisation's model for the `voiceover` task; the audio is checked (not empty, silent, cut short or runaway for the script's length) and stored as WAV in object storage. A montage mixes a ready voiceover in with `voiceover: {voiceover_id, start_ms, duck}`, lowering the clip's own sound to `duck` while it plays; it must fit inside the clip. Only the script is sent to the model (text-to-speech models speak everything they are sent), and providers' WAV replies are unwrapped so their headers are never played. Each voiceover's words are timed by transcribing its audio, keeping the script's spelling, so captioned montages caption the voiceover while it plays. Voiceovers are levelled to spoken-word loudness before mixing and the mix is limited. Deleting a voiceover removes its audio; clips already made keep their sound.

## Background jobs

Work that shouldn't run inside a request (imports from links, media processing, framing analysis, transcription, highlights, reading briefs, edit plans, voiceovers and rendering) runs as jobs.

- **Postgres is the source of truth.** Each job is a row in `jobs` with status (`queued`, `running`, `succeeded`, `failed`), attempts, last error, lease and timestamps. The row is written in the same transaction as the change that needs it, so a job can't be lost or created for a change that rolled back.
- **NATS JetStream delivers jobs.** A relay in the API publishes queued rows to a work-queue stream (`CAVI_JOBS`, subjects `cavi.jobs.<kind>`) and republishes any that stay queued too long. Messages carry only IDs (job, organisation, media) and a correlation ID (the request ID), never file contents.
- **Workers claim before running.** A worker claims the job in Postgres under a lease, and only if the message's organisation and resource match the job record. Duplicate deliveries run a job once at a time, forged messages are discarded, and a crashed worker's job is taken over when its lease expires.
- **Failures retry with backoff** (15 s doubling, capped at 5 min) up to the job's limit (5 by default). Failures that retrying can't fix, such as an undecodable file, fail at once. The user-visible resource (the media file) records the outcome.
- **The media worker** (`cmd/media-worker`) is deployed separately from the API because it needs FFmpeg and CPU. Scale it by running more copies; `CAVI_WORKER_CONCURRENCY` sets jobs per copy (default 2). It needs `CAVI_DATABASE_URL`, `CAVI_NATS_URL` and the `CAVI_STORAGE_*` variables. Locally it runs in Docker (`make media-worker`, logs with `make media-worker-logs`), so FFmpeg doesn't need installing.

## AI: transcription and highlights

AI is behind a provider boundary (`internal/ai`): product code asks for a task with a resolved model and gets validated, structured results. Gemini is the only provider so far.

- **Models are chosen per organisation** from a catalogue of models CAVI has validated (`ai.catalogue`). For transcription: Gemini 3.5 Flash-Lite. For highlights, briefs and edit plans: Gemini 3.5 Flash (platform default) and Gemini 3.5 Flash-Lite (lower cost). For voiceovers: ElevenLabs v3 (platform default; expressive, with delivery styles), Gemini 3.8 Flash TTS and 3.8 Flash-Lite TTS. Owners and admins change it on the organisation page; arbitrary model IDs are rejected, and a stored model that leaves the catalogue falls back to the default.
- **The model is fixed when work is requested.** A transcript or highlight run records its provider, model and prompt version (`transcription-v3`, `highlights-v2`, `briefs-v1`, `plans-v2`, `voiceover-v1`); retries use the same model even if the organisation changes its setting, and never switch models silently.
- **Consent:** AI work only happens when someone with `editing.manage` asks for it. The confirmation names the provider and model the audio (transcription) or transcript text (highlights) goes to, and the request is recorded in the audit log with the model.
- **How it runs:** the media worker cuts the audio into 10-minute mono MP3 sections (a section whose transcript is too long for one reply is split in half, down to 2½ minutes), sends each with a JSON schema, and validates the result (times inside the audio, non-empty text, sane language code) before saving. Times are requested as clock strings (`M:SS.ss`) and parsed strictly: with millisecond integers the models sometimes wrote seconds or minutes-and-seconds (`226` for 2:26), which could pass validation with wrong times. A section whose output is invalid is retried on its own up to three times; then the whole attempt fails. Each segment also lists its words with start times (a word ends where the next starts). Word times are clamped to their segment; a segment whose word times don't hold up keeps its text and falls back to estimates, and if more than a fifth of segments do, the output is retried. Word times make transcription output about 4.7 times longer. Invalid output and provider overload (Gemini's 503 "high demand") retry for about 15 minutes; rejected or blocked requests fail at once with a clear message. Files must have audio and be at most 3 hours long.
- **Speaker voices:** transcription also says how each speaker's voice sounds (`woman`, `man` or `unclear`), from the voice alone. Where 10-minute sections disagree it becomes `unclear`. It describes a voice, not who someone is: the transcript shows it as a suggestion, editors with `editing.manage` can correct it (audited, and marked `voice_set_by: person`), and captions can colour each speaker differently.
- **Highlights** are found in a ready transcript. The editor gives an optional objective ("the funniest exchange, for TikTok") and a clip length range (5 s to 10 min). The transcript goes to the model as numbered lines, and the model returns up to 10 clips as ranges of line numbers, with a title and a reason, best first. Times come from the transcript, never from the model, so clips start and end on whole lines of speech. Line numbers that don't exist fail validation (and retry); clips outside the requested length (±5 s) or mostly overlapping a better-ranked clip are dropped. The objective is fenced off in the prompt so it can't change the instructions or output format. One run per file is kept; running again replaces it. If the transcript is redone while a run is waiting, the run fails with a message instead of using stale lines. Highlights are suggestions for the editor to review: they use only the transcript, not the picture or sound.
- **Cost accounting:** every provider call records task, input and output tokens, audio length, model and success in `ai_usage`, attributed to the organisation.
- **Evaluation:** `make ai-eval` runs every catalogue model against synthetic speech fixtures (`internal/ai/testdata`) with the real API: word error rate under 10%, two speakers told apart, the second speaker's start time within a second, and no words invented from a tone. Because clips and captions are cut on transcript times, a timing evaluation transcribes a 151 s two-voice podcast with known line times three times and requires a median start error of at most 1 s and 90% of lines within 2 s on every run (words are aligned by longest common subsequence, so split or merged segments don't matter). Start times are measured with the model's word times, so v3 must keep the median within 0.5 s and 90% of lines within 1 s, and no voice may be labelled the opposite of how it sounds (the fixture's voices are espeak's male and female). Gemini 3.5 Flash-Lite passes every run (median 0.12-0.25 s, voices right each time); Gemini 3.5 Flash is not offered for transcription until it has passed (its quota ran out before it could be run). For highlights, a synthetic podcast transcript checks that a story is found whole (setup to payoff, not starting in small talk), show admin isn't picked, an objective changes the top clip, and an objective trying to override the instructions still yields valid clips. Add a model to the catalogue only after it passes. (Gemini 3.5 Transcribe doesn't support JSON output, Gemini 2.5 Flash isn't available to new accounts, and 3.6/3.8 Flash were overloaded when evaluated.)

Known limits: speaker labels are per 10-minute section, so on longer media "Speaker 1" in one section may be "Speaker 2" in the next; transcripts can't be edited yet.

## Clips and rendering

A clip is described by a **composition** (`internal/render`), a small versioned JSON document stored with each render: the source file, `start_ms`/`end_ms`, an output shape (`aspect`) and `focus`. Compositions are the contract between whatever plans a clip (the clip composer today; the editor and Creative Director later) and the renderer, so the renderer never takes raw FFmpeg arguments. Version 2 adds optional captions; version 1 is still read.

- **Shapes** render at fixed platform sizes: 9:16 at 1080x1920, 1:1 at 1080x1080, 4:5 at 1080x1350, 16:9 at 1920x1080. Smaller sources are scaled up, so the output is always a standard size.
- **Framing:** the renderer takes the largest region of the source with the output's shape and places it along the cropped axis by `focus` (0 left or top, 1 right or bottom). The web preview uses CSS `object-position` with the same value, which follows the same formula, so the preview matches the render.
- **Auto framing** (`framing: "auto"`, composition version 2 or 3) moves that region to follow the people in shot. After processing, every video is analysed (`media.framing`, in the media worker): frames are sampled 10 times a second, shot changes are found from brightness histograms, and every fourth frame (2.5 a second) goes to `cavi-faces`, a small helper running OpenCV's YuNet face detector (MIT licence; built from a minimal static OpenCV in the worker image, `apps/api/faces`). The track of faces and cuts is stored per file (`media_framing`). For each segment the renderer turns it into a camera path (`internal/framing`): each shot keeps one subject, the most prominent face, switching only when another stays 1.4 times bigger for 2 s; the camera holds still while the subject stays within a dead zone, glides (easing, at most one crop width a second) when they leave it, and jumps only at shot changes. Shots with nobody in them sit at the segment's `focus`. The path is applied with a crop whose position is an expression of time. `GET …/framing/path` returns the same path, which the clip editor's preview plays. Analysis runs at roughly 5 times real time (a 44-minute video takes about 9 minutes, one at a time per worker). Requesting auto framing for a file that isn't analysed is a validation error. Limits: it follows faces, not voices, so with several people in shot it keeps to the most prominent face rather than whoever is speaking (judging the talker from mouth movement and sound was tried and was no better than chance on real podcasts; it needs a trained active-speaker model); people seen only from behind or far away aren't followed; split-screen tiles keep their own focus.
- **Rules:** rendering needs `render.execute` (editors and above). The source must be a processed video in the same project, and the clip 1 second to 10 minutes long and inside the file. Anyone who can see the project can watch and download clips.
- **The render job** (`render.clip`, in the media worker) cuts with a frame-accurate seek and re-encodes to H.264/AAC MP4 (`veryfast`, CRF 20, faststart). Before keeping the output it checks it: size, length (within 0.2 s) and audio must match what was asked. Output that fails isn't stored. Sources that are deleted or unreadable fail at once with a message; storage or network errors retry (3 attempts). Render time is recorded on each render (`render_ms`) for cost accounting. Work files are deleted when each job ends.
- **Captions** are burned in when a render asks for them. Options: a look (`bold`: text with an outline; `boxed`: text on a box; `pop`: short-form style, always word by word in capitals with a heavy outline and drop shadow, each word bouncing in as it's said), timing (`lines`, or `words`: up to three words at a time with the spoken word in the highlight colour and enlarged), text, highlight and outline-or-box colours (any `#RRGGBB`; the app offers ten presets and a custom picker), a position (`bottom`, `middle`, `top`), upper case, and a text colour per speaker. If a highlight would match its speaker's colour it falls back to another colour so the spoken word stays visible. The server builds the cues from the source's ready transcript, never from the client: words spoken inside the clip (by their own times; estimated by length only where a segment has none) are wrapped to at most two lines sized to the output (word mode is drawn larger) and timed from the clip's start. The cues are copied into the composition, so re-rendering gives the same captions even if the transcript changes. The worker writes an ASS script with sizes in output pixels (one event per word in word mode) and burns it in with libass and Inter Black; on vertical shapes bottom captions sit 22% up, clear of feed buttons. The web preview uses the same layout and highlight rules. Captions use Inter (Latin, Greek and Cyrillic scripts) and repeat any transcription mistakes; transcripts can't be edited yet.
- **Storage:** outputs live at `organisations/{org}/projects/{project}/renders/{render}/clip.mp4` and download under the clip's name. A clip outlives its source file; deleting a clip, its project or its organisation deletes its file (a database trigger queues it for the sweep).

## Media

Files never pass through the API:

1. The browser asks to start an upload (`media.upload` permission, file name, type and size up to 5 GB).
2. The API records the file as `uploading` and returns a signed `PUT` URL valid for 15 minutes. The signature pins the content type and the download filename.
3. The browser uploads straight to object storage, showing progress.
4. The browser calls `complete`. The API checks the stored file: its size must match, and its first bytes must be a supported format (MP4/MOV/WebM/MKV/AVI video; MP3/M4A/WAV/AAC/FLAC/Ogg audio; PNG/JPEG/WebP images). File names and the browser's claimed type are never trusted. Files that fail are deleted and the user is told why.

**Importing from a link** (`POST …/media/imports`, `media.upload`): paste a link to one YouTube, Twitch or Kick video or clip and confirm you own it or have the creator's permission to clip it (recorded in the audit log). Only those hosts are accepted. The file is `importing` until the media worker (`media.import`) has fetched it with yt-dlp, limited to those sites' video and clip extractors: it reads the details first and refuses playlists, live streams (import them once the recording is up), videos over 4 hours or 5 GB, then downloads at most 1080p, preferring H.264/AAC in MP4, within an hour. The file is checked like an upload, stored, named after the video's title and processed as usual. Reasons a video can't be had (private, removed, members-only, age-restricted, YouTube blocking the server) are shown on the file; network failures retry. YouTube needs a JavaScript runtime, so the worker image includes Node. YouTube often blocks downloads from cloud servers; if it does, download the video yourself and upload it. Importing a video doesn't grant the right to use it: platform terms and the creator's permission still apply.

Access and storage:

- Media follows project visibility: anyone who can see a project can see and download its media, including clients it's shared with. Uploading needs `media.upload`; deleting needs `media.delete`, but people can always delete their own uploads.
- Objects are private. Browsers get signed URLs that expire after an hour; storage refuses unsigned requests. Object keys are `organisations/{org}/projects/{project}/assets/{asset}/original`, which makes ownership explicit but is never used for authorisation.
- Deleting media, a project or an organisation queues the stored objects (a database trigger on `media_assets`), and a sweep in the API deletes them every minute, retrying failures with backoff. Uploads never completed are removed after 24 hours.
- Uploads and deletions are written to `audit_events`, and each file's size is recorded for future usage accounting.

After a file is uploaded, the media worker processes it in the background: it reads the file through a short-lived signed URL with ffprobe (duration, display dimensions after rotation, frame rate, codecs, channels, sample rate) and stores a JPEG thumbnail for video and images. The project page shows "Processing…" and polls until it's done. Files FFmpeg can't decode fail at once with a clear reason; other failures retry, and anyone who can upload can retry a failed file.

Locally, `make db-up` runs SeaweedFS, which speaks the same S3 API as Cloudflare R2. For staging and production, create an R2 bucket and an API token with object read/write, set the `CAVI_STORAGE_*` variables, and run `go run ./cmd/storage-setup` once with credentials that can manage the bucket (or set the bucket's CORS rules in the Cloudflare dashboard: allow `PUT`, `GET` and `HEAD` from the web origin with headers `content-type` and `content-disposition`).
