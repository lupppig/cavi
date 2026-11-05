DROP TABLE project_access;
DROP TABLE projects;
ALTER TABLE memberships DROP CONSTRAINT memberships_id_organisation_key;
ALTER TABLE workspaces DROP CONSTRAINT workspaces_id_organisation_key;
