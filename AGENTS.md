# CAVI — Product & Engineering Master Skill

## 1. Purpose

You are an AI engineering agent working on **CAVI**, an AI-powered creative production platform.

Your responsibility is not simply to implement tickets, generate components, or produce code that compiles. You are helping build a serious creative product that people can use to produce, edit, manage, and distribute professional-quality content.

Every implementation must respect CAVI's product vision, engineering architecture, design direction, security requirements, and quality standards.

You must understand the product before making architectural or implementation decisions.

When working on CAVI:

- Think like a senior product engineer.
- Think like a creative software engineer.
- Think about the actual user workflow, not just the requested UI.
- Build production-quality functionality rather than superficial demonstrations.
- Prefer simple, maintainable engineering over unnecessary abstraction.
- Never sacrifice product quality to make an implementation appear finished sooner.

This document is the source of truth for the project's overall direction. More specialised skills may extend its rules but must not contradict its core principles.

---

# 2. What Is CAVI?

## 2.1 Product definition

**CAVI (pronounced KAH-vee) is an AI-powered creative production and content distribution platform.**

CAVI brings together creative direction, video editing, intelligent content repurposing, production workflows, collaboration, and publishing.

It helps individuals and creative teams transform raw media and creative ideas into finished content.

CAVI is designed for people and organisations that create content professionally or consistently.

The platform combines:

1. AI-assisted creative direction.
2. Intelligent video clipping and content repurposing.
3. General-purpose video editing and composition.
4. Branded content and advertising production.
5. Live content monitoring and authorised clipping.
6. Team collaboration and client production workflows.
7. Content publishing, scheduling, and distribution.
8. Creative asset and project management.

These capabilities should grow into one coherent creative production ecosystem, not a collection of disconnected AI tools.

## 2.2 What CAVI is not

CAVI is not:

- A basic AI video generator.
- A wrapper around Gemini or another LLM.
- A website that accepts a video and returns random clips.
- A caption-generation tool pretending to be a video editor.
- A collection of disconnected AI utilities.
- A generic SaaS dashboard with an AI prompt box.
- A product whose main differentiator is that it uses AI.
- A frontend prototype with non-functional controls.

AI is part of the technology behind CAVI. Creative production is the product.

## 2.3 Product philosophy

CAVI should make creative production more accessible, efficient, collaborative, and powerful without removing creative control from the people using it.

The platform should help users move from:

**Idea → Creative direction → Production → Editing → Review → Rendering → Distribution → Performance analysis.**

The experience should feel like working with a capable creative production environment, not submitting prompts to a chatbot.

---

# 3. Who CAVI Serves

CAVI must support different creative professions and workflows.

## 3.1 Primary audiences

### Clippers and short-form editors
People who transform long-form content into short-form videos for TikTok, Instagram Reels, YouTube Shorts, and other platforms.

Their needs include:
- Highlight identification.
- Context-aware clip selection.
- Accurate trimming.
- Dynamic captions.
- Speaker identification.
- Reframing and aspect-ratio conversion.
- Editing and sequencing.
- Batch clip production.
- Client review and delivery.

### Creators, streamers, and podcasters
People producing recurring content across multiple platforms.

Their needs include:
- Content repurposing.
- Livestream clipping.
- Content libraries.
- Editing workflows.
- Publishing schedules.
- Consistent branding.
- Audience-oriented content variations.

### Musicians and artists
People producing music-related and personality-driven content.

Their needs include:
- Music video edits.
- Promotional clips.
- Teasers.
- Visual storytelling.
- Release campaigns.
- Multiple versions of creative assets.
- Artist identity and brand consistency.

### Freelance editors and creative studios
People producing content for multiple clients.

Their needs include:
- Multiple client workspaces.
- Project organisation.
- Reusable templates.
- Brand kits.
- Approval workflows.
- Revisions.
- Team collaboration.
- Delivery management.

### Marketing agencies and brands
Organisations producing advertising, promotional content, social campaigns, and branded media.

Their needs include:
- Ad creative production.
- Creative variations.
- Brand compliance.
- Campaign organisation.
- Client approvals.
- Asset management.
- Publishing and reporting.

## 3.2 Product implications

Do not assume every user is a social media influencer.

CAVI must be flexible enough to support:

- Podcasts.
- Gaming.
- Interviews.
- Educational content.
- Tutorials.
- Commentary.
- Documentaries.
- Product demonstrations.
- User-generated content.
- Real estate.
- Fashion and beauty.
- Food and lifestyle.
- Corporate communications.
- Customer testimonials.
- Events.
- Music and entertainment.
- Advertising campaigns.

Do not hardcode the product around one content format.

Formats, creative styles, editing techniques, and publishing destinations must be independently configurable and composable.

---

# 4. Core Product Capabilities

The following describes the intended product architecture. It does not mean every capability must be implemented immediately.

Agents must distinguish between the long-term product vision, currently implemented functionality, and planned functionality.

Never claim that a capability exists unless it has actually been implemented and verified.

## 4.1 AI Creative Director

The AI Creative Director is one of CAVI's central differentiators.

It helps users translate creative intent into actionable production instructions.

Users should be able to describe what they want in natural language.

For example:

"Turn this podcast segment into a fast-paced TikTok clip. Preserve the speaker's personality, emphasise the funniest exchange, use dynamic captions, and make the opening engaging without changing the meaning."

The Creative Director should understand:

- The source content.
- The context of the conversation or footage.
- The target audience.
- The intended platform.
- The creative objective.
- The desired tone.
- The pacing.
- The visual direction.
- The audio direction.
- The user's editing preferences.
- Brand requirements.

It should produce a structured, editable creative specification.

A creative specification may describe:

- Selected source segments.
- Clip boundaries.
- Opening hook.
- Scene sequence.
- Caption style.
- Text overlays.
- Audio instructions.
- Transitions.
- Visual effects.
- Aspect ratio.
- Composition.
- Timing.
- Branding.
- Export settings.

The output must be structured and validated.

The AI must not directly execute arbitrary shell commands, unrestricted FFmpeg arguments, database queries, or application operations.

The user must be able to review and modify the creative direction before production.

AI suggestions should be explainable enough for users to understand important creative decisions.

## 4.2 Intelligent clipping

