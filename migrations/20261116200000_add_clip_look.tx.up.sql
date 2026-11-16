-- An organisation's look: the house style its clips start from (captions,
-- sign-off, camera, pause cutting, series badges), so a channel's clips
-- look like they belong together. NULL is the platform default.
ALTER TABLE organisations ADD COLUMN clip_look jsonb;
