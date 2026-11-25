package migrations

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed 0006_crew_task.sql
var crewTaskSQL string

func crewTaskDigest() string {
	sum := sha256.Sum256([]byte(crewTaskSQL))
	return hex.EncodeToString(sum[:])
}
func CheckCrewTask(ctx context.Context, q gateQuery) error {
	var hash string
	if err := q.QueryRow(ctx, "SELECT checksum FROM studio.project_extensions WHERE name='crew_task'").Scan(&hash); err != nil {
		return err
	}
	if hash != crewTaskDigest() {
		return errors.New("crew task migration checksum mismatch")
	}
	for _, table := range []string{"project_crew_events", "project_crew_tasks"} {
		var valid bool
		if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid=$1::regclass AND tgname='project_handoff_gate' AND tgenabled='A' AND NOT tgisinternal AND tgtype=62 AND tgnargs=0 AND tgqual IS NULL AND tgfoid='studio.project_check_writer()'::regprocedure)`, "studio."+table).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return errors.New("crew task writer guard missing")
		}
	}
	for _, table := range []string{"project_crew_events"} {
		var valid bool
		if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid=$1::regclass AND tgname='project_crew_events_immutable' AND tgenabled='A' AND NOT tgisinternal AND tgtype=58 AND tgnargs=0 AND tgqual IS NULL AND tgfoid='studio.project_reject_immutable_change()'::regprocedure)`, "studio."+table).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return errors.New("crew task immutable guard missing")
		}
	}
	return nil
}
func UpgradeCrewTask(ctx context.Context, db *pgxpool.Pool) error {
	if err := Check(ctx, db); err != nil {
		return err
	}
	if err := CheckLifecycle(ctx, db); err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(2026112601)"); err != nil {
		return err
	}
	var owner bool
	if err = tx.QueryRow(ctx, "SELECT session_user=migration_owner FROM studio.project_handoff WHERE singleton FOR SHARE").Scan(&owner); err != nil {
		return err
	}
	if !owner {
		return errors.New("crew task upgrade requires migration owner")
	}
	var hash string
	err = tx.QueryRow(ctx, "SELECT checksum FROM studio.project_extensions WHERE name='crew_task'").Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err = tx.Exec(ctx, crewTaskSQL); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO studio.project_extensions VALUES('crew_task',$1)", crewTaskDigest()); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if hash != crewTaskDigest() {
		return errors.New("crew task migration checksum mismatch")
	}
	if _, err = tx.Exec(ctx, `DO $$ DECLARE r record; BEGIN
 FOR r IN SELECT DISTINCT roles.rolname FROM pg_class c CROSS JOIN LATERAL aclexplode(c.relacl) a JOIN pg_roles roles ON roles.oid=a.grantee WHERE c.oid='studio.projects'::regclass AND a.privilege_type='INSERT' AND a.grantee<>c.relowner LOOP
 EXECUTE format('REVOKE ALL ON studio.project_crew_events,studio.project_crew_tasks FROM %I',r.rolname);
 EXECUTE format('GRANT SELECT,INSERT ON studio.project_crew_events TO %I',r.rolname);
 EXECUTE format('GRANT SELECT,INSERT,UPDATE ON studio.project_crew_tasks TO %I',r.rolname);
 END LOOP; END $$`); err != nil {
		return err
	}
	if err = CheckCrewTask(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
