-- Composite keys so child tables can require that a referenced workspace or
-- membership belongs to the same organisation as the row itself.
ALTER TABLE workspaces ADD CONSTRAINT workspaces_id_organisation_key UNIQUE (id, organisation_id);
ALTER TABLE memberships ADD CONSTRAINT memberships_id_organisation_key UNIQUE (id, organisation_id);

-- A creative production unit. Owned by an organisation and filed in one of
-- its workspaces; the composite foreign key makes cross-tenant filing
-- impossible at the database level.
CREATE TABLE projects (
    id                 uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id    uuid        NOT NULL REFERENCES organisations (id) ON DELETE CASCADE,
    workspace_id       uuid        NOT NULL,
    name               text        NOT NULL,
    description        text        NOT NULL DEFAULT '',
    created_by_user_id uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT projects_name_length CHECK (char_length(name) BETWEEN 1 AND 120),
    CONSTRAINT projects_description_length CHECK (char_length(description) <= 2000),
    CONSTRAINT projects_id_organisation_key UNIQUE (id, organisation_id),
    CONSTRAINT projects_workspace_fkey FOREIGN KEY (workspace_id, organisation_id)
        REFERENCES workspaces (id, organisation_id) ON DELETE CASCADE
);
CREATE INDEX projects_organisation_created_idx ON projects (organisation_id, created_at DESC, id DESC);
CREATE INDEX projects_workspace_id_idx ON projects (workspace_id);

-- Explicit project access. Clients see only projects listed here. Rows hang
-- off the membership, so removing someone from the organisation removes
-- their project access too.
CREATE TABLE project_access (
    project_id         uuid        NOT NULL,
    membership_id      uuid        NOT NULL,
    organisation_id    uuid        NOT NULL,
    granted_by_user_id uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (project_id, membership_id),
    CONSTRAINT project_access_project_fkey FOREIGN KEY (project_id, organisation_id)
        REFERENCES projects (id, organisation_id) ON DELETE CASCADE,
    CONSTRAINT project_access_membership_fkey FOREIGN KEY (membership_id, organisation_id)
        REFERENCES memberships (id, organisation_id) ON DELETE CASCADE
);
CREATE INDEX project_access_membership_id_idx ON project_access (membership_id);
