-- Invitations are matched to a user's verified email.
ALTER TABLE users ADD COLUMN email_verified boolean NOT NULL DEFAULT false;
CREATE INDEX users_email_lower_idx ON users (lower(email));

CREATE TABLE invitations (
    id                 uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id    uuid        NOT NULL REFERENCES organisations (id) ON DELETE CASCADE,
    email_normalized   text        NOT NULL,
    role               text        NOT NULL,
    invited_by_user_id uuid        REFERENCES users (id) ON DELETE SET NULL,
    status             text        NOT NULL DEFAULT 'pending',
    created_at         timestamptz NOT NULL DEFAULT now(),
    expires_at         timestamptz NOT NULL,
    responded_at       timestamptz,

    CONSTRAINT invitations_email_length CHECK (char_length(email_normalized) BETWEEN 3 AND 254),
    CONSTRAINT invitations_role_check CHECK (
        role IN ('owner', 'admin', 'manager', 'editor', 'creator', 'reviewer', 'viewer', 'client')
    ),
    CONSTRAINT invitations_status_check CHECK (
        status IN ('pending', 'accepted', 'declined', 'revoked')
    )
);
-- At most one pending invitation per email per organisation.
CREATE UNIQUE INDEX invitations_pending_unique
    ON invitations (organisation_id, email_normalized) WHERE status = 'pending';
CREATE INDEX invitations_email_pending_idx
    ON invitations (email_normalized) WHERE status = 'pending';

-- Security-sensitive and organisation-level actions. Never store secrets
-- or private content in metadata.
CREATE TABLE audit_events (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid        REFERENCES organisations (id) ON DELETE CASCADE,
    actor_user_id   uuid        REFERENCES users (id) ON DELETE SET NULL,
    action          text        NOT NULL,
    target_type     text,
    target_id       text,
    metadata        jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_organisation_created_idx ON audit_events (organisation_id, created_at DESC);
