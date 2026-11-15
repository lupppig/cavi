-- Voiceover word times, for captioning the voiceover in montages:
-- [{start_ms, end_ms, text}], the script's words timed against its audio.
ALTER TABLE voiceovers ADD COLUMN words jsonb NOT NULL DEFAULT '[]';
