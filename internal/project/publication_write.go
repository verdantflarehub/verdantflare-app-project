package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/verdantflarehub/verdantflare-app-project/internal/strictjson"
)

type PublicationPrepareRequest struct {
	ProjectID        string `json:"project_id"`
	OriginRevisionID string `json:"origin_revision_id"`
	CandidateKey     string `json:"candidate_key"`
	CommitID         string `json:"commit_id"`
	PrepareCommitID  string `json:"prepare_commit_id"`
}
type PublicationAssociateRequest struct {
	PublicationReceiptRequest
	ExpectedRevisionID  string `json:"expected_revision_id"`
	AssociationCommitID string `json:"association_commit_id"`
}
type publicationDocument struct {
	Opened OpenResult
	File   File
	Root   contentObject
}

func (s *Service) publicationDocument(ctx context.Context, p Principal, projectID, revisionID string) (publicationDocument, error) {
	var out publicationDocument
	opened, err := s.Open(ctx, p, projectID, revisionID)
	if err != nil {
		return out, err
	}
	if opened.ProjectID != projectID || opened.RevisionID != revisionID || opened.Manifest.ProjectID != projectID || opened.Manifest.Category != "ai_short_drama" {
		return out, ErrInvalid
	}
	var file File
	for _, document := range opened.Manifest.DomainDocuments {
		if document.Type == "project-content" {
			if file.ID != "" {
				return out, ErrInvalid
			}
			for _, f := range opened.Manifest.Files {
				if f.ID == document.FileID {
					file = f
				}
			}
		}
	}
	if file.ID == "" {
		return out, ErrInvalid
	}
	data, err := s.content.Read(ctx, p, file.Content, Access{ProjectID: projectID, RevisionID: revisionID}, MaxManifestBytes)
	if err != nil {
		return out, err
	}
	data, err = strictjson.ReadValue(bytes.NewReader(data), MaxManifestBytes)
	if err != nil {
		return out, ErrInvalid
	}
	var root contentObject
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&root) != nil || objectOf(root["ai_short_drama"]) == nil {
		return out, ErrInvalid
	}
	return publicationDocument{Opened: opened, File: file, Root: root}, nil
}
func (d publicationDocument) candidate(key string) (contentObject, error) {
	var found contentObject
	for _, value := range listOf(objectOf(d.Root["ai_short_drama"])["world_candidates"]) {
		c := objectOf(value)
		if str(c["key"]) == key {
			if found != nil {
				return nil, ErrInvalid
			}
			found = c
		}
	}
	if found == nil || objectOf(found["review"])["state"] != "approved" || !ValidID(str(found["publication_record_file_id"])) {
		return nil, ErrInvalid
	}
	return found, nil
}
func publicationRegistration(c contentObject, projectID, commitID string) (WorldRegisterRequest, error) {
	var request WorldRegisterRequest
	raw, err := json.Marshal(c["registration_request"])
	if err != nil || strictjson.Decode(bytes.NewReader(raw), MaxManifestBytes, &request) != nil {
		return request, ErrInvalid
	}
	request, err = normalizeWorld(request)
	if err != nil || request.CommitID != commitID || request.SourceProjectID != projectID || request.ReviewFileID != str(objectOf(c["review"])["record_file_id"]) {
		return request, ErrInvalid
	}
	return request, nil
}
func (d publicationDocument) commit(commitID string) (CommitRequest, error) {
	data, err := json.Marshal(d.Root)
	if err != nil {
		return CommitRequest{}, ErrInvalid
	}
	text := string(data)
	files := []FileUpdate{{ID: d.File.ID, Path: d.File.Path, Role: d.File.Role, Text: &text, MIME: "application/json"}}
	return CommitRequest{ProjectID: d.Opened.ProjectID, ExpectedRevisionID: d.Opened.RevisionID, CommitID: commitID, Changes: &Changes{UpsertFiles: &files}}, nil
}
func (s *Service) publicationCommit(ctx context.Context, p Principal, request CommitRequest) (CommitStatus, error) {
	hash, err := digest(request)
	if err != nil {
		return CommitStatus{}, err
	}
	result, err := s.commit(ctx, p, request, true)
	if err == nil {
		return CommitStatus{State: "committed", RequestHash: hash, Result: &result}, nil
	}
	status, lookup := s.Status(ctx, p, request.ProjectID, request.CommitID)
	if lookup != nil {
		return CommitStatus{}, err
	}
	if status.RequestHash != hash {
		return CommitStatus{}, ErrIdempotency
	}
	return status, nil
}

