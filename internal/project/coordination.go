package project

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/verdantflarehub/verdantflare-app-project/internal/coordination"
	"time"
)

// AuthorizeReservation is invoked only under the participant's durable barrier.
// It never performs a remote request and never reads World tables.
func AuthorizeReservation(ctx context.Context, tx pgx.Tx, o coordination.Operation, g coordination.Grant) error {
	p := Principal{OrganizationID: o.OrganizationID, SubjectID: o.SubjectID, RequestID: o.CommitID}
	if err := lockActive(ctx, tx, p, g.ResourceID); err != nil {
		if errors.Is(err, ErrForbidden) {
			return coordination.ErrDenied
		}
		return err
	}
	value, err := role(ctx, tx, p, g.ResourceID)
	if errors.Is(err, ErrForbidden) {
		return coordination.ErrDenied
	}
	if err != nil {
		return err
	}
	if g.Write && value != "owner" && value != "editor" {
		return coordination.ErrDenied
	}
	return nil
}
func (s *Service) operation(_ Principal, c commitRecord) coordination.Operation {
	if c.Plan.Operation != nil {
		return *c.Plan.Operation
	}
	return coordination.Operation{}
}
func (s *Service) validOperation(p Principal, c commitRecord) bool {
	o := s.operation(p, c)
	return o.Valid() && s.coord != nil && s.coord.Store != nil && o.Coordinator == "project" && o.OrganizationID == p.OrganizationID && o.SubjectID == c.Actor && o.ResourceID == c.ProjectID && o.CommitID == c.CommitID && o.RequestHash == c.Hash && o.Fence == s.coord.Store.Fence
}
func (s *Service) registerOperation(ctx context.Context, p Principal, c *commitRecord) error {
	if !s.validOperation(p, *c) {
		return coordination.ErrFenced
	}
	if c.Plan.Authorization == nil {
		c.Plan.Authorization = coordination.Plan{"project": {{ResourceID: c.ProjectID, Write: true}}}
	}
	// Authorization can be fixed after content preparation, but the operation
	// identity is immutable from insertion and is never reconstructed here.
	if err := s.savePlan(ctx, p, *c); err != nil {
		return err
	}
	return s.coord.Store.Register(ctx, s.operation(p, *c), c.Plan.Authorization)
}
func (s *Service) reserve(ctx context.Context, p Principal, c *commitRecord) error {
	if c.Plan.Authorization == nil {
		plan := coordination.Plan{"project": {{ResourceID: c.ProjectID, Write: true}}}
		previous := map[ContentRef]bool{}
		if c.Expected != "" {
			m, _, err := revision(ctx, s.db, p.OrganizationID, c.ProjectID, c.Expected)
			if err != nil {
				return err
			}
			for _, f := range m.Files {
				previous[f.Content] = true
			}
		}
		for _, f := range c.Plan.Manifest.Files {
			if previous[f.Content] {
				continue
			}
			v, err := s.content.Metadata(ctx, p, f.Content, Access{})
			if err != nil {
				return err
			}
			if !ValidID(v.Source.ProjectID) || v.Source.AssetID != "" || v.ContentRef != f.Content || v.OrganizationID != p.OrganizationID {
				return ErrForbidden
			}
			plan["project"] = append(plan["project"], coordination.Grant{ResourceID: v.Source.ProjectID})
		}
		for _, a := range c.Plan.Manifest.AssetRefs {
			value, err := s.assets.Resolve(ctx, p, a)
			if err != nil {
				return err
			}
			plan["world"] = append(plan["world"], value.Grants...)
		}
		normalized, err := coordination.Normalize(plan)
		if err != nil {
			return err
		}
		c.Plan.Authorization = normalized
	}
	if err := s.registerOperation(ctx, p, c); err != nil {
		return err
	}
	return s.coord.Prepare(ctx, s.operation(p, *c), c.Plan.Authorization)
}
func (s *Service) authorizeDependencyContent(ctx context.Context, p Principal, assets []AssetRef, ref ContentRef) error {
	for _, a := range assets {
		refs, err := s.assets.References(ctx, p, a)
		if err != nil {
			return err
		}
		for _, r := range refs {
			if r == ref {
				return nil
			}
		}
	}
	return ErrForbidden
}

// RecoverOnce retries persisted business preparations as their original actor.
// It also delivers terminal outbox entries; transport failures leave reservations.
func (s *Service) RecoverOnce(ctx context.Context) error {
	rows, err := s.db.Query(ctx, "SELECT organization_id::text,project_id::text,commit_id::text,created_by::text FROM studio.project_commits WHERE state='preparing' ORDER BY updated_at LIMIT 100")
	if err != nil {
		return err
	}
	type pending struct{ org, id, commit, actor string }
	var work []pending
	for rows.Next() {
		var item pending
		if err = rows.Scan(&item.org, &item.id, &item.commit, &item.actor); err != nil {
			break
		}
		work = append(work, item)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	var failures []error
	if err = s.coord.Deliver(ctx); err != nil {
		failures = append(failures, err)
	}
	for _, item := range work {
		attempt, cancel := context.WithTimeout(ctx, 2*time.Minute)
		_, err = s.resume(attempt, Principal{OrganizationID: item.org, SubjectID: item.actor, RequestID: newID()}, item.id, item.commit)
		cancel()
		if err != nil {
			failures = append(failures, err)
		}
	}
	if err = s.coord.Deliver(ctx); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}
func (s *Service) RunRecovery(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		_ = s.RecoverOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
