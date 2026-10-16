package project

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
)

var ErrBusy = errors.New("PROJECT_BUSY")
var ErrLifecycleConflict = errors.New("LIFECYCLE_CONFLICT")

type LifecycleRequest struct {
	ProjectID                string `json:"project_id"`
	ChangeID                 string `json:"change_id"`
	ExpectedRevisionID       string `json:"expected_revision_id"`
	ExpectedLifecycleVersion *int64 `json:"expected_lifecycle_version"`
}
type LifecycleStatusRequest struct {
	ProjectID string `json:"project_id"`
	ChangeID  string `json:"change_id"`
}
type LifecycleResult struct {
	ProjectID        string `json:"project_id"`
	ChangeID         string `json:"change_id"`
	State            string `json:"state"`
	Deleted          bool   `json:"deleted"`
	LifecycleVersion int64  `json:"lifecycle_version"`
	HeadRevisionID   string `json:"head_revision_id"`
}

func lifecycleOwner(ctx context.Context, q queryer, p Principal, id string) error {
	if !p.Valid() || !ValidID(id) {
		return ErrForbidden
	}
	var owner bool
	err := q.QueryRow(ctx, "SELECT owner_id=$3::uuid FROM studio.projects WHERE organization_id=$1 AND project_id=$2", p.OrganizationID, id, p.SubjectID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !owner {
		return ErrForbidden
	}
	return err
}

// lockActive serializes new commit preparations and grant reservations with deletion.
func lockActive(ctx context.Context, tx pgx.Tx, p Principal, id string) error {
	var deleted bool
	err := tx.QueryRow(ctx, "SELECT deleted FROM studio.projects WHERE organization_id=$1 AND project_id=$2 FOR SHARE", p.OrganizationID, id).Scan(&deleted)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && deleted {
		return ErrForbidden
	}
	return err
}
func (s *Service) LifecycleStatus(ctx context.Context, p Principal, r LifecycleStatusRequest) (LifecycleResult, error) {
	var out LifecycleResult
	if !ValidID(r.ChangeID) {
		return out, ErrInvalid
	}
	if err := lifecycleOwner(ctx, s.db, p, r.ProjectID); err != nil {
		return out, err
	}
	var data []byte
	err := s.db.QueryRow(ctx, "SELECT result FROM studio.project_lifecycle_changes WHERE organization_id=$1 AND project_id=$2 AND change_id=$3 AND requested_by=$4", p.OrganizationID, r.ProjectID, r.ChangeID, p.SubjectID).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(data, &out)
	return out, err
}
func (s *Service) ChangeLifecycle(ctx context.Context, p Principal, r LifecycleRequest, deleted bool) (LifecycleResult, error) {
	var out LifecycleResult
	if !p.Valid() || !ValidID(r.ProjectID) || !ValidID(r.ChangeID) || !ValidID(r.ExpectedRevisionID) || r.ExpectedLifecycleVersion == nil || *r.ExpectedLifecycleVersion < 0 || *r.ExpectedLifecycleVersion >= 9007199254740991 {
		return out, ErrInvalid
	}
	hash, _ := digest(struct {
		Request LifecycleRequest
		Deleted bool
	}{r, deleted})
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(context.Background())
	if err = advisory(ctx, tx, "lifecycle:"+p.OrganizationID+":"+r.ChangeID); err != nil {
		return out, err
	}
	if err = lifecycleOwner(ctx, tx, p, r.ProjectID); err != nil {
		return out, err
	}
	var prior, actor string
	var data []byte
	err = tx.QueryRow(ctx, "SELECT request_sha256,requested_by::text,result FROM studio.project_lifecycle_changes WHERE organization_id=$1 AND change_id=$2", p.OrganizationID, r.ChangeID).Scan(&prior, &actor, &data)
	if err == nil {
		if prior != hash || actor != p.SubjectID {
			return out, ErrIdempotency
		}
		err = json.Unmarshal(data, &out)
		return out, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	var current bool
	var version int64
	var head string
	err = tx.QueryRow(ctx, "SELECT deleted,lifecycle_version,COALESCE(head_revision_id::text,'') FROM studio.projects WHERE organization_id=$1 AND project_id=$2 FOR UPDATE", p.OrganizationID, r.ProjectID).Scan(&current, &version, &head)
	if err != nil {
		return out, err
	}
	if head != r.ExpectedRevisionID {
		return out, &ConflictError{CurrentRevisionID: head}
	}
	if version != *r.ExpectedLifecycleVersion || current == deleted {
		return out, ErrLifecycleConflict
	}
	if deleted {
		var busy bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM studio.project_commits WHERE organization_id=$1 AND project_id=$2 AND state='preparing') OR EXISTS(SELECT 1 FROM studio.project_coord_reservations WHERE organization_id=$1 AND grant_resource_id=$2)`, p.OrganizationID, r.ProjectID).Scan(&busy)
		if err != nil {
			return out, err
		}
		if busy {
			return out, ErrBusy
		}
	}
	out = LifecycleResult{ProjectID: r.ProjectID, ChangeID: r.ChangeID, State: "completed", Deleted: deleted, LifecycleVersion: version + 1, HeadRevisionID: head}
	if _, err = tx.Exec(ctx, "UPDATE studio.projects SET deleted=$3,lifecycle_version=lifecycle_version+1 WHERE organization_id=$1 AND project_id=$2", p.OrganizationID, r.ProjectID, deleted); err != nil {
		return out, err
	}
	data, _ = json.Marshal(out)
	if _, err = tx.Exec(ctx, "INSERT INTO studio.project_lifecycle_changes(organization_id,change_id,project_id,requested_by,request_sha256,result) VALUES($1,$2,$3,$4,$5,$6)", p.OrganizationID, r.ChangeID, r.ProjectID, p.SubjectID, hash, data); err != nil {
		return out, err
	}
	action := "project.restore"
	if deleted {
		action = "project.delete"
	}
	if err = audit(ctx, tx, p, action, r.ProjectID); err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