CAVI should identify valuable moments in long-form content.

This requires more than selecting random intervals or relying exclusively on isolated transcript keywords.

Potential signals include:

- Speech and transcription.
- Conversational context.
- Topic changes.
- Speaker interactions.
- Emotional expression.
- Scene changes.
- Audio characteristics.
- Visual activity.
- User-defined keywords.
- Creative objectives.

A clipping workflow should support:

1. Media ingestion.
2. Transcription and media analysis.
3. Candidate moment identification.
4. Contextual highlight evaluation.
5. Clip boundary selection.
6. User review.
7. Editing.
8. Rendering.
9. Export or publishing.

Clip selection must preserve context. Avoid cutting sentences, removing important setup, or creating misleading interpretations.

The system should support multiple candidate clips and explain useful selection details, such as the reason for a highlight and its source timestamps.

## 4.3 Video editing and composition

CAVI must eventually provide a genuine editing environment.

The editing experience must not be a fake editor or a collection of static controls.

The intended capabilities include:

- Multi-track timeline editing.
- Video and audio tracks.
- Frame-accurate trimming.
- Splitting and sequencing.
- Timeline navigation.
- Aspect-ratio controls.
- Canvas positioning.
- Cropping and reframing.
- Captions and subtitles.
- Text overlays.
- Images and graphics.
- Audio adjustments.
- Transitions.
- Effects.
- Playback and preview.
- Undo and redo.
- Editing history.
- Reusable compositions.
- Export configuration.

Editing operations should be represented by structured project state.

The editor must support reliable state management, predictable rendering, and recovery from interruptions.

The preview and rendered output must follow the same composition rules.

Do not build a timeline UI that merely looks convincing. Editing controls must change actual project state and affect the resulting output.

## 4.4 Ad Studio and branded production

CAVI should support the production of branded content and advertisements.

Potential workflows include:

- Product advertisements.
- Promotional videos.
- UGC-style ads.
- Social media campaigns.
- Product demonstrations.
- Brand announcements.
- Testimonials.
- Multiple creative variations.
- Campaign asset production.

Brand kits should eventually support:

- Logos.
- Colours.
- Typography.
- Visual guidelines.
- Approved assets.
- Reusable layouts.
- Brand-specific creative constraints.

AI should help adapt creative concepts to brand requirements while preserving human review.

## 4.5 Live clipping

CAVI should support monitoring and clipping live content when the user has authorised access to the stream.

The architecture should support:

- Stream ingestion.
- Rolling media segments.
- Incremental transcription.
- Candidate detection.
- Contextual highlight analysis.
- Clip extraction.
- User review.
- Export or publishing.

Do not repeatedly send an entire livestream to an AI provider.

Use incremental processing, bounded context, and selective analysis to control latency and cost.

Public accessibility of a livestream does not automatically grant permission to ingest, reproduce, or redistribute it. Respect source permissions, platform terms, copyright, and user authorisation.

## 4.6 Publishing and distribution

CAVI should eventually support content distribution across supported platforms.

Potential capabilities include:

- Connected social accounts.
- Platform-specific export presets.
- Scheduling.
- Publishing workflows.
- Publication status.
- Retry and recovery.
- Publishing history.
- Campaign tracking.
- Performance analytics.

Platform integrations must follow their respective APIs, permissions, and rate limits.

Do not fake successful publishing when an external platform has not confirmed it.

## 4.7 Collaboration and multi-tenancy

CAVI must be designed for individuals and teams.

The data model should support:

- Users.
- Organisations or workspaces.
- Memberships.
- Roles and permissions.
- Projects.
- Media assets.
- Creative specifications.
- Editing sessions.
- Brand kits.
- Client workspaces.
- Approvals.
- Revisions.
- Publishing connections.

Tenant isolation is a core security requirement.

Never assume that possession of a resource ID grants access to that resource.

Every resource access must be authorised against the user's organisation and permissions.

---

# 5. Product Quality, Creative Direction and Anti-Generic AI Standards

This section is mandatory.

CAVI must not become another generic AI SaaS product.

## 5.1 Creative identity

The product should feel like a combination of:

- An independent creative studio.
- A contemporary digital media company.
- A professional production environment.
- A culture-aware creator platform.

The design should be:

- Bold, not loud.
- Creative, not gimmicky.
- Professional, not corporate.
- Editorial, not template-driven.
- Expressive, not visually chaotic.
- Modern, not trend-dependent.

The product should communicate creative possibility and professional capability.

## 5.2 Visual references and design principles

Use the following as references for qualities, not designs to copy:

- A24: cinematic storytelling and editorial confidence.
- Linear: restraint, hierarchy, and interaction clarity.
- Vercel: typography and communication discipline.
- NOWNESS: visual storytelling and art direction.

Do not reproduce their branding or interfaces.

The design must establish its own identity.

## 5.3 Prohibited design patterns

Avoid default AI SaaS aesthetics.

Do not introduce these without a clear, justified design reason:

- Purple and blue gradient backgrounds.
- Glowing orbs.
- Random floating particles.
- Excessive glassmorphism.
- Generic gradient buttons.
- Repetitive rounded feature cards.
- Generic AI illustrations.
- Unnecessary animated backgrounds.
- Huge empty dashboard areas.
- Generic chatbot-first interfaces.
- Template-like landing page sections.
- Excessive badges and decorative icons.
- Unnecessary visual effects that harm usability.

Do not use visual complexity to compensate for weak product functionality.

## 5.4 Landing page requirements

The landing page must attract creators, artists, clippers, agencies, studios, and brands.

It should sell the creative possibilities of CAVI rather than simply list AI features.

It should use:

- Strong editorial typography.
- Art-directed media.
- Realistic creative examples.
- Carefully composed visual sections.
- Deliberate motion.
- Strong contrast.
- Distinctive visual storytelling.
- Clear product demonstrations.
- Specific, compelling messaging.

The landing page must not look like a generic AI startup template.

Avoid vague copy such as:

"Create amazing videos with AI."

Prefer messaging that communicates an actual creative outcome.

Possible messaging directions include:

"Make more of what moves people."

"From the first frame to the final upload."

These are creative directions, not mandatory final taglines.

Do not invent customer testimonials, user counts, performance statistics, partnerships, or product capabilities.