// Only the server's publication operations use the larger document allowance.
// Public Commit and every other inline write retain the original 1 MiB bound.
func publicationChange(previous Manifest, r CommitRequest) error {
	c := r.Changes
	if previous.Category != "ai_short_drama" || r.Manifest != nil || len(r.Reselected) != 0 || c == nil || c.Metadata != nil || c.RemoveFileIDs != nil || c.Selections != nil || c.AssetRefs != nil || c.RunRefs != nil || c.DomainDocuments != nil || c.UpsertFiles == nil || len(*c.UpsertFiles) != 1 {
		return ErrInvalid
	}
	f := (*c.UpsertFiles)[0]
	if f.Content != nil || f.Text == nil || f.MIME != "application/json" || len(*f.Text) > MaxManifestBytes {
		return ErrInvalid
	}
	for _, d := range previous.DomainDocuments {
		if d.Type != "project-content" || d.FileID != f.ID {
			continue
		}
		for _, old := range previous.Files {
			if old.ID == f.ID && old.Path == f.Path && old.Role == f.Role {
				return nil
			}
		}
	}
	return ErrInvalid
}
func (s *Service) PublicationPrepare(ctx context.Context, p Principal, r PublicationPrepareRequest) (CommitStatus, error) {
	if !p.Valid() {
		return CommitStatus{}, ErrForbidden
	}
	if !ValidID(r.ProjectID) || !ValidID(r.OriginRevisionID) || !ValidID(r.CommitID) || !ValidID(r.PrepareCommitID) || !publicationKey.MatchString(r.CandidateKey) || r.CommitID == r.PrepareCommitID {
		return CommitStatus{}, ErrInvalid
	}
	if err := writable(ctx, s.db, p, r.ProjectID); err != nil {
		return CommitStatus{}, err
	}
	document, err := s.publicationDocument(ctx, p, r.ProjectID, r.OriginRevisionID)
	if err != nil {
		return CommitStatus{}, err
	}
	candidate, err := document.candidate(r.CandidateKey)
	if err != nil {
		return CommitStatus{}, err
	}
	var request WorldRegisterRequest
	if candidate["registration_request"] != nil {
		request, err = publicationRegistration(candidate, r.ProjectID, r.CommitID)
		if err != nil {
			return CommitStatus{}, err
		}
	} else {
		drama := objectOf(document.Root["ai_short_drama"])
		locator := objectOf(candidate["source"])
		prefix := "globals/"
		if locator["kind"] == "deliverable" {
			prefix = "deliverables/"
		}
		owner := dramaOwners(drama)[prefix+str(locator["key"])]
		if owner == nil || len(listOf(owner["versions"])) == 0 {
			return CommitStatus{}, ErrInvalid
		}
		recordID := str(candidate["publication_record_file_id"])
		var record File
		for _, f := range document.Opened.Manifest.Files {
			if f.ID == recordID {
				record = f
			}
		}
		if !record.Content.Valid() {
			return CommitStatus{}, ErrInvalid
		}
		data, e := s.content.Read(ctx, p, record.Content, Access{ProjectID: r.ProjectID, RevisionID: r.OriginRevisionID}, MaxManifestBytes)
		if e != nil {
			return CommitStatus{}, e
		}
		var publication dramaPublication
		if json.Unmarshal(data, &publication) != nil {
			return CommitStatus{}, ErrInvalid
		}
		ids := []string{recordID}
		for _, version := range publication.Versions {
			for _, file := range version.FileRefs {
				ids = append(ids, file.FileID)
			}
		}
		for _, file := range publication.Supporting {
			ids = append(ids, file.FileID)
		}
		sort.Strings(ids)
		assetType, subject := str(owner["kind"]), str(owner["name"])
		if locator["kind"] == "deliverable" {
			assetType, subject = "video-project", str(owner["title"])
		}
		request, err = normalizeWorld(WorldRegisterRequest{CommitID: r.CommitID, SourceProjectID: r.ProjectID, SourceRevisionID: r.OriginRevisionID, FileIDs: ids, Name: str(candidate["name"]), AssetType: assetType, Subjects: []string{subject}, Relation: "produced_in", ReviewFileID: str(objectOf(candidate["review"])["record_file_id"])})
		if err != nil {
			return CommitStatus{}, err
		}
		if err = validateDramaPublication(data, candidate, owner, document.Opened.Manifest, recordID, request); err != nil {
			return CommitStatus{}, err
		}
	}
	candidate["registration_request"] = request
	// Existing successful status belongs to the same exact request; preserving
	// it avoids removing an already verified association during a recovery.
	commit, err := document.commit(r.PrepareCommitID)
	if err != nil {
		return CommitStatus{}, err
	}
	return s.publicationCommit(ctx, p, commit)
}

