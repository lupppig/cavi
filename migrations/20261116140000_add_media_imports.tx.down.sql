DELETE FROM media_assets WHERE status = 'importing';
DELETE FROM jobs WHERE kind = 'media.import';
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights', 'render.clip', 'brief.read', 'voiceover.speak',
                    'plans.create', 'media.framing'));
ALTER TABLE media_assets DROP COLUMN import_error, DROP COLUMN source_url;
ALTER TABLE media_assets DROP CONSTRAINT media_assets_size_positive;
ALTER TABLE media_assets ADD CONSTRAINT media_assets_size_positive CHECK (size_bytes > 0);
ALTER TABLE media_assets DROP CONSTRAINT media_assets_status_check;
ALTER TABLE media_assets ADD CONSTRAINT media_assets_status_check CHECK (status IN ('uploading', 'uploaded'));
