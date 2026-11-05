-- Background jobs. Postgres is the source of truth for job state; NATS
-- JetStream only delivers job IDs to workers. A job row is written in the
-- same transaction as the change that needs it (an outbox), and a relay
-- publishes rows whose published_at is null.
ALTER TABLE media_assets ADD CONSTRAINT media_assets_id_organisation_key UNIQUE (id, organisation_id);

CREATE TABLE jobs (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    kind             text        NOT NULL,
    version          integer     NOT NULL DEFAULT 1,
    organisation_id  uuid        NOT NULL REFERENCES organisations (id) ON DELETE CASCADE,
    media_asset_id   uuid,
    status           text        NOT NULL DEFAULT 'queued',
    attempts         integer     NOT NULL DEFAULT 0,
    max_attempts     integer     NOT NULL DEFAULT 5,
    last_error       text,
    correlation_id   text,
    published_at     timestamptz,
    lease_expires_at timestamptz,
    started_at       timestamptz,
    finished_at      timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT jobs_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    CONSTRAINT jobs_kind_check CHECK (kind IN ('media.process')),
    CONSTRAINT jobs_attempts_check CHECK (attempts >= 0 AND max_attempts >= 1),
    -- A job's media must belong to the job's organisation.
    CONSTRAINT jobs_media_asset_fkey FOREIGN KEY (media_asset_id, organisation_id)
        REFERENCES media_assets (id, organisation_id) ON DELETE CASCADE
);
CREATE INDEX jobs_unpublished_idx ON jobs (created_at) WHERE status = 'queued';
CREATE INDEX jobs_media_asset_id_idx ON jobs (media_asset_id);
CREATE INDEX jobs_organisation_created_idx ON jobs (organisation_id, created_at DESC);

-- What processing learned about each file.
ALTER TABLE media_assets
    ADD COLUMN processing_status text NOT NULL DEFAULT 'pending',
    ADD COLUMN processing_error  text,
    ADD COLUMN container         text,
    ADD COLUMN duration_ms       bigint,
    ADD COLUMN width             integer,
    ADD COLUMN height            integer,
    ADD COLUMN frame_rate        numeric(8, 3),
    ADD COLUMN video_codec       text,
    ADD COLUMN audio_codec       text,
    ADD COLUMN audio_channels    integer,
    ADD COLUMN sample_rate       integer,
    ADD COLUMN bit_rate          bigint,
    ADD COLUMN thumbnail_key     text,
    ADD COLUMN processed_at      timestamptz,
    ADD CONSTRAINT media_assets_processing_status_check
        CHECK (processing_status IN ('pending', 'processing', 'ready', 'failed')),
    ADD CONSTRAINT media_assets_thumbnail_key_key UNIQUE (thumbnail_key),
    ADD CONSTRAINT media_assets_dimensions_check CHECK (
        (width IS NULL OR width > 0) AND (height IS NULL OR height > 0) AND (duration_ms IS NULL OR duration_ms >= 0)
    );

-- Queue thumbnails for deletion along with originals, and old thumbnails
-- when a file is reprocessed.
CREATE OR REPLACE FUNCTION queue_media_object_deletion() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        INSERT INTO storage_deletions (storage_key) VALUES (OLD.storage_key);
        IF OLD.thumbnail_key IS NOT NULL THEN
            INSERT INTO storage_deletions (storage_key) VALUES (OLD.thumbnail_key);
        END IF;
    ELSIF OLD.thumbnail_key IS NOT NULL AND OLD.thumbnail_key IS DISTINCT FROM NEW.thumbnail_key THEN
        INSERT INTO storage_deletions (storage_key) VALUES (OLD.thumbnail_key);
    END IF;
    RETURN NULL;
END;
$$;

CREATE TRIGGER media_assets_queue_replaced_thumbnail
    AFTER UPDATE OF thumbnail_key ON media_assets
    FOR EACH ROW EXECUTE FUNCTION queue_media_object_deletion();
