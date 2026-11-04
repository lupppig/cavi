DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS invitations;
DROP INDEX IF EXISTS users_email_lower_idx;
ALTER TABLE users DROP COLUMN IF EXISTS email_verified;
