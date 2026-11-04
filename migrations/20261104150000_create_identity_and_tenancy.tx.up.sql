-- People who can sign in. Identity is verified by WorkOS; this table is
-- CAVI's own record and the anchor for all tenancy and authorisation.
CREATE TABLE users (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    workos_user_id  text        NOT NULL,
    email           text        NOT NULL,
    first_name      text,
    last_name       text,
    avatar_url      text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    last_sign_in_at timestamptz,

    CONSTRAINT users_workos_user_id_key UNIQUE (workos_user_id),
    CONSTRAINT users_email_length CHECK (char_length(email) BETWEEN 3 AND 254)
);

-- Tenants. A personal organisation belongs to exactly one user and is
-- created on first sign-in; team organisations have no personal owner.
CREATE TABLE organisations (
    id                     uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name                   text        NOT NULL,
    personal_owner_user_id uuid        REFERENCES users (id) ON DELETE CASCADE,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT organisations_name_length CHECK (char_length(name) BETWEEN 1 AND 120),
    CONSTRAINT organisations_personal_owner_key UNIQUE (personal_owner_user_id)
);

CREATE TABLE workspaces (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid        NOT NULL REFERENCES organisations (id) ON DELETE CASCADE,
    name            text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT workspaces_name_length CHECK (char_length(name) BETWEEN 1 AND 120)
);
CREATE INDEX workspaces_organisation_id_idx ON workspaces (organisation_id);

-- A user's role in an organisation. Roles map to permissions in code.
CREATE TABLE memberships (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid        NOT NULL REFERENCES organisations (id) ON DELETE CASCADE,
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role            text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT memberships_organisation_user_key UNIQUE (organisation_id, user_id),
    CONSTRAINT memberships_role_check CHECK (
        role IN ('owner', 'admin', 'manager', 'editor', 'creator', 'reviewer', 'viewer', 'client')
    )
);
CREATE INDEX memberships_user_id_idx ON memberships (user_id);

-- Browser sessions. Only a SHA-256 hash of the session token is stored.
CREATE TABLE sessions (
    id                uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash        bytea       NOT NULL,
    user_id           uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workos_session_id text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    expires_at        timestamptz NOT NULL,

    CONSTRAINT sessions_token_hash_key UNIQUE (token_hash)
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
