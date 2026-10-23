// Package migrations owns the independent Project schema. Legacy SQL is retained
// byte-for-byte as evidence and is never reapplied to an existing Studio database.
package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/verdantflarehub/verdantflare-app-project/internal/coordination"
)

//go:embed 0001_project.sql legacy/*.sql
var files embed.FS

func source() (string, string, error) {
	data, err := files.ReadFile("0001_project.sql")
	if err != nil {
		return "", "", err
	}
	coord, err := coordination.Schema("project")
	if err != nil {
		return "", "", err
	}
	sql := string(data) + "\n" + coord
	sum := sha256.Sum256([]byte(sql))
	return sql, hex.EncodeToString(sum[:]), nil
}
func Check(ctx context.Context, db *pgxpool.Pool) error {
	_, expected, err := source()
	if err != nil {
		return err
	}
	var actual string
	if err = db.QueryRow(ctx, "SELECT checksum FROM studio.project_schema_migrations WHERE version=1").Scan(&actual); err != nil {
		return err
	}
	if actual != expected {
		return errors.New("Project schema checksum mismatch")
	}
	return CheckGate(ctx, db)
}

// ApplyFresh is an explicit, empty-database installation. It refuses takeover
// of Studio tables; data adoption requires the reviewed freeze/import procedure.
// Fence values must come from the external recovery authority, never a default.
func ApplyFresh(ctx context.Context, db *pgxpool.Pool, projectFence, worldFence int64) error {
	if projectFence < 1 || worldFence < 1 {
		return coordination.ErrFenced
	}
	sql, sum, err := source()
	if err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(2026101101)"); err != nil {
		return err
	}
	var occupied bool
	if err = tx.QueryRow(ctx, "SELECT to_regclass('studio.projects') IS NOT NULL OR to_regclass('studio.schema_migrations') IS NOT NULL OR to_regclass('studio.world_assets') IS NOT NULL").Scan(&occupied); err != nil {
		return err
	}
	if occupied {
		return errors.New("fresh Project migration requires an empty business schema")
	}
	if _, err = tx.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS studio; CREATE TABLE studio.project_schema_migrations(version integer PRIMARY KEY,checksum text NOT NULL);"+sql); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO studio.project_schema_migrations(version,checksum) VALUES(1,$1)", sum); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO studio.project_coord_fences(coordinator,fence) VALUES('project',$1),('world',$2)", projectFence, worldFence); err != nil {
		return err
	}
	if err = applyGate(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "CREATE TABLE studio.project_extensions(name text PRIMARY KEY,checksum text NOT NULL);"+lifecycleSQL); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO studio.project_extensions VALUES('lifecycle',$1)", lifecycleDigest()); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, setReviewSQL); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO studio.project_extensions VALUES('set_review',$1)", setReviewDigest()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
