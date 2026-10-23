CREATE TABLE studio.project_set_review_targets (
 organization_id uuid NOT NULL, project_id uuid NOT NULL, target_hash text NOT NULL CHECK(target_hash ~ '^[0-9a-f]{64}$'),
 material_key text NOT NULL, state_key text NOT NULL, version_key text NOT NULL,
 content_revision_id uuid NOT NULL, production_hash text NOT NULL CHECK(production_hash ~ '^[0-9a-f]{64}$'),
 target_json text NOT NULL, first_revision_id uuid NOT NULL,
 PRIMARY KEY(organization_id,project_id,target_hash),
 FOREIGN KEY(organization_id,project_id) REFERENCES studio.projects(organization_id,project_id),
 FOREIGN KEY(first_revision_id) REFERENCES studio.project_revisions(revision_id),
 FOREIGN KEY(content_revision_id) REFERENCES studio.project_revisions(revision_id)
);
CREATE UNIQUE INDEX project_set_review_version ON studio.project_set_review_targets(organization_id,project_id,material_key,state_key,version_key);
CREATE TABLE studio.project_set_review_records (
 organization_id uuid NOT NULL, project_id uuid NOT NULL, target_hash text NOT NULL,
 record_id uuid NOT NULL, previous_record_id uuid, sequence_no bigint NOT NULL CHECK(sequence_no>0),
 revision_id uuid NOT NULL, file_id uuid NOT NULL, commit_id uuid NOT NULL,
 record_sha256 text NOT NULL CHECK(record_sha256 ~ '^[0-9a-f]{64}$'),
 scope_json text NOT NULL, scope_hash text NOT NULL, action text NOT NULL CHECK(action IN ('comment','request_changes','approve')),
 origin text NOT NULL CHECK(origin IN ('human_interactive','agent')), created_at timestamptz NOT NULL,
 PRIMARY KEY(organization_id,project_id,record_id),
 UNIQUE(organization_id,project_id,target_hash,sequence_no),
 UNIQUE(organization_id,project_id,commit_id),
 FOREIGN KEY(organization_id,project_id,target_hash) REFERENCES studio.project_set_review_targets(organization_id,project_id,target_hash),
 FOREIGN KEY(organization_id,project_id,previous_record_id) REFERENCES studio.project_set_review_records(organization_id,project_id,record_id),
 FOREIGN KEY(organization_id,project_id,commit_id) REFERENCES studio.project_commits(organization_id,project_id,commit_id),
 FOREIGN KEY(revision_id) REFERENCES studio.project_revisions(revision_id)
);
CREATE TABLE studio.project_set_review_heads (
 organization_id uuid NOT NULL, project_id uuid NOT NULL, target_hash text NOT NULL,
 record_id uuid NOT NULL, sequence_no bigint NOT NULL,
 PRIMARY KEY(organization_id,project_id,target_hash),
 FOREIGN KEY(organization_id,project_id,target_hash) REFERENCES studio.project_set_review_targets(organization_id,project_id,target_hash),
 FOREIGN KEY(organization_id,project_id,record_id) REFERENCES studio.project_set_review_records(organization_id,project_id,record_id)
);
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_set_review_targets FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_set_review_records FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_set_review_heads FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.project_set_review_targets ENABLE ALWAYS TRIGGER project_handoff_gate;
ALTER TABLE studio.project_set_review_records ENABLE ALWAYS TRIGGER project_handoff_gate;
ALTER TABLE studio.project_set_review_heads ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE TRIGGER project_set_review_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON studio.project_set_review_targets FOR EACH STATEMENT EXECUTE FUNCTION studio.project_reject_immutable_change();
CREATE TRIGGER project_set_review_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON studio.project_set_review_records FOR EACH STATEMENT EXECUTE FUNCTION studio.project_reject_immutable_change();
ALTER TABLE studio.project_set_review_targets ENABLE ALWAYS TRIGGER project_set_review_immutable;
ALTER TABLE studio.project_set_review_records ENABLE ALWAYS TRIGGER project_set_review_immutable;
