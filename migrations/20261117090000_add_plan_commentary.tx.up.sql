-- Commentary: each plan's narration lines in the channel's own words,
-- anchored to its segments ([{segment, text}]).
ALTER TABLE edit_plans ADD COLUMN commentary jsonb NOT NULL DEFAULT '[]';
