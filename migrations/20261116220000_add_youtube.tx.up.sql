-- YouTube: an organisation can connect one channel (read-only consent) so
-- CAVI can read its videos' analytics. The refresh token is encrypted
-- (AES-GCM, CAVI_TOKEN_KEY); access tokens are never stored.
CREATE TABLE youtube_connections (
    organisation_id      uuid        PRIMARY KEY REFERENCES organisations (id) ON DELETE CASCADE,
    channel_id           text        NOT NULL,
    channel_title        text        NOT NULL,
    refresh_token        bytea       NOT NULL,
    scopes               text        NOT NULL,
    connected_by_user_id uuid        REFERENCES users (id) ON DELETE SET NULL,
    connected_at         timestamptz NOT NULL DEFAULT now(),
    sync_status          text        NOT NULL DEFAULT 'idle',
    sync_error           text,
    synced_at            timestamptz,

    CONSTRAINT youtube_connections_sync_status_check CHECK (sync_status IN ('idle', 'queued', 'running', 'failed'))
);

-- Consent requests in flight: the state sent to Google (hashed) and who
-- asked, for which organisation. They expire after 10 minutes.
CREATE TABLE youtube_oauth_states (
    state_hash      bytea       PRIMARY KEY,
    organisation_id uuid        NOT NULL REFERENCES organisations (id) ON DELETE CASCADE,
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at      timestamptz NOT NULL
);

-- The channel's uploads and their lifetime numbers, as of the last sync,
-- matched to the CAVI clip they came from where it can be told.
CREATE TABLE youtube_videos (
    organisation_id     uuid             NOT NULL REFERENCES organisations (id) ON DELETE CASCADE,
    video_id            text             NOT NULL,
    title               text             NOT NULL,
    published_at        timestamptz,
    duration_ms         bigint           NOT NULL DEFAULT 0,
    views               bigint           NOT NULL DEFAULT 0,
    watch_minutes       double precision NOT NULL DEFAULT 0,
    avg_view_seconds    double precision NOT NULL DEFAULT 0,
    avg_view_percentage double precision NOT NULL DEFAULT 0,
    subscribers_gained  bigint           NOT NULL DEFAULT 0,
    likes               bigint           NOT NULL DEFAULT 0,
    shares              bigint           NOT NULL DEFAULT 0,
    render_id           uuid             REFERENCES renders (id) ON DELETE SET NULL,
    updated_at          timestamptz      NOT NULL DEFAULT now(),

    PRIMARY KEY (organisation_id, video_id)
);

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights', 'render.clip', 'brief.read', 'voiceover.speak',
                    'plans.create', 'media.framing', 'media.import', 'youtube.sync'));
