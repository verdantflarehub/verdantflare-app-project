package project

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"

	"github.com/verdantflarehub/verdantflare-app-project/internal/strictjson"
)

type PublicationReceiptRequest struct {
	ProjectID          string `json:"project_id"`
	PreparedRevisionID string `json:"prepared_revision_id"`
	CandidateKey       string `json:"candidate_key"`
	CommitID           string `json:"commit_id"`
	AssetID            string `json:"asset_id"`
	VersionID          string `json:"asset_version_id"`
}

var publicationKey = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (r PublicationReceiptRequest) valid() bool {
	return ValidID(r.ProjectID) && ValidID(r.PreparedRevisionID) && ValidID(r.CommitID) && ValidID(r.AssetID) && ValidID(r.VersionID) && publicationKey.MatchString(r.CandidateKey)
}

type publicationSnapshot func(context.Context, Principal, string, string) (OpenResult, contentObject, error)

func (s *Service) readPublicationSnapshot(ctx context.Context, p Principal, projectID, revisionID string) (OpenResult, contentObject, error) {
	opened, err := s.Open(ctx, p, projectID, revisionID)
	if err != nil {
		return OpenResult{}, nil, err
	}
	drama, err := s.readDramaRevision(ctx, p, opened.Manifest, revisionID)
	return opened, drama, err
}
func (s *Service) PublicationReceipt(ctx context.Context, p Principal, r PublicationReceiptRequest) (WorldStatus, error) {
	if !p.Valid() {
		return WorldStatus{}, ErrForbidden
	}
	if !r.valid() {
		return WorldStatus{}, ErrInvalid
	}
	if err := writable(ctx, s.db, p, r.ProjectID); err != nil {
		return WorldStatus{}, err
	}
	result, err := publicationReceipt(ctx, p, r, s.readPublicationSnapshot, s.assets)
	if err != nil {
		return WorldStatus{}, err
	}
	// A completed receipt read is not a new write authorization. Recheck now;
	// the later Project Commit independently checks membership and the receipt.
	if err = writable(ctx, s.db, p, r.ProjectID); err != nil {
		return WorldStatus{}, err
	}
	return result, nil
}
func publicationReceipt(ctx context.Context, p Principal, r PublicationReceiptRequest, read publicationSnapshot, assets AssetResolver) (WorldStatus, error) {
	var out WorldStatus
	if !p.Valid() {
		return out, ErrForbidden
	}
	if !r.valid() || read == nil || assets == nil {
		return out, ErrInvalid
	}
	prepared, drama, err := read(ctx, p, r.ProjectID, r.PreparedRevisionID)
	if err != nil {
		return out, err
	}
	if prepared.ProjectID != r.ProjectID || prepared.RevisionID != r.PreparedRevisionID || prepared.Manifest.ProjectID != r.ProjectID || prepared.Manifest.Category != "ai_short_drama" {
		return out, ErrDependency
	}
	var candidate contentObject
	for _, value := range listOf(drama["world_candidates"]) {
		c := objectOf(value)
		if str(c["key"]) == r.CandidateKey {
			if candidate != nil {
				return out, ErrInvalid
			}
			candidate = c
		}
	}
	if candidate == nil || objectOf(candidate["review"])["state"] != "approved" || !ValidID(str(candidate["publication_record_file_id"])) {
		return out, ErrInvalid
	}
	var request WorldRegisterRequest
	raw, err := json.Marshal(candidate["registration_request"])
	if err != nil || strictjson.Decode(bytes.NewReader(raw), MaxManifestBytes, &request) != nil {
		return out, ErrInvalid
	}
	request, err = normalizeWorld(request)
	if err != nil || request.CommitID != r.CommitID || request.SourceProjectID != r.ProjectID || request.ReviewFileID != str(objectOf(candidate["review"])["record_file_id"]) || request.AssetID != "" && request.AssetID != r.AssetID {
		return out, ErrInvalid
	}
	source, _, err := read(ctx, p, r.ProjectID, request.SourceRevisionID)
	if err != nil {
		return out, err
	}
	if source.ProjectID != r.ProjectID || source.RevisionID != request.SourceRevisionID || source.Manifest.ProjectID != r.ProjectID || source.Manifest.Validate() != nil {
		return out, ErrDependency
	}
	receipt, err := assets.Receipt(ctx, p, r.AssetID, r.CommitID)
	if err != nil {
		return out, err
	}
	hash, err := digest(request)
	if err != nil {
		return out, err
	}
	if receipt.Hash != hash {
		return out, ErrInvalid
	}
	if receipt.State != "committed" {
		return out, ErrNotReady
	}
	result := receipt.Result
	if !ValidID(receipt.Actor) || receipt.Version != r.VersionID || result == nil || result.AssetID != r.AssetID || result.VersionID != r.VersionID || !result.ManifestRef.Valid() || result.Manifest.Validate() != nil {
		return out, ErrInvalid
	}
	manifest := result.Manifest
	if manifest.AssetID != r.AssetID || manifest.Name != request.Name || manifest.AssetType != request.AssetType || !contentEqual(manifest.Subjects, request.Subjects) || !contentEqual(manifest.DependsOn, *request.DependsOn) || manifest.Source != (AssetSource{ProjectID: r.ProjectID, RevisionID: request.SourceRevisionID, Relation: request.Relation, ReviewFileID: request.ReviewFileID}) || len(manifest.Files) != len(request.FileIDs) {
		return out, ErrInvalid
	}
	files := map[string]File{}
	for _, f := range source.Manifest.Files {
		files[f.ID] = f
	}
	selected := map[string]bool{}
	for _, id := range request.FileIDs {
		selected[id] = true
	}
	for _, f := range manifest.Files {
		if !selected[f.ID] || files[f.ID] != f {
			return out, ErrInvalid
		}
		delete(selected, f.ID)
	}
	if len(selected) != 0 {
		return out, ErrInvalid
	}
	return WorldStatus{State: "committed", RequestHash: hash, Result: result}, nil
}
