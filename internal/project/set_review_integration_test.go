//go:build integration

package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/verdantflarehub/verdantflare-app-project/internal/coordination"
	"github.com/verdantflarehub/verdantflare-app-project/internal/testdb"
	"github.com/verdantflarehub/verdantflare-app-project/migrations"
)

// The real database transaction, migration guards, commit recovery and CAS are
// exercised only against the explicitly configured disposable 5090 dev database.
// Artifact is an in-memory transport fixture, not cross-service acceptance.
type reviewIntegrationArtifact struct{ *integrationArtifact }

func (a *reviewIntegrationArtifact) Metadata(ctx context.Context, p Principal, ref ContentRef, access Access) (ContentVersion, error) {
	v, err := a.integrationArtifact.Metadata(ctx, p, ref, access)
	if err != nil {
		return v, err
	}
	raw := a.bytes[ref]
	sum := sha256.Sum256(raw)
	v.SHA256, v.Size = hex.EncodeToString(sum[:]), int64(len(raw))
	return v, nil
}

func (a *reviewIntegrationArtifact) Write(ctx context.Context, p Principal, project string, w TextWrite) (ContentVersion, error) {
	v, err := a.integrationArtifact.Write(ctx, p, project, w)
	if err != nil {
		return v, err
	}
	return a.Metadata(ctx, p, v.ContentRef, Access{})
}

