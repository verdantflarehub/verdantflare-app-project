ALTER TABLE studio.projects ADD COLUMN deleted boolean NOT NULL DEFAULT false;
ALTER TABLE studio.projects ADD COLUMN lifecycle_version bigint NOT NULL DEFAULT 0 CHECK(lifecycle_version>=0);
CREATE TABLE studio.project_lifecycle_changes (
 organization_id uuid NOT NULL, change_id uuid NOT NULL, project_id uuid NOT NULL,
 requested_by uuid NOT NULL, request_sha256 text NOT NULL, result jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(organization_id,change_id),
 FOREIGN KEY(organization_id,project_id) REFERENCES studio.projects(organization_id,project_id)
);
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_lifecycle_changes FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.project_lifecycle_changes ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE TRIGGER project_lifecycle_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON studio.project_lifecycle_changes FOR EACH STATEMENT EXECUTE FUNCTION studio.project_reject_immutable_change();
