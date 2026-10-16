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

//go:embed 0004_lifecycle.sql
var lifecycleSQL string

func lifecycleDigest() string {
	sum := sha256.Sum256([]byte(lifecycleSQL))
	return hex.EncodeToString(sum[:])
}

func CheckLifecycle(ctx context.Context, q gateQuery) error {
	var checksum string
	if err := q.QueryRow(ctx, "SELECT checksum FROM studio.project_extensions WHERE name='lifecycle'").Scan(&checksum); err != nil {
		return err
	}
	if checksum != lifecycleDigest() {
		return errors.New("lifecycle migration checksum mismatch")
	}
	return nil
}

// UpgradeLifecycle preserves the original gate ledger and is migration-owner only.
func UpgradeLifecycle(ctx context.Context, db *pgxpool.Pool) error {
	if err := Check(ctx, db); err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(2026101601)"); err != nil {
		return err
	}
	var owner bool
	if err = tx.QueryRow(ctx, "SELECT session_user=migration_owner FROM studio.project_handoff WHERE singleton FOR SHARE").Scan(&owner); err != nil {
		return err
	}
	if !owner {
		return errors.New("lifecycle upgrade requires migration owner")
	}
	if _, err = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS studio.project_extensions(name text PRIMARY KEY,checksum text NOT NULL)"); err != nil {
		return err
	}
	var sum string
	err = tx.QueryRow(ctx, "SELECT checksum FROM studio.project_extensions WHERE name='lifecycle'").Scan(&sum)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err = tx.Exec(ctx, lifecycleSQL); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO studio.project_extensions VALUES('lifecycle',$1)", lifecycleDigest()); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if sum != lifecycleDigest() {
		return errors.New("lifecycle migration checksum mismatch")
	}
	// Copy only existing explicit application grantees; never grant PUBLIC access.
	if _, err = tx.Exec(ctx, `DO $$ DECLARE r record; BEGIN
 FOR r IN SELECT DISTINCT roles.rolname FROM pg_class c CROSS JOIN LATERAL aclexplode(c.relacl) a JOIN pg_roles roles ON roles.oid=a.grantee WHERE c.oid='studio.projects'::regclass AND a.privilege_type='INSERT' AND a.grantee<>c.relowner LOOP
	EXECUTE format('REVOKE ALL ON studio.project_lifecycle_changes,studio.project_extensions FROM %I',r.rolname);
 EXECUTE format('GRANT SELECT,INSERT ON studio.project_lifecycle_changes TO %I',r.rolname);
 EXECUTE format('GRANT SELECT ON studio.project_extensions TO %I',r.rolname);
 END LOOP; END $$`); err != nil {
		return err
	}
	if err = CheckLifecycle(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
