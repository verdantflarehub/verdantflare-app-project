package project

import (
	"context"
	"encoding/json"
)

// DashboardEdit narrows the existing commit protocol to project presentation data.
// The normal commit path still owns CAS, request recovery, schema and authorization.
func (s *Service) DashboardEdit(ctx context.Context, p Principal, r CommitRequest) (Result, error) {
	if err := dashboardEditShape(r); err != nil {
		return Result{}, err
	}
	if err := writable(ctx, s.db, p, r.ProjectID); err != nil {
		return Result{}, err
	}
	m, _, err := revision(ctx, s.db, p.OrganizationID, r.ProjectID, r.ExpectedRevisionID)
	if err != nil {
		return Result{}, err
	}
	if err = dashboardEditDocument(r, m); err != nil {
		return Result{}, err
	}
	return s.commitWithPreparation(ctx, p, r, false, changedDashboard)
}

// A first domain document uses a request-local placeholder. Allocate the real
// file ID inside the durable preparation, without changing the original hash.
func changedDashboard(previous Manifest, request CommitRequest) (preparePlan, error) {
	if err := dashboardEditDocument(request, previous); err != nil {
		return preparePlan{}, err
	}
	c := request.Changes
	if c.UpsertFiles == nil || c.DomainDocuments == nil {
		return changed(previous, request)
	}
	copy := *c
	files := append([]FileUpdate{}, (*c.UpsertFiles)...)
	files[0].ID = ""
	copy.UpsertFiles = &files
	copy.DomainDocuments = nil
	normalized := request
	normalized.Changes = &copy
	plan, err := changed(previous, normalized)
	if err != nil {
		return preparePlan{}, err
	}
	if len(plan.Writes) != 1 {
		return preparePlan{}, ErrInvalid
	}
	fileID := plan.Writes[0].FileID
	plan.Manifest.DomainDocuments = append(plan.Manifest.DomainDocuments, DomainDocument{Type: "project-content", FileID: fileID})
	if plan.Manifest.validate(map[string]bool{fileID: true}) != nil {
		return preparePlan{}, ErrInvalid
	}
	raw, err := json.Marshal(plan.Manifest)
	if err != nil || len(raw) > MaxManifestBytes {
		return preparePlan{}, ErrInvalid
	}
	return plan, nil
}

func dashboardEditShape(r CommitRequest) error {
	c := r.Changes
	if !ValidID(r.ProjectID) || !ValidID(r.ExpectedRevisionID) || !ValidID(r.CommitID) || r.Manifest != nil || len(r.Reselected) != 0 || c == nil || c.RemoveFileIDs != nil || c.Selections != nil || c.AssetRefs != nil || c.RunRefs != nil || (c.Metadata == nil && c.UpsertFiles == nil) {
		return ErrInvalid
	}
	if c.Metadata != nil && (c.Metadata.Name == nil || !validText(*c.Metadata.Name) || c.Metadata.Category != nil || c.Metadata.Status != nil || c.Metadata.EntryDocumentID != nil) {
		return ErrInvalid
	}
	if c.UpsertFiles == nil {
		if c.DomainDocuments != nil {
			return ErrInvalid
		}
		return nil
	}
	files := *c.UpsertFiles
	if len(files) != 1 || !ValidID(files[0].ID) || files[0].Text == nil || files[0].Content != nil || files[0].MIME != "application/json" || len(*files[0].Text) > 1<<20 {
		return ErrInvalid
	}
	return nil
}

func dashboardEditDocument(r CommitRequest, m Manifest) error {
	c := r.Changes
	if c.UpsertFiles == nil {
		return nil
	}
	f := (*c.UpsertFiles)[0]
	var existing string
	for _, d := range m.DomainDocuments {
		if d.Type == "project-content" {
			if existing != "" {
				return ErrInvalid
			}
			existing = d.FileID
		}
	}
	if existing != "" {
		if c.DomainDocuments != nil || f.ID != existing {
			return ErrInvalid
		}
		for _, old := range m.Files {
			if old.ID == existing && old.Path == f.Path && old.Role == f.Role {
				return nil
			}
		}
		return ErrInvalid
	}
	if f.Path != "project-content.json" || f.Role != "domain" || c.DomainDocuments == nil || len(*c.DomainDocuments) != len(m.DomainDocuments)+1 {
		return ErrInvalid
	}
	for _, old := range m.Files {
		if old.ID == f.ID || PathKey(old.Path) == PathKey(f.Path) {
			return ErrInvalid
		}
	}
	docs := *c.DomainDocuments
	for i, old := range m.DomainDocuments {
		if docs[i] != old {
			return ErrInvalid
		}
	}
	if docs[len(docs)-1] != (DomainDocument{Type: "project-content", FileID: f.ID}) {
		return ErrInvalid
	}
	return nil
}