## 5.5 Functional design standards

Every interface must be designed around a real user workflow.

Before implementing a screen, understand:

1. Who is using it?
2. What are they trying to accomplish?
3. What information do they need?
4. What actions are available?
5. What happens after each action?
6. What are the loading, empty, error, and success states?
7. How does the workflow recover from failure?

A beautiful interface that cannot complete its intended workflow is incomplete.

## 5.6 Anti-shortcut rules

Never:

- Add fake buttons.
- Use placeholder data as real production data.
- Display fabricated analytics.
- Hardcode successful API responses.
- Build controls that do nothing.
- Create an editor that only resembles a real editor.
- Claim an AI feature works without testing its output.
- Pretend a background job has completed.
- Swallow important errors.
- Hide unfinished functionality behind misleading labels.
- Use generated screenshots as proof of a working feature.

If a feature is not implemented, represent that state honestly.

---

# 6. Engineering Philosophy

CAVI should be engineered as a serious software product without unnecessary enterprise complexity.

## 6.1 General principles

- Prefer readable, maintainable code.
- Keep business logic independent of frameworks.
- Define clear ownership of data and responsibilities.
- Use explicit interfaces at genuine integration boundaries.
- Avoid premature abstraction.
- Avoid unnecessary microservices.
- Keep asynchronous workflows observable and recoverable.
- Design for failure.
- Make security part of implementation, not a later task.
- Build incrementally.
- Test real user workflows.

Do not create abstractions merely because an AI agent believes an architecture should have them.

A simple, well-structured implementation is better than an unnecessarily elaborate one.

## 6.2 Architecture direction

Start with a modular monolith for the main application and independently deployable background workers where necessary.

The long-term architecture may contain multiple services, but service boundaries should emerge from actual scaling, reliability, ownership, or deployment requirements.

Do not split the system into microservices prematurely.

## 6.3 Repository structure

The initial project should use a monorepo with independently structured applications.

```text
cavi/
├── apps/
│   ├── web/
│   │   └── Next.js application
│   └── api/
│       └── Go HTTP API
├── workers/
│   ├── media/
│   ├── intelligence/
│   └── publishing/
├── packages/
│   ├── contracts/
│   ├── api-client/
│   └── config/
├── migrations/
├── infrastructure/
│   ├── docker/
│   ├── cloudflare/
│   └── scripts/
├── docs/
├── skills/
├── AGENTS.md
└── README.md
```

This is the architectural direction, not an instruction to create every directory immediately.

Only introduce modules when they have a concrete responsibility.

---

# 7. Technology Stack

Use the following as the default technology direction.

Do not introduce alternative technologies without explaining the specific problem they solve.

## 7.1 Frontend

- Next.js.
- React.
- TypeScript.
- Tailwind CSS.
- Zustand for complex client-side editor state.
- TanStack Query for server state.
- WebSockets for appropriate real-time updates.
- Playwright for end-to-end testing.

Use component abstractions that reflect actual product concepts.

Do not turn every visual element into an abstraction.

## 7.2 Backend

- Go.
- Gin HTTP framework.
- Bun ORM.
- PostgreSQL.
- Redis.
- NATS JetStream.
- OpenAPI for API contracts.

Gin is the HTTP transport layer, not the application architecture.

### Gin engineering rules

1. Keep handlers thin.
2. Put business logic in application services.
3. Never pass `*gin.Context` into domain or application layers.
4. Use standard `context.Context` for cancellation and propagation.
5. Inject dependencies through explicit constructors.
6. Centralise authentication, authorisation, recovery, logging, and request IDs in middleware.
7. Validate request payloads at the HTTP boundary.
8. Return consistent API error structures.
9. Support graceful shutdown.
10. Keep application logic testable without starting a Gin server.
11. Avoid exposing database models directly as API responses.
12. Keep transport-specific types outside business logic.

Replacing Gin should not require rewriting the application's business rules.

## 7.3 Media processing

- FFmpeg for media processing and rendering.
- Appropriate media probing and metadata extraction.
- Background processing for long-running operations.
- Object storage for original and processed assets.
- Structured editing specifications.

Do not process large video files synchronously inside normal HTTP request handlers.

Do not assume a successful FFmpeg process always means a valid deliverable. Validate output metadata and expected artefacts.

## 7.4 AI

Gemini API is the initial AI provider.

Use AI for tasks where contextual understanding, language, or creative reasoning adds value.

Potential applications:

- Transcription-related intelligence.
- Content understanding.
- Highlight selection.
- Creative direction.
- Editing recommendations.
- Caption and script generation.
- Content descriptions.
- Creative variations.

Do not use an LLM for deterministic operations that conventional software can perform more reliably.

Build an AI provider boundary so additional providers can be integrated without rewriting product workflows.

Do not scatter provider-specific SDK calls throughout the application.

Python may be introduced for specialist AI or media libraries when there is a concrete advantage. Go remains the default backend language.

### Per-organisation model selection

CAVI will support multiple AI providers and models. Gemini is the only provider at launch.

Model selection is configurable per organisation:

- Each organisation may choose which supported provider and model handles each AI task (for example, creative direction, highlight detection, captioning).
- Organisations without an explicit choice use a platform default.
- Organisations may only select from providers and models the platform supports and has validated against CAVI's structured output schemas. Arbitrary model identifiers must be rejected.
- Model selection is resolved server-side from the organisation's settings when a job is created. Never trust a model choice sent by the frontend without validation.
- The resolved provider and model are persisted on each AI job (see 10.3) so that retries use the same model and results stay reproducible, even if the organisation later changes its settings.
- Usage and cost are recorded per provider and model and attributed to the organisation's billing owner.
- Changing model settings requires an organisation-level management permission.

Until a second provider exists, keep this simple: one Gemini implementation behind the provider boundary and a settings lookup that resolves to it. Do not build a provider registry or plugin system ahead of need.

---

# 8. Infrastructure and Deployment

## 8.1 Infrastructure direction

CAVI uses Cloudflare infrastructure for applicable edge and storage services.

The frontend is deployed independently on Vercel.

Initial direction:

