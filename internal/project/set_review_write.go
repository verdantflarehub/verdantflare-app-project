package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Service) ReviewAppend(ctx context.Context, p Principal, r ReviewAppendRequest) (ReviewAppendResult, error) {
	var out ReviewAppendResult
	if !r.TargetLocator.valid() || r.TargetLocator.StateKey == "" || r.TargetLocator.VersionKey == "" || !reviewHashPattern.MatchString(r.ExpectedTargetHash) || !ValidID(r.CommitID) || !ValidID(r.ExpectedRevisionID) || r.ExpectedRecordID != nil && !ValidID(*r.ExpectedRecordID) || !reviewBodyValid(r.Body) {
		return out, ErrInvalid
	}
	supersedes, err := r.supersedes()
	if err != nil {
		return out, err
	}
	if r.Action != "comment" && r.Action != "request_changes" && r.Action != "approve" {
		return out, ErrInvalid
	}
	if err := writable(ctx, s.db, p, r.TargetLocator.ProjectID); err != nil {
		return out, err
	}
	if reviewOrigin(p) != "human_interactive" && r.Action != "comment" {
		return out, ErrForbidden
	}
	requestJSON, hash, err := reviewJSON(r)
	if err != nil || len(requestJSON) > 1<<20 {
		return out, ErrInvalid
	}
	prior, err := loadCommit(ctx, s.db, p.OrganizationID, r.TargetLocator.ProjectID, r.CommitID)
	if err == nil {
		if prior.Actor != p.SubjectID {
			return out, ErrForbidden
		}
		if prior.Hash != hash || prior.Plan.Review == nil || prior.Plan.Review.Record.Actor.Origin != reviewOrigin(p) {
			return out, ErrIdempotency
		}
		availability, e := s.reviewAvailability(ctx, p, prior.Plan.Review.Record.TargetManifest)
		if e != nil {
			return out, e
		}
		if len(availability) > 0 {
			return out, ErrForbidden
		}
		result, e := s.resume(ctx, p, r.TargetLocator.ProjectID, r.CommitID)
		if e != nil {
			return out, e
		}
		if e = s.reviewResultAuthorization(ctx, p, prior.Plan.Review.Record.TargetManifest); e != nil {
			return out, e
		}
		return reviewAppendResult(prior, result), nil
	}
	if !errors.Is(err, ErrNotFound) {
		return out, err
	}
	target, production, legacy, err := s.resolveReviewTarget(ctx, p, r.TargetLocator, r.ExpectedTargetHash)
	if err != nil {
		return out, err
	}
	if legacy {
		return out, ErrLegacyTarget
	}
	availability, err := s.reviewAvailability(ctx, p, target)
	if err != nil {
		return out, err
	}
	if len(availability) > 0 {
		return out, ErrForbidden
	}
	if err = validateReviewScope(target, r.Scope, r.Action); err != nil {
		return out, err
	}
	ids := []string{}
	for _, e := range target.Evidence {
		ids = append(ids, e.EvidenceID)
	}
	if _, err = reviewIDs(r.technicalRefs()); err != nil || !subsetReview(r.technicalRefs(), ids) {
		return out, ErrInvalid
	}
	indexes, err := s.reviewIndexes(ctx, p, r.TargetLocator.ProjectID, r.ExpectedTargetHash, nil)
	if err != nil {
		return out, err
	}
	history, head := reviewSummary(indexes)
	if optionalID(head) != optionalID(r.ExpectedRecordID) {
		return out, ErrReviewConflict
	}
	if err = validateReviewDecision(history, r); err != nil {
		return out, err
	}
	document, err := s.publicationDocument(ctx, p, r.TargetLocator.ProjectID, r.ExpectedRevisionID)
	if err != nil {
		return out, err
	}
	if document.Opened.HeadRevisionID != r.ExpectedRevisionID {
		return out, &ConflictError{CurrentRevisionID: document.Opened.HeadRevisionID}
	}
	currentLocator := target.TargetLocator
	currentLocator.ContentRevisionID = r.ExpectedRevisionID
	_, currentProduction, currentErr := s.buildReviewTarget(ctx, p, document, currentLocator)
	projection := currentErr == nil && currentProduction == production
	if currentErr != nil && !errors.Is(currentErr, ErrNotFound) && !errors.Is(currentErr, ErrLegacyTarget) {
		return out, currentErr
	}
	scope, err := normalizeReviewScope(r.Scope)
	if err != nil {
		return out, err
	}
	record := ReviewRecord{SchemaVersion: 1, RecordID: newID(), TargetHash: r.ExpectedTargetHash, TargetManifest: target, PreviousRecordID: r.ExpectedRecordID, Action: r.Action, Scope: scope, Body: r.Body, Actor: ReviewActor{SubjectID: p.SubjectID, OrganizationID: p.OrganizationID, Origin: reviewOrigin(p)}, CreatedAt: time.Now().UTC(), SupersedesRecordID: supersedes, TechnicalRefs: append([]string{}, r.technicalRefs()...)}
	state, _ := reviewProjection(append(history, record))
	recordJSON, recordHash, err := reviewJSON(record)
	if err != nil || len(recordJSON) > MaxManifestBytes {
		return out, ErrTooLarge
	}
	plan, err := prepareReview(document, r, record, string(recordJSON), recordHash, production, state, projection)
	if err != nil {
		return out, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(context.Background())
	if err = advisory(ctx, tx, "commit:"+p.OrganizationID+":"+r.TargetLocator.ProjectID+":"+r.CommitID); err != nil {
		return out, err
	}
	if err = lockActive(ctx, tx, p, r.TargetLocator.ProjectID); err != nil {
		return out, err
	}
	if err = writable(ctx, tx, p, r.TargetLocator.ProjectID); err != nil {
		return out, err
	}
	c, err := loadCommit(ctx, tx, p.OrganizationID, r.TargetLocator.ProjectID, r.CommitID)
	if err == nil {
		if c.Actor != p.SubjectID {
			return out, ErrForbidden
		}
		if c.Hash != hash || c.Plan.Review == nil || c.Plan.Review.Record.Actor.Origin != reviewOrigin(p) {
			return out, ErrIdempotency
		}
	} else if errors.Is(err, ErrNotFound) {
		c = commitRecord{ProjectID: r.TargetLocator.ProjectID, CommitID: r.CommitID, Actor: p.SubjectID, Hash: hash, Expected: r.ExpectedRevisionID, Revision: newID(), Plan: plan}
		if err = s.insertCommit(ctx, tx, p, c); err != nil {
			return out, err
		}
	} else {
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return out, err
	}
	result, err := s.resume(ctx, p, r.TargetLocator.ProjectID, r.CommitID)
	if err != nil {
		return out, err
	}
	if err = s.reviewResultAuthorization(ctx, p, c.Plan.Review.Record.TargetManifest); err != nil {
		return out, err
	}
	return reviewAppendResult(c, result), nil
}
func prepareReview(document publicationDocument, r ReviewAppendRequest, record ReviewRecord, body, recordHash, production, state string, projection bool) (preparePlan, error) {
	raw, err := json.Marshal(document.Opened.Manifest)
	if err != nil {
		return preparePlan{}, ErrInvalid
	}
	var manifest Manifest
	if json.Unmarshal(raw, &manifest) != nil {
		return preparePlan{}, ErrInvalid
	}
	fileID := newID()
	manifest.Files = append(manifest.Files, File{ID: fileID, Path: "reviews/set/" + record.RecordID + ".json", Role: "record"})
	writes := []TextWrite{{WriteID: newID(), FileID: fileID, MIME: "application/json", Text: body}}
	pending := map[string]bool{fileID: true}
	if projection {
		root, e := json.Marshal(document.Root)
		if e != nil {
			return preparePlan{}, ErrInvalid
		}
		var copy contentObject
		if json.Unmarshal(root, &copy) != nil {
			return preparePlan{}, ErrInvalid
		}
		document.Root = copy
		material, _, version, e := reviewMaterial(document, r.TargetLocator)
		if e != nil {
			return preparePlan{}, e
		}
		review := contentObject{"state": state, "record_file_id": fileID}
		version["review"] = review
		selected := listOf(material["selected_versions"])
		if len(selected) == 1 {
			selection := objectOf(selected[0])
			if selection["state_key"] == r.TargetLocator.StateKey && selection["version_key"] == r.TargetLocator.VersionKey {
				material["review"] = review
			}
		}
		data, e := json.Marshal(document.Root)
		if e != nil || len(data) > MaxManifestBytes {
			return preparePlan{}, ErrTooLarge
		}
		writes = append(writes, TextWrite{WriteID: newID(), FileID: document.File.ID, MIME: "application/json", Text: string(data)})
		pending[document.File.ID] = true
		for i, f := range manifest.Files {
			if f.ID == document.File.ID {
				manifest.Files[i].Content = ContentRef{}
			}
		}
	}
	manifest, invalid, err := reconcileSelections(document.Opened.Manifest, manifest, nil, pending)
	if err != nil {
		return preparePlan{}, ErrInvalid
	}
	return preparePlan{Manifest: manifest, Writes: writes, ManifestWriteID: newID(), Refs: []ContentRef{}, Invalidated: invalid, Review: &reviewPlan{Request: r, Record: record, FileID: fileID, RecordSHA256: recordHash, ProductionHash: production, ReviewState: state, ProjectionApplied: projection}}, nil
}
func reviewAppendResult(c commitRecord, result Result) ReviewAppendResult {
	r := c.Plan.Review
	return ReviewAppendResult{State: "committed", CommitID: c.CommitID, RecordID: r.Record.RecordID, TargetHash: r.Record.TargetHash, ReviewState: r.ReviewState, RecordsHeadRecordID: r.Record.RecordID, RevisionID: result.RevisionID, ManifestRef: result.ManifestRef, ProjectionApplied: r.ProjectionApplied}
}
func (s *Service) ReviewStatus(ctx context.Context, p Principal, r StatusRequest) (ReviewStatusResult, error) {
	var out ReviewStatusResult
	if !ValidID(r.ProjectID) || !ValidID(r.CommitID) {
		return out, ErrInvalid
	}
	member, err := role(ctx, s.db, p, r.ProjectID)
	if err != nil {
		return out, err
	}
	c, err := loadCommit(ctx, s.db, p.OrganizationID, r.ProjectID, r.CommitID)
	if err != nil {
		return out, err
	}
	if c.Actor != p.SubjectID && member != "owner" {
		return out, ErrForbidden
	}
	if c.Plan.Review == nil {
		return out, ErrNotFound
	}
	availability, availabilityErr := s.reviewAvailability(ctx, p, c.Plan.Review.Record.TargetManifest)
	if availabilityErr != nil {
		return out, availabilityErr
	}
	if len(availability) > 0 {
		return out, ErrForbidden
	}
	out = ReviewStatusResult{State: c.State, RequestSHA256: c.Hash, ErrorCode: c.Error}
	if c.State == "failed" || c.State == "conflict" {
		out.State = "rejected"
	}
	if c.State == "committed" {
		if c.Result == nil {
			return out, ErrNotReady
		}
		value := reviewAppendResult(c, *c.Result)
		out.Result = &value
	}
	if err = s.reviewResultAuthorization(ctx, p, c.Plan.Review.Record.TargetManifest); err != nil {
		return ReviewStatusResult{}, err
	}
	if member, err = role(ctx, s.db, p, r.ProjectID); err != nil {
		return ReviewStatusResult{}, err
	}
	if c.Actor != p.SubjectID && member != "owner" {
		return ReviewStatusResult{}, ErrForbidden
	}
	return out, nil
}

func (s *Service) reviewResultAuthorization(ctx context.Context, p Principal, target ReviewTarget) error {
	if _, err := role(ctx, s.db, p, target.TargetLocator.ProjectID); err != nil {
		return err
	}
	issues, err := s.reviewDependencyAvailability(ctx, p, target)
	if err != nil {
		return err
	}
	if len(issues) > 0 {
		return ErrForbidden
	}
	return nil
}

// Called in the same transaction that publishes the project revision. The head
// CAS and immutable index cannot be visible without the artifact-bearing revision.
func (s *Service) publishReview(ctx context.Context, tx pgx.Tx, p Principal, c commitRecord) error {
	if c.Plan.Review == nil {
		return nil
	}
	r := c.Plan.Review
	record := r.Record
	var head string
	var sequence int64
	err := tx.QueryRow(ctx, `SELECT record_id::text,sequence_no FROM studio.project_set_review_heads WHERE organization_id=$1 AND project_id=$2 AND target_hash=$3 FOR UPDATE`, p.OrganizationID, c.ProjectID, record.TargetHash).Scan(&head, &sequence)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if head != optionalID(record.PreviousRecordID) {
		return ErrReviewConflict
	}
	targetJSON, hash, err := reviewJSON(record.TargetManifest)
	if err != nil || hash != record.TargetHash {
		return ErrTargetMismatch
	}
	l := record.TargetManifest.TargetLocator
	_, err = tx.Exec(ctx, `INSERT INTO studio.project_set_review_targets(organization_id,project_id,target_hash,material_key,state_key,version_key,content_revision_id,production_hash,target_json,first_revision_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(organization_id,project_id,target_hash) DO NOTHING`, p.OrganizationID, c.ProjectID, record.TargetHash, l.MaterialKey, l.StateKey, l.VersionKey, l.ContentRevisionID, r.ProductionHash, string(targetJSON), c.Revision)
	if err != nil {
		return err
	}
	scope, scopeHash, err := reviewJSON(record.Scope)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO studio.project_set_review_records(organization_id,project_id,target_hash,record_id,previous_record_id,sequence_no,revision_id,file_id,commit_id,record_sha256,scope_json,scope_hash,action,origin,created_at) VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, p.OrganizationID, c.ProjectID, record.TargetHash, record.RecordID, optionalID(record.PreviousRecordID), sequence+1, c.Revision, r.FileID, c.CommitID, r.RecordSHA256, string(scope), scopeHash, record.Action, record.Actor.Origin, record.CreatedAt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO studio.project_set_review_heads(organization_id,project_id,target_hash,record_id,sequence_no) VALUES($1,$2,$3,$4,$5) ON CONFLICT(organization_id,project_id,target_hash) DO UPDATE SET record_id=EXCLUDED.record_id,sequence_no=EXCLUDED.sequence_no`, p.OrganizationID, c.ProjectID, record.TargetHash, record.RecordID, sequence+1)
	return err
}

func protectedReviewChange(previous, current contentObject) bool {
	if contentEqual(previous, current) {
		return false
	}
	if str(previous["record_file_id"]) != "" {
		return true
	}
	return str(current["state"]) != "unreviewed" || str(current["record_file_id"]) != ""
}

// Generic commit/edit cannot create approvals or write the reserved record
// namespace. Existing legacy facts can be retained, but not forged anew.
func (s *Service) guardSetReviews(ctx context.Context, p Principal, c commitRecord) error {
	var old Manifest
	var err error
	if c.Expected != "" {
		old, _, err = revision(ctx, s.db, p.OrganizationID, c.ProjectID, c.Expected)
		if err != nil {
			return err
		}
	}
	if err := guardReviewFiles(old, c.Plan); err != nil {
		return err
	}
	if old.Category != "ai_short_drama" && c.Plan.Manifest.Category != "ai_short_drama" {
		return nil
	}
	beforeFiles := map[string]File{}
	for _, f := range old.Files {
		beforeFiles[f.ID] = f
	}
	var prior contentObject
	if c.Expected != "" {
		prior, err = s.readDramaRevision(ctx, p, old, c.Expected)
		if err != nil {
			return err
		}
	}
	var current contentObject
	for _, d := range c.Plan.Manifest.DomainDocuments {
		if d.Type != "project-content" {
			continue
		}
		for _, f := range c.Plan.Manifest.Files {
			if f.ID != d.FileID {
				continue
			}
			access := Access{}
			if beforeFiles[f.ID].Content == f.Content {
				access = Access{ProjectID: c.ProjectID, RevisionID: c.Expected}
			}
			raw, e := s.content.Read(ctx, p, f.Content, access, MaxManifestBytes)
			if e != nil {
				return e
			}
			var root contentObject
			if json.Unmarshal(raw, &root) != nil {
				return ErrInvalid
			}
			current = objectOf(root["ai_short_drama"])
		}
	}
	oldOwners, newOwners := dramaOwners(prior), dramaOwners(current)
	for path, owner := range newOwners {
		if owner["kind"] != "location" {
			continue
		}
		previous := oldOwners[path]
		if protectedReviewChange(objectOf(previous["review"]), objectOf(owner["review"])) {
			if c.Plan.Review == nil || !c.Plan.Review.ProjectionApplied || path != "globals/"+c.Plan.Review.Record.TargetManifest.TargetLocator.MaterialKey || str(objectOf(owner["review"])["record_file_id"]) != c.Plan.Review.FileID || str(objectOf(owner["review"])["state"]) != c.Plan.Review.ReviewState {
				return ErrForbidden
			}
		}
		oldVersions, _ := indexObjects(previous["versions"], path)
		for _, value := range listOf(owner["versions"]) {
			version := objectOf(value)
			oldVersion := oldVersions[str(version["key"])]
			if !protectedReviewChange(objectOf(oldVersion["review"]), objectOf(version["review"])) {
				continue
			}
			r := c.Plan.Review
			if r == nil || !r.ProjectionApplied || path != "globals/"+r.Record.TargetManifest.TargetLocator.MaterialKey || version["key"] != r.Record.TargetManifest.TargetLocator.VersionKey || str(objectOf(version["review"])["record_file_id"]) != r.FileID || str(objectOf(version["review"])["state"]) != r.ReviewState {
				return ErrForbidden
			}
		}
	}
	if c.Plan.Review != nil {
		r := c.Plan.Review
		_, hash, e := reviewJSON(r.Request)
		if e != nil || hash != c.Hash || r.Record.Actor.SubjectID != c.Actor || r.Record.Actor.OrganizationID != p.OrganizationID {
			return ErrForbidden
		}
		if r.Record.Actor.Origin != "human_interactive" && r.Record.Action != "comment" {
			return ErrForbidden
		}
		found := false
		for _, w := range c.Plan.Writes {
			if w.FileID == r.FileID {
				if found {
					return ErrTargetMismatch
				}
				found = true
				_, recordHash, e := reviewJSON(r.Record)
				actual := sha256.Sum256([]byte(w.Text))
				if e != nil || recordHash != r.RecordSHA256 || hex.EncodeToString(actual[:]) != r.RecordSHA256 || w.MIME != "application/json" {
					return ErrTargetMismatch
				}
			}
		}
		if !found {
			return ErrTargetMismatch
		}
	}
	return nil
}

func guardReviewFiles(old Manifest, plan preparePlan) error {
	beforeFiles := map[string]File{}
	managed := false
	for _, file := range old.Files {
		beforeFiles[file.ID] = file
		managed = managed || strings.HasPrefix(strings.ToLower(file.Path), "reviews/set/")
	}
	if managed && old.Category != plan.Manifest.Category {
		return ErrForbidden
	}
	for _, f := range plan.Manifest.Files {
		if strings.HasPrefix(strings.ToLower(f.Path), "reviews/set/") {
			oldFile, exists := beforeFiles[f.ID]
			if exists {
				if oldFile != f {
					return ErrForbidden
				}
			} else if plan.Manifest.Category != "ai_short_drama" || plan.Review == nil || plan.Review.FileID != f.ID || f.Path != "reviews/set/"+plan.Review.Record.RecordID+".json" {
				return ErrForbidden
			}
		}
	}
	for _, f := range old.Files {
		if strings.HasPrefix(strings.ToLower(f.Path), "reviews/set/") {
			found := false
			for _, next := range plan.Manifest.Files {
				if next == f {
					found = true
				}
			}
			if !found {
				return ErrForbidden
			}
		}
	}
	return nil
}
