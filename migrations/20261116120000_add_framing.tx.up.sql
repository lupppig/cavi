-- Framing analysis: where the faces are in each video and where its shots
-- change, so crops can follow people (auto framing). framing_status is
-- NULL until analysis is requested (files processed before it existed,
-- and anything that isn't a video).
ALTER TABLE media_assets
    ADD COLUMN framing_status text,
    ADD COLUMN framing_error  text,
    ADD CONSTRAINT media_assets_framing_status_check
        CHECK (framing_status IN ('pending', 'processing', 'ready', 'failed'));

-- One track per file, from analyser_version; track is framing.Track.
CREATE TABLE media_framing (
    media_asset_id   uuid        PRIMARY KEY,
    organisation_id  uuid        NOT NULL,
    analyser_version integer     NOT NULL,
    track            jsonb       NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT media_framing_asset_fkey FOREIGN KEY (media_asset_id, organisation_id)
        REFERENCES media_assets (id, organisation_id) ON DELETE CASCADE
);

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights', 'render.clip', 'brief.read', 'voiceover.speak',
                    'plans.create', 'media.framing'));