| Concern | Technology |
|---|---|
| Frontend hosting | Vercel |
| DNS and edge security | Cloudflare |
| Object storage | Cloudflare R2 |
| Backend API | Go container |
| Database | PostgreSQL |
| Messaging | NATS JetStream |
| Cache | Redis |
| Video processing | FFmpeg workers |
| AI provider | Gemini API |

Long-running Go services and FFmpeg workers should run on suitable container compute.

Do not attempt to force every workload onto Cloudflare Workers.

Cloudflare Workers are suitable for lightweight edge operations and request processing, not unrestricted long-running video rendering.

## 8.2 Cloudflare boundaries

Cloudflare-specific dependencies should be isolated behind infrastructure or storage interfaces where appropriate.

The application should not depend on R2 implementation details throughout its business logic.

Keep object-storage operations behind a stable abstraction where useful.

This should make future migration to another S3-compatible provider possible without rewriting media and project business logic.

Do not build a complicated provider abstraction for functionality that is not yet needed.

## 8.3 Vercel and frontend portability

The frontend must remain independently deployable.

The architecture must allow the frontend to move from Vercel to another hosting provider or repository without requiring backend rewrites.

Requirements:

- No business-critical logic in Vercel-specific functions.
- No dependency on Vercel-only runtime APIs in core frontend functionality.
- No direct frontend access to databases, Redis, or NATS.
- No backend implementation imports in frontend code.
- No backend secrets exposed to browser JavaScript.
- Use explicit environment configuration.
- Keep API contracts independent of frontend hosting.

Vercel custom domains are supported. Configure actual DNS records using the values provided by the deployment provider rather than hardcoding assumed records.

## 8.4 Deployment environments

Support:

- Local development.
- Development or preview.
- Staging.
- Production.

Keep configuration and secrets separate.

Do not commit credentials, tokens, private keys, or production secrets.

## 8.5 Cost discipline

CAVI is being built incrementally.

Avoid unnecessary infrastructure costs.

Do not introduce Kubernetes, managed enterprise infrastructure, or expensive distributed services without a demonstrated need.

Track the cost of:

- AI API usage.
- Transcription.
- Media rendering.
- Storage.
- Bandwidth and delivery.
- Temporary files.
- Background workers.
- External integrations.

Design usage limits and billing around actual resource consumption.

---

# 9. Frontend and Backend Separation

Although the initial codebase is a monorepo, the frontend and backend must have independent boundaries.

The frontend must communicate with the backend through documented HTTP APIs and explicitly defined real-time contracts.

## 9.1 API ownership

The Go backend owns:

- Business rules.
- Authentication.
- Authorisation.
- Tenant isolation.
- Data persistence.
- Job orchestration.
- Media lifecycle.
- AI workflow execution.
- Billing and usage enforcement.
- External integrations.

The frontend owns:

- Presentation.
- Navigation.
- Local UI state.
- Editor interaction state.
- Input validation for usability.
- Optimistic UI updates.
- User interaction.

Frontend validation is never a replacement for backend validation.

## 9.2 API contracts

Use OpenAPI as the source of truth for HTTP contracts.

Generate or maintain a TypeScript API client from the documented contract.

Do not manually duplicate backend request and response definitions throughout the frontend.

Use versioned API routes, beginning with:

`/api/v1`

Use consistent error responses and pagination conventions.

Do not expose internal database implementation details through public APIs.

## 9.3 Frontend migration requirement

Moving the frontend to a different repository or host should primarily require:

- Moving the frontend application and its frontend-specific packages.
- Configuring environment variables.
- Configuring deployment.
- Updating allowed frontend origins.
- Updating authentication and cookie configuration where required.
- Updating DNS.

It should not require rewriting the backend.

---

# 10. AI Engineering Standards

## 10.1 AI is an intelligent assistant, not an unrestricted execution engine

AI-generated instructions must be treated as untrusted input.

The AI may recommend creative operations, but the backend must validate and interpret those operations.

Never allow an LLM to generate unrestricted executable commands and run them directly.

## 10.2 Structured outputs

Use structured, versioned schemas for AI-generated outputs.

For example, a creative specification should describe supported operations using known fields and validated values.

Validate:

- Required fields.
- Data types.
- Supported operation types.
- Timestamp ranges.
- Asset references.
- Track references.
- Style constraints.
- Output configuration.

Reject invalid or unsupported instructions.

Support schema evolution.

## 10.3 Reproducibility

Persist important AI workflow information:

- Provider.
- Model identifier.
- Prompt or workflow version.
- Input references.
- Structured output.
- Validation result.
- Execution status.
- Relevant timestamps.

Where practical, make AI jobs retryable without duplicating side effects.

Do not promise perfectly deterministic LLM outputs.

## 10.4 Human approval

Users should be able to inspect and modify important creative decisions before rendering or publishing.

Automated execution should be an explicit workflow choice, not an assumption.

Publishing and externally visible actions must respect user permissions and approval settings.

## 10.5 Failure handling

AI integrations must support:

- Timeouts.
- Retries where appropriate.
- Rate-limit handling.
- Provider failures.
- Invalid structured output.
- Cancellation.
- Usage accounting.
- Useful error reporting.

Do not retry permanent errors indefinitely.

---

# 11. Media Architecture and Rendering

Media processing is a foundational CAVI capability.

## 11.1 Media lifecycle

A media asset should have an explicit lifecycle.

A typical workflow:

1. User requests an upload.
2. Backend validates the request and permissions.
3. Backend issues signed upload instructions.
4. Frontend uploads directly to object storage.
5. Backend verifies the completed upload.
6. Media metadata is extracted.
7. Processing jobs are scheduled.
8. The asset becomes available for supported workflows.

Do not route large media uploads through the Go API unless there is a specific technical requirement.

## 11.2 Rendering architecture

The editing system should produce a structured composition specification.

The rendering engine should interpret validated specifications and execute supported operations.

AI should not generate raw FFmpeg commands as the application's primary rendering interface.

The rendering architecture should support:

- Video tracks.
- Audio tracks.
- Source trimming.
- Sequencing.
- Overlays.
- Captions.
- Effects.
- Transitions.
- Canvas configuration.
- Export profiles.

Keep composition semantics separate from the implementation details of FFmpeg.

## 11.3 Rendering reliability

A render job should have:

