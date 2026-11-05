-- Media files uploaded to a project. The bytes live in object storage under
-- storage_key; this row is the source of truth for ownership and state.
--
-- Lifecycle: a row is created as 'uploading' when the browser is given a
-- signed upload URL, and becomes 'uploaded' once the API has checked the
-- stored object (size and real file type). Uploads that fail checks are
-- deleted; abandoned ones are swept.
CREATE TABLE media_assets (
    id                    uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id       uuid        NOT NULL,
    project_id            uuid        NOT NULL,
    storage_key           text        NOT NULL,
    filename              text        NOT NULL,
    kind                  text,
    content_type          text        NOT NULL,
    size_bytes            bigint      NOT NULL,
    status                text        NOT NULL DEFAULT 'uploading',
    uploaded_by_user_id   uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    uploaded_at           timestamptz,

    CONSTRAINT media_assets_storage_key_key UNIQUE (storage_key),
    CONSTRAINT media_assets_filename_length CHECK (char_length(filename) BETWEEN 1 AND 255),
    CONSTRAINT media_assets_size_positive CHECK (size_bytes > 0),
    CONSTRAINT media_assets_status_check CHECK (status IN ('uploading', 'uploaded')),
    CONSTRAINT media_assets_kind_check CHECK (kind IN ('video', 'audio', 'image')),
    CONSTRAINT media_assets_uploaded_has_kind CHECK (status <> 'uploaded' OR (kind IS NOT NULL AND uploaded_at IS NOT NULL)),
    CONSTRAINT media_assets_project_fkey FOREIGN KEY (project_id, organisation_id)
        REFERENCES projects (id, organisation_id) ON DELETE CASCADE
);
CREATE INDEX media_assets_project_created_idx ON media_assets (project_id, created_at DESC, id DESC);
CREATE INDEX media_assets_uploading_idx ON media_assets (created_at) WHERE status = 'uploading';

-- Objects waiting to be removed from storage. Rows are added by the trigger
-- below whenever a media row disappears, however that happens (direct
-- delete, project or organisation deletion, rejected or abandoned upload),
-- and a background sweeper deletes the objects and then these rows.
CREATE TABLE storage_deletions (
    id          bigserial   PRIMARY KEY,
    storage_key text        NOT NULL,
    attempts    integer     NOT NULL DEFAULT 0,
    last_error  text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    next_try_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX storage_deletions_next_try_idx ON storage_deletions (next_try_at);

CREATE FUNCTION queue_media_object_deletion() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO storage_deletions (storage_key) VALUES (OLD.storage_key);
    RETURN OLD;
END;
$$;

CREATE TRIGGER media_assets_queue_object_deletion
    AFTER DELETE ON media_assets
    FOR EACH ROW EXECUTE FUNCTION queue_media_object_deletion();
