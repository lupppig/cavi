DROP TRIGGER media_assets_queue_replaced_thumbnail ON media_assets;
CREATE OR REPLACE FUNCTION queue_media_object_deletion() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO storage_deletions (storage_key) VALUES (OLD.storage_key);
    RETURN OLD;
END;
$$;
ALTER TABLE media_assets
    DROP CONSTRAINT media_assets_dimensions_check,
    DROP CONSTRAINT media_assets_thumbnail_key_key,
    DROP CONSTRAINT media_assets_processing_status_check,
    DROP COLUMN processed_at,
    DROP COLUMN thumbnail_key,
    DROP COLUMN bit_rate,
    DROP COLUMN sample_rate,
    DROP COLUMN audio_channels,
    DROP COLUMN audio_codec,
    DROP COLUMN video_codec,
    DROP COLUMN frame_rate,
    DROP COLUMN height,
    DROP COLUMN width,
    DROP COLUMN duration_ms,
    DROP COLUMN container,
    DROP COLUMN processing_error,
    DROP COLUMN processing_status;
DROP TABLE jobs;
ALTER TABLE media_assets DROP CONSTRAINT media_assets_id_organisation_key;