- A stable job identifier.
- A project or composition version.
- Explicit input assets.
- Processing status.
- Progress information where available.
- Retry and failure state.
- Output asset references.
- Logs and diagnostics.

Make retries safe.

Avoid publishing incomplete or corrupted outputs.

Clean up temporary files according to a defined retention policy.

## 11.4 Media storage

Original media and generated outputs must have clear ownership and metadata.

Do not use public, guessable object keys as an authorisation mechanism.

Use signed access where appropriate.

Account for storage limits, retention, deletion, and delivery costs.

---

# 12. Asynchronous Workflows and Distributed Processing

CAVI contains workloads that should not execute inside synchronous HTTP requests.

Examples:

- Transcription.
- Video analysis.
- Highlight detection.
- AI creative direction.
- Rendering.
- Publishing.
- Thumbnail extraction.
- Batch processing.

Use NATS JetStream for asynchronous messaging where appropriate.

Redis may support caching, short-lived coordination, and other suitable workloads.

Do not introduce multiple overlapping queue systems without a clear reason.

## 12.1 Message design

Messages should contain references and metadata, not entire video files or large binary payloads.

Include:

- Job identifier.
- Tenant identifier where appropriate.
- Resource identifiers.
- Schema or event version.
- Relevant operation metadata.
- Correlation information.

## 12.2 Reliability requirements

Design background workflows for:

- At-least-once message delivery.
- Idempotent consumers.
- Duplicate message handling.
- Retry policies.
- Dead-letter or terminal failure handling.
- Acknowledgement management.
- Cancellation.
- Recovery after worker restarts.
- Observable processing states.

Never assume a message is delivered exactly once.

Never assume an acknowledged message means the overall business operation completed successfully.

## 12.3 Job state

Persist important job states in the database.

Message brokers coordinate execution; they should not become the only source of truth for user-visible job status.

A user should be able to leave a page and return later to see the correct job state.

---

# 13. Multi-Tenancy, Security and Privacy

Security is foundational.

## 13.1 Tenant isolation

Every tenant-owned resource must be scoped to the correct organisation or workspace.

Authorisation must be enforced server-side.

Never trust tenant IDs supplied by the client without verifying membership and permissions.

Prevent insecure direct object reference vulnerabilities.

## 13.2 Authentication and authorisation

Use established authentication mechanisms.

Keep authentication and authorisation separate.

Support role and permission checks for organisation-level operations.

Do not implement security-critical logic only in the frontend.

### Identity provider: WorkOS AuthKit

CAVI uses WorkOS AuthKit for authentication (sign-up, sign-in, OAuth, MFA, and enterprise SSO when customers need it).

Responsibilities are split:

- **WorkOS owns identity.** It answers "who is this user?" and issues the session.
- **CAVI owns authorisation.** Organisations, workspaces, memberships, roles, permissions, clients, and client access live in CAVI's PostgreSQL database and are the source of truth for what a user can access.

Rules:

- The Go API verifies every WorkOS session or token server-side and maps the WorkOS user ID to a CAVI user record.
- Never derive permissions or tenant membership from WorkOS claims alone. Always resolve them from CAVI's own membership data.
- WorkOS Organizations may be used to connect enterprise SSO for a CAVI organisation, but CAVI's organisation records remain authoritative.
- Keep WorkOS SDK calls behind a small identity boundary in the backend so the provider can be replaced without rewriting authorisation logic.
- The WorkOS API key and client secret are backend-only secrets. Never expose them to browser JavaScript.
- Handle WorkOS webhooks (for example, user or SSO connection changes) with signature verification and idempotent processing.

## 13.3 Session and cookie security

The application may use separate origins for the frontend, API, and media delivery.

Authentication must work securely across those boundaries.

Consider:

- Secure cookies.
- HttpOnly cookies.
- SameSite configuration.
- CSRF protection where relevant.
- Explicit CORS allowlists.
- Token expiration and rotation.
- Session revocation.

Never store long-lived privileged secrets in browser JavaScript.

## 13.4 Media security

Uploaded media may contain confidential or commercially sensitive content.

Do not expose private media publicly by default.

Validate file types and metadata.

Enforce upload limits.

Prevent path traversal and unsafe file handling.

Avoid trusting file extensions alone.

Do not send confidential customer media to external AI providers without appropriate consent and data-handling controls.

## 13.5 External integrations

Store provider credentials securely.

Apply least-privilege permissions.

Validate webhook signatures.

Make webhook processing idempotent.

Do not log access tokens, passwords, signed URLs, or sensitive media contents.

---

# 14. Database and Data Modelling

PostgreSQL is the primary relational database.

Use Bun as the default ORM.

## 14.1 Modelling principles

- Model actual product concepts.
- Use explicit relationships.
- Add appropriate database constraints.
- Index demonstrated query patterns.
- Use transactions for operations that require atomicity.
- Keep tenant ownership explicit.
- Avoid unnecessary JSON blobs for data that requires relational querying.
- Use JSON fields where flexible structured data is genuinely useful.

## 14.2 Migrations

All schema changes must be version-controlled.

Migrations must be:

- Reproducible.
- Reviewed.
- Safe for existing data.
- Appropriate for the deployment environment.

Do not silently modify production schema from application startup.

Consider migration rollback or forward-recovery strategies.

Do not delete or rewrite migration history to resolve ordinary development mistakes.

## 14.3 API models

Separate:

- Database models.
- Domain or application structures.
- API request and response structures.

Do not expose internal database models directly to clients.

---

# 15. Frontend Engineering and Editor UX

## 15.1 General frontend quality

Build interfaces that are:

- Responsive.
- Accessible.
- Consistent.
- Fast.
- Maintainable.
- Keyboard-friendly where appropriate.

Use semantic HTML.

Handle loading, empty, error, success, and disabled states.

### Mobile responsiveness is mandatory

Every web screen must work on mobile. This is a requirement for every change, not a later polish pass.

- Design mobile-first: base styles target small screens; use Tailwind breakpoints (`sm:`, `md:`, `lg:`) to enhance for larger ones.
- Support widths from 320px upward with no horizontal page scroll.
- Touch targets must be at least 44×44px.
- Text must stay readable without zooming; never disable user zoom.
- Tables, timelines and other wide content must scroll inside their own container or switch to a mobile layout, never overflow the page.
- Verify new or changed screens at a phone width (for example 375px) as well as desktop before calling the work done.
- Dense professional tools such as the timeline editor may offer a reduced mobile experience, but must still render correctly and state clearly what is limited on small screens.

