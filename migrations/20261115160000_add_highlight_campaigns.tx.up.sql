-- Brief-aware highlights: the creator brief a run used (campaign message,
-- guidance and rules, copied when requested so the run can be traced and
-- retried as asked), and how each clip connects to the campaign.
ALTER TABLE highlight_runs ADD COLUMN campaign jsonb;
ALTER TABLE highlights ADD COLUMN campaign_link text NOT NULL DEFAULT '';
