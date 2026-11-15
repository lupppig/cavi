-- Imports: videos brought in from a link (YouTube, Twitch, Kick) by the
-- media worker instead of uploaded by the browser. A row is 'importing'
-- (with source_url, and size 0 until the file arrives) until the worker
-- has downloaded and checked the file, when it becomes 'uploaded' like
-- any upload. import_error is set when importing gives up.
ALTER TABLE media_assets DROP CONSTRAINT media_assets_status_check;
ALTER TABLE media_assets ADD CONSTRAINT media_assets_status_check CHECK (status IN ('uploading', 'importing', 'uploaded'));
ALTER TABLE media_assets DROP CONSTRAINT media_assets_size_positive;
ALTER TABLE media_assets ADD CONSTRAINT media_assets_size_positive CHECK (size_bytes > 0 OR status = 'importing');
ALTER TABLE media_assets
    ADD COLUMN source_url   text,
    ADD COLUMN import_error text,
    ADD CONSTRAINT media_assets_source_url_length CHECK (char_length(source_url) <= 2048),
    ADD CONSTRAINT media_assets_importing_has_source CHECK (status <> 'importing' OR source_url IS NOT NULL);

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights', 'render.clip', 'brief.read', 'voiceover.speak',
                    'plans.create', 'media.framing', 'media.import'));