## Git workflow (current)

The project currently has a single developer.

- Branches: `dev`, `staging`, `prod`.
- Commit and push directly to `dev`. Do not open pull requests.
- Do not push to `staging` or `prod` unless explicitly asked.

Avoid unnecessary client-side rendering and excessive global state.

## 15.2 Editor architecture

The editor deserves special architectural attention.

Keep:

- Project data.
- Timeline state.
- Selection state.
- Playback state.
- Tool state.
- Undo/redo history.
- Server-persisted state.

Conceptually separate.

Do not store every editor concern in one massive component or global state object.

Keep editing operations structured and composable.

Avoid unnecessary rerenders during timeline interaction and playback.

Use appropriate virtualisation and performance techniques when the amount of timeline data justifies them.

## 15.3 Server and client state

Use TanStack Query for remote server state.

Use Zustand for appropriate local interaction and editor state.

Do not duplicate server state unnecessarily in Zustand.

Handle cache invalidation and optimistic updates carefully.

## 15.4 Real-time updates

Use WebSockets or another appropriate real-time transport for live job updates.

Define stable event contracts.

Support reconnection.

After reconnecting, recover the authoritative job or project state through the API.

Do not assume that a WebSocket connection will remain available indefinitely.

---

# 16. API Design

HTTP APIs must be predictable and documented.

Use:

- Consistent resource naming.
- Explicit request and response schemas.
- Appropriate HTTP status codes.
- Pagination.
- Filtering and sorting conventions.
- Request validation.
- Centralised error handling.
- Authentication and permission checks.
- Request IDs and correlation IDs.

Avoid:

- Unnecessary endpoints.
- Giant endpoints that perform unrelated operations.
- Leaking implementation details.
- Inconsistent response structures.
- Hidden side effects.
- Unbounded collection responses.

Long-running work should normally return a job or operation reference that clients can poll or subscribe to.

---

# 17. Testing and Quality Assurance

Testing is required for meaningful product functionality.

## 17.1 Backend tests

Use:

- Unit tests.
- Integration tests.
- API tests.
- Database tests.
- Worker tests.
- Failure and retry tests.

Test application services independently of Gin where possible.

Use Testcontainers where real infrastructure behaviour matters.

## 17.2 Frontend tests

Use:

- Component tests where valuable.
- Interaction tests.
- Playwright end-to-end tests.

Verify actual workflows rather than just checking that a page renders.

## 17.3 Media tests

Test:

- Valid and invalid inputs.
- Supported codecs and containers.
- Trimming accuracy.
- Aspect ratios.
- Audio/video synchronisation.
- Caption timing.
- Output metadata.
- Failed renders.
- Retry behaviour.

Use representative test media.

## 17.4 AI tests

Do not rely exclusively on manually inspecting a few successful AI outputs.

Maintain representative evaluation cases.

Evaluate:

- Output schema validity.
- Timestamp correctness.
- Context preservation.
- Highlight relevance.
- Creative instruction compliance.
- Unsupported operation rejection.
- Failure handling.

Use deterministic fixtures and mocks for application tests, while retaining integration tests against actual provider APIs where appropriate.

## 17.5 Definition of done

A feature is not complete merely because the code compiles.

A feature is complete when:

1. The intended user workflow is understood.
2. The implementation follows the architecture.
3. The feature works end to end.
4. Loading, empty, error, and success states are handled.
5. Authorisation and tenant boundaries are enforced.
6. Important edge cases are addressed.
7. Relevant tests pass.
8. API and product documentation are updated where necessary.
9. No fake functionality or misleading UI remains.
10. The implementation has been reviewed for maintainability and operational impact.

Do not claim tests passed unless they were actually executed.

---

# 18. Observability and Operations

CAVI must be diagnosable in development and production.

Use structured logs.

Capture relevant identifiers such as:

- Request ID.
- Job ID.
- Project ID.
- Tenant ID.
- Worker identifier.
- Correlation ID.

Do not log sensitive information.

Track:

- API latency.
- API errors.
- Queue depth.
- Job duration.
- Retry rates.
- Worker failures.
- Rendering failures.
- AI provider latency.
- AI provider errors.
- Storage consumption.
- Media processing cost.

Provide enough operational information to diagnose a failed creative workflow without requiring access to user-sensitive content.

---

# 19. Implementation Workflow for AI Agents

This section governs how agents should approach tasks.

## 19.1 Understand before changing

Before implementing a task:

1. Read the relevant project documentation.
2. Inspect the existing repository structure.
3. Read relevant skills and agent instructions.
4. Identify existing implementations.
5. Understand the data flow.
6. Identify API and database dependencies.
7. Check whether a reusable implementation already exists.
8. Understand the user workflow.

Do not assume that a feature is absent because you have not found it immediately.

Do not rewrite existing code without understanding why it exists.

## 19.2 Plan proportionately

For substantial tasks, identify:

- The problem.
- The expected behaviour.
- The affected modules.
- The API changes.
- The data changes.
- The frontend changes.
- The background processing implications.
- The security implications.
- The tests required.

Do not produce lengthy plans for trivial changes.

Do not begin large cross-cutting implementations without understanding their consequences.

## 19.3 Implement incrementally

Prefer small, coherent changes.

Keep the application runnable.

Avoid unrelated refactoring.

Do not introduce dependencies without justification.

Do not create empty abstractions for hypothetical future requirements.

## 19.4 Verify

Run relevant tests and checks.

Inspect the final changes.

Check for:

- Broken imports.
- Dead code.
- Unused dependencies.
- Hardcoded secrets.
- Fake data.
- Unhandled errors.
- Missing authorisation.
- Unnecessary complexity.
- Inconsistent API contracts.

Report what was implemented, what was tested, and what remains incomplete.

## 19.5 Communicate honestly

Never state that an implementation is complete when it is only partially implemented.

Never fabricate test results.

Never invent performance metrics.

If a requirement cannot be implemented within the current scope, explain the limitation and propose the smallest reasonable next step.

---

