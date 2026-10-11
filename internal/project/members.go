package project

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

type MemberChangeRequest struct {
	ProjectID string `json:"project_id"`
	SubjectID string `json:"subject_id"`
	ChangeID  string `json:"change_id"`
	Role      string `json:"role"`
}
type MemberChangeResult struct {
	ChangeID string `json:"change_id"`
	State    string `json:"state"`
}

// ChangeMember persists the intent before waiting. An authorized retry using the
// same change_id completes it after reservations drain; no TTL revokes the grant.
func (s *Service) ChangeMember(ctx context.Context, p Principal, r MemberChangeRequest) (MemberChangeResult, error) {
	result := MemberChangeResult{ChangeID: r.ChangeID, State: "pending"}
	if !p.Valid() || !ValidID(r.ProjectID) || !ValidID(r.SubjectID) || !ValidID(r.ChangeID) || (r.Role != "reader" && r.Role != "editor" && r.Role != "none") {
		return result, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(context.Background())
	if err = advisory(ctx, tx, "member:"+p.OrganizationID+":"+r.ProjectID+":"+r.SubjectID); err != nil {
		return result, err
	}
	actor, err := role(ctx, tx, p, r.ProjectID)
	if err != nil {
		return result, err
	}
	if actor != "owner" {
		return result, ErrForbidden
	}
	var owner string
	if err = tx.QueryRow(ctx, "SELECT owner_id::text FROM studio.projects WHERE organization_id=$1 AND project_id=$2", p.OrganizationID, r.ProjectID).Scan(&owner); err != nil {
		return result, err
	}
	if owner == r.SubjectID {
		return result, ErrInvalid
	}
	var projectID, subjectID, requestedBy, wanted, state string
	err = tx.QueryRow(ctx, "SELECT project_id::text,subject_id::text,requested_by::text,desired_role,state FROM studio.project_member_changes WHERE organization_id=$1 AND change_id=$2 FOR UPDATE", p.OrganizationID, r.ChangeID).Scan(&projectID, &subjectID, &requestedBy, &wanted, &state)
	if err == nil {
		if projectID != r.ProjectID || subjectID != r.SubjectID || requestedBy != p.SubjectID || wanted != r.Role {
			return result, ErrIdempotency
		}
		if state == "completed" {
			result.State = state
			return result, tx.Commit(ctx)
		}
	} else if errors.Is(err, pgx.ErrNoRows) {
		var pending bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM studio.project_member_changes WHERE organization_id=$1 AND project_id=$2 AND subject_id=$3 AND state='pending')", p.OrganizationID, r.ProjectID, r.SubjectID).Scan(&pending); err != nil {
			return result, err
		}
		if pending {
			return result, ErrConflict
		}
		if _, err = tx.Exec(ctx, "INSERT INTO studio.project_member_changes(organization_id,change_id,project_id,subject_id,requested_by,desired_role,state) VALUES($1,$2,$3,$4,$5,$6,'pending')", p.OrganizationID, r.ChangeID, r.ProjectID, r.SubjectID, p.SubjectID, r.Role); err != nil {
			return result, err
		}
	} else {
		return result, err
	}
	pending, err := s.coord.Store.BlockTx(ctx, tx, p.OrganizationID, r.ProjectID, r.SubjectID)
	if err != nil {
		return result, err
	}
	if pending {
		if err = audit(ctx, tx, p, "project.member.pending", r.ChangeID); err != nil {
			return result, err
		}
		return result, tx.Commit(ctx)
	}
	if r.Role == "none" {
		_, err = tx.Exec(ctx, "DELETE FROM studio.project_members WHERE organization_id=$1 AND project_id=$2 AND subject_id=$3", p.OrganizationID, r.ProjectID, r.SubjectID)
	} else {
		_, err = tx.Exec(ctx, "INSERT INTO studio.project_members(organization_id,project_id,subject_id,role) VALUES($1,$2,$3,$4) ON CONFLICT(organization_id,project_id,subject_id) DO UPDATE SET role=EXCLUDED.role", p.OrganizationID, r.ProjectID, r.SubjectID, r.Role)
	}
	if err != nil {
		return result, err
	}
	if err = s.coord.Store.ChangedTx(ctx, tx, p.OrganizationID, r.ProjectID, r.SubjectID); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, "UPDATE studio.project_member_changes SET state='completed',updated_at=now() WHERE organization_id=$1 AND change_id=$2", p.OrganizationID, r.ChangeID); err != nil {
		return result, err
	}
	if err = audit(ctx, tx, p, "project.member."+r.Role, r.ChangeID); err != nil {
		return result, err
	}
	result.State = "completed"
	return result, tx.Commit(ctx)
}
