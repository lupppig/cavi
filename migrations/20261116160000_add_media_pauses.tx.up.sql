-- Pauses: stretches of a file with nobody speaking ([[start_ms, end_ms],
-- ...]), found from its loudness when it is processed, so clips can cut
-- them out. NULL until found (files processed before this, or without
-- sound).
ALTER TABLE media_assets ADD COLUMN pauses jsonb;
