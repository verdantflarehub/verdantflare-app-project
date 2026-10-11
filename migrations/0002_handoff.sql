CREATE TABLE studio.project_handoff (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 state text NOT NULL CHECK(state IN ('closed','importing','staged','active')),
 migration_owner name NOT NULL DEFAULT CURRENT_USER,
 handoff_id uuid,
 archive_digest text,
 domain_digest text,
 project_fence bigint,
 world_fence bigint,
 CHECK((handoff_id IS NULL)=(archive_digest IS NULL)),
 CHECK((handoff_id IS NULL)=(domain_digest IS NULL)),
 CHECK((handoff_id IS NULL)=(project_fence IS NULL)),
 CHECK((handoff_id IS NULL)=(world_fence IS NULL))
);
INSERT INTO studio.project_handoff(state) VALUES('closed');
CREATE FUNCTION studio.project_check_writer() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE gate studio.project_handoff;
BEGIN
 SELECT * INTO STRICT gate FROM studio.project_handoff WHERE singleton FOR SHARE;
 IF gate.state='active' OR (gate.state='importing' AND session_user=gate.migration_owner) THEN RETURN NULL; END IF;
 RAISE EXCEPTION 'business writer is closed' USING ERRCODE='55000';
END;
$$;
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.projects FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.projects ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_members FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.project_members ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_create_requests FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.project_create_requests ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_commits FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.project_commits ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_revisions FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.project_revisions ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_events FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.project_events ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_coord_operations FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.project_coord_operations ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_coord_participants FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.project_coord_participants ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_coord_barriers FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.project_coord_barriers ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_coord_reservations FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.project_coord_reservations ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_coord_outbox FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.project_coord_outbox ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE TRIGGER project_handoff_gate BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON studio.project_member_changes FOR EACH STATEMENT EXECUTE FUNCTION studio.project_check_writer();
ALTER TABLE studio.project_member_changes ENABLE ALWAYS TRIGGER project_handoff_gate;
CREATE FUNCTION studio.project_runtime_ready() RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog AS $$
SELECT state='active'
AND NOT pg_has_role(session_user,migration_owner,'MEMBER')
AND NOT pg_has_role(session_user,(SELECT nspowner FROM pg_namespace WHERE nspname='studio'),'MEMBER')
AND NOT has_schema_privilege(session_user,'studio','CREATE')
AND NOT EXISTS(SELECT 1 FROM pg_class WHERE relnamespace='studio'::regnamespace AND relkind IN ('r','p') AND pg_has_role(session_user,relowner,'MEMBER'))
AND NOT (SELECT rolsuper FROM pg_roles WHERE rolname=session_user)
AND NOT has_table_privilege(session_user,'studio.project_handoff','INSERT,UPDATE,DELETE,TRUNCATE')
AND NOT has_table_privilege(session_user,'studio.project_coord_fences','INSERT,UPDATE,DELETE,TRUNCATE')
AND NOT has_table_privilege(session_user,'studio.project_schema_migrations','INSERT,UPDATE,DELETE,TRUNCATE')
FROM studio.project_handoff WHERE singleton;
$$;
