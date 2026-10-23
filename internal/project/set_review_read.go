package project

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

type reviewIndex struct {
	Record     ReviewRecord
	FileID     string
	RevisionID string
	SHA256     string
	Sequence   int64
}
type reviewCursor struct {
	OrganizationID string `json:"organization_id"`
	SubjectID      string `json:"subject_id"`
	ProjectID      string `json:"project_id"`
	TargetHash     string `json:"target_hash"`
	HeadRecordID   string `json:"head_record_id"`
	RevisionID     string `json:"records_revision_id"`
	Offset         int    `json:"offset"`
}

func (s *Service) storedReviewTarget(ctx context.Context, p Principal, l ReviewLocator, hash string) (ReviewTarget, string, error) {
	var raw, production string
	err := s.db.QueryRow(ctx, `SELECT target_json,production_hash FROM studio.project_set_review_targets WHERE organization_id=$1 AND project_id=$2 AND target_hash=$3`, p.OrganizationID, l.ProjectID, hash).Scan(&raw, &production)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewTarget{}, "", ErrNotFound
	}
	if err != nil {
		return ReviewTarget{}, "", err
	}
	var target ReviewTarget
	if json.Unmarshal([]byte(raw), &target) != nil {
		return target, "", ErrNotReady
	}
	_, actual, err := reviewJSON(target)
	if err != nil || actual != hash || target.TargetLocator != l {
		return target, "", ErrTargetMismatch
	}
	return target, production, nil
}
func (s *Service) resolveReviewTarget(ctx context.Context, p Principal, l ReviewLocator, hash string) (ReviewTarget, string, bool, error) {
	if hash != "" {
		target, production, err := s.storedReviewTarget(ctx, p, l, hash)
		if err == nil {
			if _, err = s.Open(ctx, p, l.ProjectID, l.ContentRevisionID); err != nil {
				return target, "", false, err
			}
			return target, production, false, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return target, "", false, err
		}
	}
	document, err := s.publicationDocument(ctx, p, l.ProjectID, l.ContentRevisionID)
	if err != nil {
		return ReviewTarget{}, "", false, err
	}
	target, production, err := s.buildReviewTarget(ctx, p, document, l)
	if errors.Is(err, ErrLegacyTarget) {
		return target, production, true, nil
	}
	if err != nil {
		return target, "", false, err
	}
	{
		var stored, priorProduction string
		err = s.db.QueryRow(ctx, `SELECT target_json,production_hash FROM studio.project_set_review_targets WHERE organization_id=$1 AND project_id=$2 AND material_key=$3 AND state_key=$4 AND version_key=$5 ORDER BY first_revision_id LIMIT 1`, p.OrganizationID, l.ProjectID, l.MaterialKey, l.StateKey, l.VersionKey).Scan(&stored, &priorProduction)
		if err == nil {
			if priorProduction != production {
				return target, "", false, ErrTargetMismatch
			}
			if json.Unmarshal([]byte(stored), &target) != nil {
				return target, "", false, ErrNotReady
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return target, "", false, err
		}
	}
	_, actual, err := reviewJSON(target)
	if err != nil {
		return target, "", false, err
	}
	if hash != "" && actual != hash {
		return target, "", false, ErrTargetMismatch
	}
	return target, production, false, nil
}
func (s *Service) reviewIndexes(ctx context.Context, p Principal, project, hash string, manifest *Manifest) ([]reviewIndex, error) {
	rows, err := s.db.Query(ctx, `SELECT record_id::text,COALESCE(previous_record_id::text,''),sequence_no,revision_id::text,file_id::text,record_sha256,scope_json,action,origin,created_at FROM studio.project_set_review_records WHERE organization_id=$1 AND project_id=$2 AND target_hash=$3 ORDER BY sequence_no`, p.OrganizationID, project, hash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	visible := map[string]bool{}
	if manifest != nil {
		for _, f := range manifest.Files {
			visible[f.ID] = true
		}
	}
	out := []reviewIndex{}
	var previous string
	for rows.Next() {
		var item reviewIndex
		var parent, scope string
		if err = rows.Scan(&item.Record.RecordID, &parent, &item.Sequence, &item.RevisionID, &item.FileID, &item.SHA256, &scope, &item.Record.Action, &item.Record.Actor.Origin, &item.Record.CreatedAt); err != nil {
			return nil, err
		}
		if manifest != nil && !visible[item.FileID] {
			continue
		}
		if parent != previous || item.Sequence != int64(len(out)+1) || json.Unmarshal([]byte(scope), &item.Record.Scope) != nil {
			return nil, ErrNotReady
		}
		if parent != "" {
			copy := parent
			item.Record.PreviousRecordID = &copy
		}
		item.Record.TargetHash = hash
		out = append(out, item)
		previous = item.Record.RecordID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i := range out {
		item := &out[i]
		published, _, readErr := revision(ctx, s.db, p.OrganizationID, project, item.RevisionID)
		if readErr != nil {
			return nil, readErr
		}
		var file File
		for _, f := range published.Files {
			if f.ID == item.FileID {
				file = f
			}
		}
		if !file.Content.Valid() {
			return nil, ErrNotReady
		}
		raw, readErr := s.content.Read(ctx, p, file.Content, Access{ProjectID: project, RevisionID: item.RevisionID}, MaxManifestBytes)
		if readErr != nil {
			return nil, readErr
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != item.SHA256 {
			return nil, ErrTargetMismatch
		}
		var full ReviewRecord
		if json.Unmarshal(raw, &full) != nil || full.RecordID != item.Record.RecordID || full.TargetHash != hash || full.Actor.OrganizationID != p.OrganizationID || full.Actor.Origin != item.Record.Actor.Origin || full.Action != item.Record.Action || optionalID(full.PreviousRecordID) != optionalID(item.Record.PreviousRecordID) || !contentEqual(full.Scope, item.Record.Scope) {
			return nil, ErrTargetMismatch
		}
		_, actualHash, readErr := reviewJSON(full.TargetManifest)
		if readErr != nil || actualHash != hash {
			return nil, ErrTargetMismatch
		}
		item.Record = full
	}
	return out, nil
}
func reviewSummary(indexes []reviewIndex) ([]ReviewRecord, *string) {
	records := make([]ReviewRecord, 0, len(indexes))
	for _, item := range indexes {
		records = append(records, item.Record)
	}
	if len(records) == 0 {
		return records, nil
	}
	id := records[len(records)-1].RecordID
	return records, &id
}
func (s *Service) ReviewOpen(ctx context.Context, p Principal, r ReviewOpenRequest) (ReviewOpenResult, error) {
	out := ReviewOpenResult{TargetLocator: r.TargetLocator, Availability: []ReviewIssue{}, MappingState: "mapped", ReviewState: "unreviewed", ScopeDecisions: []ReviewDecision{}, Records: []any{}, Missing: []ReviewIssue{}, Capabilities: ReviewCapabilities{ReasonCodes: []string{}}}
	if !r.TargetLocator.valid() || r.ExpectedTargetHash != "" && !reviewHashPattern.MatchString(r.ExpectedTargetHash) || r.RecordsRevisionID != "" && !ValidID(r.RecordsRevisionID) || len(r.Cursor) > 4096 {
		return out, ErrInvalid
	}
	member, err := role(ctx, s.db, p, r.TargetLocator.ProjectID)
	if err != nil {
		return out, err
	}
	var cursor reviewCursor
	if r.Cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(r.Cursor)
		if e != nil || decodeReview(raw, &cursor) != nil || cursor.OrganizationID != p.OrganizationID || cursor.SubjectID != p.SubjectID || cursor.ProjectID != r.TargetLocator.ProjectID || !ValidID(cursor.RevisionID) || cursor.Offset < 1 || cursor.Offset > MaxFiles {
			return out, ErrInvalid
		}
		if r.RecordsRevisionID != "" && r.RecordsRevisionID != cursor.RevisionID {
			return out, ErrReviewConflict
		}
		r.RecordsRevisionID = cursor.RevisionID
	}
	current, err := s.Open(ctx, p, r.TargetLocator.ProjectID, r.RecordsRevisionID)
	if err != nil {
		return out, err
	}
	out.HeadRevisionID = current.HeadRevisionID
	out.RecordsRevisionID = current.RevisionID
	target, _, legacy, err := s.resolveReviewTarget(ctx, p, r.TargetLocator, r.ExpectedTargetHash)
	if err != nil {
		return out, err
	}
	out.TargetLocator = target.TargetLocator
	view := cloneReviewTarget(target)
	out.TargetView = &view
	availability, err := s.reviewAvailability(ctx, p, target)
	if err != nil {
		return out, err
	}
	out.Availability = availability
	if legacy {
		out.MappingState = "legacy_unmapped"
		out.Missing = append(out.Missing, ReviewIssue{Code: "LEGACY_TARGET_UNMAPPED"})
		out.Capabilities.ReasonCodes = append(out.Capabilities.ReasonCodes, "LEGACY_TARGET_UNMAPPED")
		if len(availability) > 0 {
			restrictReviewResult(&out, nil)
		}
		if _, err = role(ctx, s.db, p, r.TargetLocator.ProjectID); err != nil {
			return ReviewOpenResult{}, err
		}
		return out, nil
	}
	_, hash, err := reviewJSON(target)
	if err != nil {
		return out, err
	}
	out.TargetHash = &hash
	for _, required := range target.Requirements {
		if required.State != "satisfied" {
			out.Missing = append(out.Missing, ReviewIssue{Code: required.ReasonCode})
		}
	}
	indexes, err := s.reviewIndexes(ctx, p, r.TargetLocator.ProjectID, hash, &current.Manifest)
	if err != nil {
		return out, err
	}
	summary, head := reviewSummary(indexes)
	out.RecordsHeadRecordID = head
	out.ReviewState, out.ScopeDecisions = reviewProjection(summary)
	if r.Cursor != "" && (cursor.TargetHash != hash || cursor.HeadRecordID != optionalID(head) || cursor.Offset >= len(indexes)) {
		return out, ErrReviewConflict
	}
	if len(availability) > 0 {
		// The canonical bytes never change. A restricted response is deliberately
		// incomplete and cannot be used to recompute the target digest.
		restrictReviewResult(&out, summary)
		if _, err = role(ctx, s.db, p, r.TargetLocator.ProjectID); err != nil {
			return ReviewOpenResult{}, err
		}
		return out, nil
	}
	out.TargetManifest = &target
	start := cursor.Offset
	end := start + 50
	if end > len(indexes) {
		end = len(indexes)
	}
	for _, index := range indexes[start:end] {
		out.Records = append(out.Records, index.Record)
	}
	if end < len(indexes) {
		next := reviewCursor{OrganizationID: p.OrganizationID, SubjectID: p.SubjectID, ProjectID: r.TargetLocator.ProjectID, TargetHash: hash, HeadRecordID: optionalID(head), RevisionID: current.RevisionID, Offset: end}
		raw, _ := json.Marshal(next)
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	finalAvailability, err := s.reviewDependencyAvailability(ctx, p, target)
	if err != nil {
		return ReviewOpenResult{}, err
	}
	// Use the final role after all potentially slow dependency/artifact reads.
	if member, err = role(ctx, s.db, p, r.TargetLocator.ProjectID); err != nil {
		return ReviewOpenResult{}, err
	}
	if len(finalAvailability) > 0 {
		out.Availability = append(out.Availability, finalAvailability...)
		restrictReviewResult(&out, summary)
		return out, nil
	}
	out.Capabilities.CanComment = member == "owner" || member == "editor"
	out.Capabilities.CanDecide = out.Capabilities.CanComment && reviewOrigin(p) == "human_interactive"
	if !out.Capabilities.CanComment {
		out.Capabilities.ReasonCodes = append(out.Capabilities.ReasonCodes, "READ_ONLY")
	}
	if reviewOrigin(p) != "human_interactive" {
		out.Capabilities.ReasonCodes = append(out.Capabilities.ReasonCodes, "AGENT_SUGGESTIONS_ONLY")
	}
	return out, nil
}

func restrictReviewResult(out *ReviewOpenResult, records []ReviewRecord) {
	out.TargetView = nil
	out.TargetManifest = nil
	out.ScopeDecisions = []ReviewDecision{}
	out.Records = []any{}
	out.NextCursor = ""
	out.Capabilities = ReviewCapabilities{ReasonCodes: []string{"RESTRICTED"}}
	for i := range out.Availability {
		out.Availability[i].EvidenceID = ""
	}
	for _, record := range records {
		out.Records = append(out.Records, map[string]any{"record_id": record.RecordID, "visibility": "restricted", "record_view": nil})
	}
}
