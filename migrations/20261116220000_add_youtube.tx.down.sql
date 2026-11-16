DELETE FROM jobs WHERE kind = 'youtube.sync';
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('media.process', 'media.transcribe', 'media.highlights', 'render.clip', 'brief.read', 'voiceover.speak',
                    'plans.create', 'media.framing', 'media.import'));
DROP TABLE youtube_videos;
DROP TABLE youtube_oauth_states;
DROP TABLE youtube_connections;