func TestSetReviewDurableRecoveryCASAndGuard(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if err := migrations.ApplyFresh(ctx, db, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := migrations.ActivateFresh(ctx, db); err != nil {
		t.Fatal(err)
	}
	f := newPublicationFixture(t)
	f.principal.ReviewOrigin = "human_interactive"
	f.data[f.source.ManifestRef] = publicationJSON(t, f.source.Manifest)
	content := &reviewIntegrationArtifact{&integrationArtifact{versions: map[ContentRef]ContentVersion{}, bytes: f.data}}
	for ref := range f.data {
		mime := "application/json"
		if ref == f.source.Manifest.Files[1].Content {
			mime = "image/png"
		}
		if ref == f.source.Manifest.Files[4].Content {
			mime = "text/markdown"
		}
		content.versions[ref] = ContentVersion{ContentRef: ref, OrganizationID: f.principal.OrganizationID, Source: Source{Kind: "user_edit", ProjectID: f.source.ProjectID}, MIME: mime}
	}
	if _, err := db.Exec(ctx, "INSERT INTO studio.projects(project_id,organization_id,owner_id,name,category,status) VALUES($1,$2,$3,$4,$5,'active')", f.source.ProjectID, f.principal.OrganizationID, f.principal.SubjectID, f.source.Manifest.Name, f.source.Manifest.Category); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO studio.project_members(organization_id,project_id,subject_id,role) VALUES($1,$2,$3,'owner')", f.principal.OrganizationID, f.source.ProjectID, f.principal.SubjectID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO studio.project_commits(organization_id,project_id,commit_id,created_by,request_sha256,revision_id,state,plan,result) VALUES($1,$2,$3,$4,$5,$6,'committed','{}',$7)", f.principal.OrganizationID, f.source.ProjectID, id(90), f.principal.SubjectID, strings.Repeat("a", 64), f.source.RevisionID, publicationJSON(t, f.source.Result)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO studio.project_revisions(revision_id,organization_id,project_id,manifest_ref,manifest_json,created_by,commit_id) VALUES($1,$2,$3,$4,$5,$6,$7)", f.source.RevisionID, f.principal.OrganizationID, f.source.ProjectID, publicationJSON(t, f.source.ManifestRef), publicationJSON(t, f.source.Manifest), f.principal.SubjectID, id(90)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "UPDATE studio.projects SET head_revision_id=$1 WHERE project_id=$2", f.source.RevisionID, f.source.ProjectID); err != nil {
		t.Fatal(err)
	}
	newService := func() *Service {
		t.Helper()
		store, err := coordination.NewStore(db, "project", 1, AuthorizeReservation)
		if err != nil {
			t.Fatal(err)
		}
		engine := &coordination.Engine{Store: store, Peers: map[string]coordination.Peer{"world": &integrationPeer{}}}
		s, err := NewService(ctx, db, content, integrationAssets{}, engine)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := newService()
	r := reviewTestRequest()
	r.TargetLocator.ProjectID, r.TargetLocator.ContentRevisionID, r.ExpectedRevisionID = f.source.ProjectID, f.source.RevisionID, f.source.RevisionID
	r.CommitID = id(400)
	opened, err := s.ReviewOpen(ctx, f.principal, ReviewOpenRequest{TargetLocator: r.TargetLocator})
	if err != nil || opened.TargetHash == nil || !opened.Capabilities.CanDecide {
		t.Fatal("open exact target", err)
	}
	r.ExpectedTargetHash = *opened.TargetHash
	content.fail = true
	if _, err := s.ReviewAppend(ctx, f.principal, r); !errors.Is(err, ErrDependency) {
		t.Fatal("write failure did not preserve preparation", err)
	}
	prepared, err := loadCommit(ctx, db, f.principal.OrganizationID, f.source.ProjectID, r.CommitID)
	if err != nil || prepared.State != "preparing" || prepared.Plan.Review == nil {
		t.Fatal("missing durable review intent", err)
	}
	recordID, fileID := prepared.Plan.Review.Record.RecordID, prepared.Plan.Review.FileID
	status, err := s.ReviewStatus(ctx, f.principal, StatusRequest{ProjectID: f.source.ProjectID, CommitID: r.CommitID})
	if err != nil || status.State != "preparing" {
		t.Fatal("preparing status", err)
	}
	content.fail = false
	s = newService()
	if err := s.RecoverOnce(ctx); err != nil {
		t.Fatal("recovery", err)
	}
	result, err := s.ReviewAppend(ctx, f.principal, r)
	if err != nil || result.RecordID != recordID || !result.ProjectionApplied || result.ReviewState != "in_review" {
		t.Fatal("recovered review changed intent", err)
	}
	after, err := loadCommit(ctx, db, f.principal.OrganizationID, f.source.ProjectID, r.CommitID)
	if err != nil || after.Plan.Review.FileID != fileID || after.Plan.Review.Record.Actor.Origin != "human_interactive" {
		t.Fatal("recovery lost authenticated origin or file ID", err)
	}
	changed := r
	changed.Body += " altered"
	if _, err := s.ReviewAppend(ctx, f.principal, changed); !errors.Is(err, ErrIdempotency) {
		t.Fatal("idempotency conflict missing", err)
	}
	stale := r
	stale.CommitID, stale.ExpectedRevisionID = id(401), result.RevisionID
	if _, err := s.ReviewAppend(ctx, f.principal, stale); !errors.Is(err, ErrReviewConflict) {
		t.Fatal("stale record predecessor accepted", err)
	}
	read, err := s.ReviewOpen(ctx, f.principal, ReviewOpenRequest{TargetLocator: r.TargetLocator, ExpectedTargetHash: r.ExpectedTargetHash})
	if err != nil || optionalID(read.RecordsHeadRecordID) != result.RecordID || len(read.Records) != 1 {
		t.Fatal("immutable audit file/index readback", err)
	}
	reentry := r.TargetLocator
	reentry.ContentRevisionID = result.RevisionID
	read, err = s.ReviewOpen(ctx, f.principal, ReviewOpenRequest{TargetLocator: reentry})
	if err != nil || read.TargetHash == nil || *read.TargetHash != r.ExpectedTargetHash || read.TargetLocator != r.TargetLocator {
		t.Fatal("audit-only revision changed frozen target", err)
	}
	current, err := s.publicationDocument(ctx, f.principal, f.source.ProjectID, result.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	duplicateTarget, _, err := s.buildReviewTarget(ctx, f.principal, current, reentry)
	if err != nil {
		t.Fatal(err)
	}
	_, duplicateHash, err := reviewJSON(duplicateTarget)
	if err != nil || duplicateHash == r.ExpectedTargetHash {
		t.Fatal("fixture failed to produce alternate source manifest")
	}
	if _, err := s.ReviewOpen(ctx, f.principal, ReviewOpenRequest{TargetLocator: reentry, ExpectedTargetHash: duplicateHash}); !errors.Is(err, ErrTargetMismatch) {
		t.Fatal("same production version allowed a second frozen target", err)
	}
	category := "image"
	if _, err := s.Commit(ctx, f.principal, CommitRequest{ProjectID: f.source.ProjectID, ExpectedRevisionID: result.RevisionID, CommitID: id(402), Changes: &Changes{Metadata: &MetadataChange{Category: &category}}}); !errors.Is(err, ErrForbidden) {
		t.Fatal("category bypass of managed review accepted", err)
	}
	if _, err := db.Exec(ctx, "UPDATE studio.project_set_review_records SET action='approve' WHERE record_id=$1", result.RecordID); err == nil {
		t.Fatal("immutable record trigger allowed mutation")
	}
	if _, err := db.Exec(ctx, "DELETE FROM studio.project_set_review_targets WHERE project_id=$1", f.source.ProjectID); err == nil {
		t.Fatal("immutable target trigger allowed deletion")
	}
}