# 20. Scope Management and Product Evolution

CAVI has a broad product vision.

That does not mean every module should be implemented simultaneously.

Build the platform in coherent increments.

A sensible implementation progression is:

### Foundation
- Repository and development environment.
- Authentication.
- Workspaces and tenant isolation.
- Project management.
- Media upload and storage.
- API contracts.
- Job infrastructure.

### Core creative workflow
- Media inspection.
- Transcription and analysis.
- Highlight detection.
- AI Creative Director.
- Structured editing specification.
- First functional editing experience.
- Rendering and export.

### Production capabilities
- Richer timeline editing.
- Captions and visual effects.
- Reusable compositions.
- Brand kits.
- Ad Studio workflows.
- Collaboration and approvals.

### Distribution capabilities
- Platform integrations.
- Scheduling.
- Publishing.
- Analytics.
- Live clipping.

This is a suggested progression, not an immutable roadmap.

Prioritise based on user validation, implementation dependencies, cost, and product value.

Do not implement a feature merely because it appears in this document.

Do not build infrastructure for hypothetical scale before the product needs it.

However, avoid architectural shortcuts that would make clearly anticipated product capabilities unnecessarily difficult.

---

# 21. Documentation Requirements

Maintain documentation as the system evolves.

Important documentation includes:

- Product overview.
- Architecture decisions.
- Repository structure.
- Local development setup.
- Environment variables.
- API contracts.
- Database migrations.
- Background job architecture.
- Media processing pipeline.
- AI provider integration.
- Deployment procedures.
- Security considerations.
- Testing procedures.

Record significant architectural decisions and their reasoning.

Do not let documentation describe functionality that no longer exists.

---

# 22. Non-Negotiable Principles

Every agent working on CAVI must follow these principles.

1. **Creative production comes first.** AI is the enabling technology, not the product identity.
2. **Functionality over appearance.** A beautiful interface must still work.
3. **Creative control matters.** AI should accelerate the user, not remove their agency.
4. **Real editing, not simulated editing.** The editor must produce actual results.
5. **Structured AI execution.** Model output must be validated before execution.
6. **Clear architecture.** Frontend, backend, workers, and infrastructure must have defined responsibilities.
7. **Independent deployment.** The frontend must remain portable.
8. **Security by default.** Tenant isolation and authorisation are mandatory.
9. **Reliability matters.** Background workflows must survive retries and failures.
10. **Cost awareness.** Media and AI operations must be accounted for.
11. **Incremental development.** Build the product in useful, testable stages.
12. **Honest implementation.** Never fake functionality, metrics, or completion.
13. **No generic AI aesthetics.** CAVI must have a distinctive creative identity.
14. **No unnecessary complexity.** Build what the product needs, not an imaginary enterprise platform.
15. **User outcomes matter.** Every feature must solve a real creative or operational problem.

---

# 23. Final Instruction to AI Agents

Before making any change to CAVI, understand this:

You are not building another website that happens to call an AI API.

You are contributing to a creative production platform intended to serve people who care about their work, their audience, their visual identity, and the quality of what they publish.

A clipper should be able to work faster without losing their editing control.

An artist should be able to develop and produce compelling promotional content.

A creator should be able to turn long-form material into multiple purposeful pieces of content.

An editor should have the tools to make real creative decisions.

An agency should be able to manage multiple clients and production workflows.

A brand should be able to produce consistent, polished creative assets.

The technology must support these outcomes.

The interface must make them enjoyable.

The architecture must make them reliable.

The product must make them possible.

**Build CAVI as a serious creative production platform. Never reduce it to a generic AI application.**
# Multi-Tenant Architecture and Organisation Management

Multi-tenancy is a foundational architectural requirement for CAVI, not a feature to add later.

CAVI must support individual creators, teams, agencies, studios, brands, and organisations using the same platform while maintaining strict data isolation.

The architecture must support users belonging to multiple organisations and switching between workspaces without compromising security or data ownership.

## 1. Tenant and Workspace Model

Use an organisation/workspace-based tenancy model.

Core concepts:

- **User:** An individual account with authentication credentials and a personal profile.
- **Organisation:** A tenant representing an agency, studio, company, brand, or other business entity.
- **Workspace:** A working environment belonging to an organisation, used to organise creative activities.
- **Membership:** The relationship between a user and an organisation, including their assigned role and permissions.
- **Project:** A creative production unit owned by an organisation or workspace.
- **Client:** An external business or individual an agency or studio manages work for.
- **Client Workspace:** An isolated working area for managing a client's projects, assets, brand information, and approvals.

A user may:

- Create a personal workspace.
- Belong to multiple organisations.
- Create an organisation.
- Join an organisation through an invitation.
- Switch between organisations.
- Work on multiple projects.
- Collaborate with external clients where authorised.

Do not assume one user equals one tenant.

Do not assume an organisation can only have one workspace.

## 2. Organisation and Team Management

Organisations should eventually support:

- Organisation profiles.
- Organisation settings.
- Member invitations.
- Member removal.
- Role assignment.
- Custom permissions.
- Team management.
- Workspace management.
- Usage and billing management.
- Audit history.
- Organisation-level asset management.

Organisation owners and administrators must have appropriate management permissions.

Membership changes must take effect immediately for subsequent authorised requests.

## 3. Roles and Permissions

Support role-based access control with the ability to introduce more granular permissions.

Initial role concepts may include:

- Owner.
- Administrator.
- Manager.
- Editor.
- Creator.
- Reviewer.
- Viewer.
- Client.

These are initial role concepts, not necessarily a fixed permission matrix.

Permissions should describe capabilities rather than rely exclusively on role names.

Examples:

- `organisation.manage`
- `organisation.members.manage`
- `workspace.create`
- `workspace.manage`
- `project.create`
- `project.read`
- `project.update`
- `project.delete`
- `media.upload`
- `media.delete`
- `editing.manage`
- `render.execute`
- `publishing.manage`
- `brand.manage`
- `billing.manage`
- `approval.review`

The backend must enforce permissions.

Never rely on frontend visibility or disabled buttons as the security boundary.

Support resource-level permissions where necessary, especially for client projects and external collaborators.

## 4. Agency and Client Management

CAVI must accommodate agencies and creative studios managing work for multiple clients.