// priorAssociation reconstructs the precise request from the original base and
// this commit's durable plan. This permits recovery after World goes offline,
// while rejecting reused IDs, altered locators and any unrelated file mutation.
func priorAssociation(document publicationDocument, candidate contentObject, request WorldRegisterRequest, r PublicationAssociateRequest, p Principal, prior commitRecord) (CommitRequest, error) {
	if prior.Actor != p.SubjectID {
		return CommitRequest{}, ErrForbidden
	}
	if prior.Expected != r.ExpectedRevisionID || prior.ProjectID != r.ProjectID || prior.CommitID != r.AssociationCommitID || len(prior.Plan.Writes) != 1 {
		return CommitRequest{}, ErrIdempotency
	}
	write := prior.Plan.Writes[0]
	if write.FileID != document.File.ID || write.MIME != "application/json" {
		return CommitRequest{}, ErrIdempotency
	}
	var root contentObject
	decoder := json.NewDecoder(bytes.NewBufferString(write.Text))
	decoder.UseNumber()
	if decoder.Decode(&root) != nil {
		return CommitRequest{}, ErrIdempotency
	}
	planned, err := (publicationDocument{Root: root}).candidate(r.CandidateKey)
	if err != nil {
		return CommitRequest{}, ErrIdempotency
	}
	plannedRequest, err := publicationRegistration(planned, r.ProjectID, r.CommitID)
	if err != nil || !contentEqual(plannedRequest, request) {
		return CommitRequest{}, ErrIdempotency
	}
	raw, err := json.Marshal(planned["registration_status"])
	var status WorldStatus
	if err != nil || strictjson.Decode(bytes.NewReader(raw), MaxManifestBytes, &status) != nil {
		return CommitRequest{}, ErrIdempotency
	}
	hash, _ := digest(request)
	if status.State != "committed" || status.RequestHash != hash || status.Result == nil || status.Result.AssetID != r.AssetID || status.Result.VersionID != r.VersionID || status.Result.Manifest.Source.ProjectID != r.ProjectID || status.Result.Manifest.Source.RevisionID != request.SourceRevisionID {
		return CommitRequest{}, ErrIdempotency
	}
	candidate["registration_status"] = status
	commit, err := document.commit(r.AssociationCommitID)
	if err != nil {
		return CommitRequest{}, err
	}
	actual, err := digest(commit)
	if err != nil || actual != prior.Hash || *(*commit.Changes.UpsertFiles)[0].Text != write.Text {
		return CommitRequest{}, ErrIdempotency
	}
	return commit, nil
}
func (s *Service) PublicationAssociate(ctx context.Context, p Principal, r PublicationAssociateRequest) (CommitStatus, error) {
	if !p.Valid() {
		return CommitStatus{}, ErrForbidden
	}
	if !r.PublicationReceiptRequest.valid() || !ValidID(r.ExpectedRevisionID) || !ValidID(r.AssociationCommitID) || r.AssociationCommitID == r.CommitID {
		return CommitStatus{}, ErrInvalid
	}
	if err := writable(ctx, s.db, p, r.ProjectID); err != nil {
		return CommitStatus{}, err
	}
	prepared, err := s.publicationDocument(ctx, p, r.ProjectID, r.PreparedRevisionID)
	if err != nil {
		return CommitStatus{}, err
	}
	preparedCandidate, err := prepared.candidate(r.CandidateKey)
	if err != nil {
		return CommitStatus{}, err
	}
	request, err := publicationRegistration(preparedCandidate, r.ProjectID, r.CommitID)
	if err != nil {
		return CommitStatus{}, err
	}
	base, err := s.publicationDocument(ctx, p, r.ProjectID, r.ExpectedRevisionID)
	if err != nil {
		return CommitStatus{}, err
	}
	candidate, err := base.candidate(r.CandidateKey)
	if err != nil {
		return CommitStatus{}, err
	}
	currentRequest, err := publicationRegistration(candidate, r.ProjectID, r.CommitID)
	if err != nil || !contentEqual(currentRequest, request) {
		return CommitStatus{}, ErrInvalid
	}
	for _, field := range []string{"source", "review", "publication_record_file_id"} {
		if !contentEqual(candidate[field], preparedCandidate[field]) {
			return CommitStatus{}, ErrInvalid
		}
	}
	prior, err := loadCommit(ctx, s.db, p.OrganizationID, r.ProjectID, r.AssociationCommitID)
	if err == nil {
		commit, err := priorAssociation(base, candidate, request, r, p, prior)
		if err != nil {
			return CommitStatus{}, err
		}
		return s.publicationCommit(ctx, p, commit)
	}
	if !errors.Is(err, ErrNotFound) {
		return CommitStatus{}, err
	}
	status, err := s.PublicationReceipt(ctx, p, r.PublicationReceiptRequest)
	if err != nil {
		return CommitStatus{}, err
	}
	candidate["registration_status"] = status
	commit, err := base.commit(r.AssociationCommitID)
	if err != nil {
		return CommitStatus{}, err
	}
	return s.publicationCommit(ctx, p, commit)
}
