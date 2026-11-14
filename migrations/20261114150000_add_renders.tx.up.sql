-- Renders: finished clips made from a composition of a source file.

-- A render's composition (what to cut from which file, in what shape) is
-- versioned JSON: it will grow (captions, overlays, multiple sources) and
-- is always read whole. The source may be deleted later; the finished clip
-- stays, so source_asset_id is then cleared.
CREATE TABLE renders (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id      uuid        NOT NULL,
    project_id           uuid        NOT NULL,
    source_asset_id      uuid,
    name                 text        NOT NULL,
    composition          jsonb       NOT NULL,
    composition_version  integer     NOT NULL,
    status               text        NOT NULL DEFAULT 'queued',
    error                text,
    output_key           text,
    output_size_bytes    bigint,
    output_duration_ms   bigint,
    output_width         integer,
    output_height        integer,
    -- Wall-clock time spent rendering, for cost accounting.
    render_ms            bigint,
    requested_by_user_id uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    completed_at         timestamptz,

    CONSTRAINT renders_id_organisation_key UNIQUE (id, organisation_id),
    CONSTRAINT renders_output_key_key UNIQUE (output_key),
    CONSTRAINT renders_status_check CHECK (status IN ('queued', 'running', 'ready', 'failed')),
    CONSTRAINT renders_ready_check CHECK (status <> 'ready' OR output_key IS NOT NULL),
    CONSTRAINT renders_project_fkey FOREIGN KEY (project_id, organisation_id)
        REFERENCES projects (id, organisation_id) ON DELETE CASCADE,
    CONSTRAINT renders_source_asset_fkey FOREIGN KEY (source_asset_id, organisation_id)
        REFERENCES media_assets (id, organisation_id) ON DELETE SET NULL (source_asset_id)
);
CREATE INDEX renders_project_created_idx ON renders (project_id, created_at DESC, id DESC);
CREATE INDEX renders_source_asset_idx ON renders (source_asset_id);

-- Delete a render's output with the row, however the row goes.
CREATE FUNCTION queue_render_output_deletion() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.output_key IS NOT NULL THEN
        INSERT INTO storage_deletions (storage_key) VALUES (OLD.output_key);
    END IF;
    RETURN OLD;
END;
$$;

CREATE TRIGGER renders_queue_output_deletion
    AFTER DELETE ON renders
    FOR EACH ROW EXECUTE FUNCTION queue_render_output_deletion();

-- Jobs can now work on a render; its organisation must match the job's.
ALTER TABLE jobs ADD COLUMN render_id uuid;
ALTER TABLE jobs ADD CONSTRAINT jobs_render_fkey FOREIGN KEY (render_id, organisation_id)
    REFERENCES renders (id, organisation_id) ON DELETE CASCADE;
CREATE INDEX jobs_render_id_idx ON jobs (render_id);

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights', 'render.clip'));
