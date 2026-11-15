-- Creator briefs: a project's campaign brief, read by AI into timed rules
-- people review. Exclude rules are enforced when clips are made; the other
-- kinds are shown as flags on clips that touch them.

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights', 'render.clip', 'brief.read'));

-- One brief per project. media_asset_id is the video the brief's
-- timestamps refer to; a brief.read job carries it, and a video belongs to
-- one project, so it identifies the brief. The provider, model and prompt
-- version of the last reading are kept so rules can be traced.
CREATE TABLE briefs (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id      uuid        NOT NULL,
    project_id           uuid        NOT NULL,
    media_asset_id       uuid,
    source_text          text        NOT NULL,
    campaign_message     text        NOT NULL DEFAULT '',
    guidance             text        NOT NULL DEFAULT '',
    status               text        NOT NULL DEFAULT 'queued',
    provider             text        NOT NULL,
    model                text        NOT NULL,
    prompt_version       text        NOT NULL,
    error                text,
    requested_by_user_id uuid        REFERENCES users (id) ON DELETE SET NULL,
    updated_by_user_id   uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    completed_at         timestamptz,

    CONSTRAINT briefs_project_key UNIQUE (project_id),
    CONSTRAINT briefs_media_asset_key UNIQUE (media_asset_id),
    CONSTRAINT briefs_id_organisation_key UNIQUE (id, organisation_id),
    CONSTRAINT briefs_status_check CHECK (status IN ('queued', 'running', 'ready', 'failed')),
    CONSTRAINT briefs_source_text_length CHECK (char_length(source_text) BETWEEN 1 AND 20000),
    CONSTRAINT briefs_campaign_length CHECK (char_length(campaign_message) <= 2000 AND char_length(guidance) <= 2000),
    CONSTRAINT briefs_project_fkey FOREIGN KEY (project_id, organisation_id)
        REFERENCES projects (id, organisation_id) ON DELETE CASCADE,
    -- Deleting the video keeps the brief's text; its rules go with it.
    CONSTRAINT briefs_media_asset_fkey FOREIGN KEY (media_asset_id, organisation_id)
        REFERENCES media_assets (id, organisation_id) ON DELETE SET NULL (media_asset_id)
);

-- Timed rules, in time order. A single timestamp has start_ms = end_ms.
CREATE TABLE brief_rules (
    brief_id        uuid    NOT NULL,
    position        integer NOT NULL,
    organisation_id uuid    NOT NULL,
    media_asset_id  uuid    NOT NULL,
    kind            text    NOT NULL,
    start_ms        bigint  NOT NULL,
    end_ms          bigint  NOT NULL,
    note            text    NOT NULL,
    PRIMARY KEY (brief_id, position),
    CONSTRAINT brief_rules_kind_check CHECK (kind IN ('priority', 'exclude', 'blur', 'mute', 'caution', 'review')),
    CONSTRAINT brief_rules_times_check CHECK (start_ms >= 0 AND end_ms >= start_ms),
    CONSTRAINT brief_rules_note_length CHECK (char_length(note) BETWEEN 1 AND 300),
    CONSTRAINT brief_rules_brief_fkey FOREIGN KEY (brief_id, organisation_id)
        REFERENCES briefs (id, organisation_id) ON DELETE CASCADE,
    CONSTRAINT brief_rules_media_asset_fkey FOREIGN KEY (media_asset_id, organisation_id)
        REFERENCES media_assets (id, organisation_id) ON DELETE CASCADE
);
-- Clips are checked against a video's rules.
CREATE INDEX brief_rules_media_asset_idx ON brief_rules (media_asset_id, start_ms);
