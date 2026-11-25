CREATE TABLE studio.project_crew_tasks (
 organization_id uuid NOT NULL, project_id uuid NOT NULL, task_id uuid NOT NULL, state jsonb NOT NULL,
 PRIMARY KEY(organization_id,project_id,task_id),
 FOREIGN KEY(organization_id,project_id) REFERENCES studio.projects(organization_id,project_id)
);
CREATE TABLE studio.project_crew_events (
 organization_id uuid NOT NULL, project_id uuid NOT NULL, task_id uuid NOT NULL, event_id uuid NOT NULL,
 generation uuid NOT NULL, actor uuid NOT NULL, action text NOT NULL, request_hash text NOT NULL,
 payload jsonb NOT NULL, receipt jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(organization_id,project_id,task_id,event_id),
 FOREIGN KEY(organization_id,project_id,task_id) REFERENCES studio.project_crew_tasks
);
CREATE INDEX project_crew_events_generation ON studio.project_crew_events(organization_id,project_id,task_id,generation,created_at);
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_crew_tasks FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.project_crew_tasks ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_crew_events FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.project_crew_events ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE TRIGGER project_crew_events_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON studio.project_crew_events FOR EACH STATEMENT EXECUTE FUNCTION studio.project_reject_immutable_change();
ALTER TABLE studio.project_crew_events ENABLE ALWAYS TRIGGER project_crew_events_immutable;