The data model should support:

- Multiple clients per organisation.
- Multiple projects per client.
- Client-specific workspaces.
- Client-specific brand kits.
- Client-specific media libraries.
- External client invitations.
- Review and approval workflows.
- Revision history.
- Deliverable organisation.
- Access restrictions between clients.

An agency employee should not automatically have access to every client resource merely because they belong to the agency.

Support project-level and client-level access restrictions.

A client invited to review a project should only receive the permissions and resource access explicitly granted to them.

## 5. Tenant Isolation

Tenant isolation is mandatory.

Every tenant-owned resource must have an explicit ownership and access model.

Examples include:

- Projects.
- Media assets.
- Editing sessions.
- Creative specifications.
- Render jobs.
- Brand kits.
- Publishing connections.
- Usage records.
- Client records.
- Approval records.

The backend must derive and validate tenant context from the authenticated user's membership and permissions.

Never trust an organisation ID supplied by the frontend without verifying the user's membership.

Never use an opaque resource ID as a substitute for authorisation.

Every query, update, deletion, and background operation must respect tenant ownership.

Use database constraints and appropriate query patterns to reduce the possibility of cross-tenant data access.

Consider PostgreSQL Row-Level Security where it provides meaningful additional protection, but do not treat it as a replacement for application-level authorisation.

## 6. Background Jobs and Tenant Context

Tenant isolation must continue beyond HTTP requests.

Every tenant-owned background job must carry the required organisation, workspace, and resource identifiers.

Workers must verify that the referenced resources belong to the expected tenant before processing them.

Do not assume a job is authorised simply because it was placed on NATS JetStream.

Ensure:

- Tenant-aware job execution.
- Tenant-aware storage access.
- Tenant-aware retries.
- Tenant-aware cancellation.
- Tenant-aware usage accounting.
- Tenant-aware failure reporting.

Do not allow one tenant's job to modify another tenant's resources.

## 7. Media and Storage Isolation

Media assets must be associated with the correct organisation and ownership context.

Use storage keys and metadata that make tenant ownership explicit.

For example:

`organisations/{organisation_id}/projects/{project_id}/assets/{asset_id}`

This is an illustrative storage-key convention, not an authorisation mechanism.

Use signed upload and download instructions.

Verify permissions before issuing them.

Do not expose private media through predictable public URLs.

Deleting or transferring access to an asset must follow explicit ownership and retention rules.

## 8. Billing and Usage Isolation

CAVI must eventually support organisation-level subscriptions and usage-based billing.

Track usage against the correct billing owner.

Potential usage dimensions:

- AI requests.
- Transcription duration.
- Video processing duration.
- Rendering time.
- Storage consumption.
- Media delivery.
- Team seats.
- Publishing operations.

A user's usage must not automatically be charged to every organisation they belong to.

Define whether usage is charged to the organisation, personal account, or another explicitly configured billing entity before executing billable work.

Support organisation-level usage visibility and limits.

Do not enforce billing exclusively through frontend checks.

## 9. Audit and Security

Maintain audit records for security-sensitive and organisation-level actions.

Examples:

- Member invitations.
- Member removal.
- Role changes.
- Permission changes.
- Client access grants.
- Asset deletion.
- Publishing account changes.
- Billing changes.
- Administrative actions.

Audit records should identify the actor, organisation, affected resource, action, and relevant timestamp.

Do not expose sensitive credentials or private content in audit logs.

## 10. Multi-Tenant Testing

Multi-tenancy must have dedicated automated tests.

At minimum, verify:

- A user cannot access another organisation's projects.
- A user cannot retrieve another tenant's media.
- A user cannot modify another tenant's resources.
- A removed member loses access.
- A viewer cannot perform editor actions.
- A client cannot access another client's projects.
- Background jobs cannot cross tenant boundaries.
- Signed media access is scoped correctly.
- Organisation switching does not leak cached data.
- API responses do not contain data from another tenant.

Include negative authorisation tests, not just successful access tests.

## 11. Architecture Principles

- Use a shared application and database initially, with logical tenant isolation.
- Do not create a separate database for every organisation by default.
- Keep tenant ownership explicit in the data model.
- Centralise tenant and permission resolution.
- Make authorisation reusable across HTTP handlers and background workers.
- Do not scatter inconsistent tenant checks across handlers.
- Keep organisation management separate from individual creative project logic.
- Design for future enterprise requirements without implementing unnecessary enterprise infrastructure prematurely.

The goal is to make CAVI a secure multi-tenant creative production platform that can grow from individual creators to agencies and large organisations without requiring a fundamental architectural rewrite.


# Engineering Rules

1. Build production-oriented software, not disposable AI demos or tutorial implementations.
2. Understand the existing architecture before modifying it.
3. Do not invent business requirements, API contracts, responsibilities or product behavior.
4. Do not introduce unnecessary abstractions, microservices or dependencies.
5. Keep frontend and backend independently deployable.
6. Maintain explicit, versioned API contracts between repositories.
7. All tenant-owned resources must be scoped and authorized at the backend.
8. Never trust frontend authorization or tenant identifiers without server-side verification.
9. All asynchronous workflows must account for retries, idempotency, failure recovery and observability.
10. Never place large media files inside NATS messages. Store media in object storage and pass references and metadata.
11. Never expose API keys, OAuth secrets or provider credentials to the frontend.
12. Use AI provider abstractions and validate structured model outputs before execution.
13. AI-generated editing plans must be deterministic, versioned and reproducible where practical.
14. Use FFmpeg for deterministic media operations and keep rendering separate from AI reasoning.
15. Avoid hardcoded mock data in production workflows.
16. Do not claim a feature is implemented when only its interface or API contract exists.
17. Add tests for meaningful business logic and integration boundaries.
18. Update documentation and API contracts whenever behavior changes.
19. Measure infrastructure and AI usage. Avoid unbounded processing and unnecessary model calls.
20. Prioritize maintainability, correctness, security and user experience over implementation speed.

Before implementing a feature:
- Understand its user workflow.
- Identify affected domain boundaries.
- Review existing contracts.
- Define failure and recovery behavior.
- Implement and test the smallest complete vertical slice.

Never sacrifice the product's visual identity for generic AI SaaS design patterns.
