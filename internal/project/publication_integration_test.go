//go:build integration

package project

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/verdantflarehub/verdantflare-app-project/internal/coordination"
	"github.com/verdantflarehub/verdantflare-app-project/internal/testdb"
	"github.com/verdantflarehub/verdantflare-app-project/migrations"
)

// Runs only against disposable 5090 databases. Artifact and World are controlled
// in-memory clients; this is not a real Artifact/World transport acceptance test.
func TestPublicationLargeDocumentRestartAndAssociationRecovery(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if err := migrations.ApplyFresh(ctx, db, 1, 1); err != nil {
		t.Fatal("fresh schema failed")
	}
	if err := migrations.ActivateFresh(ctx, db); err != nil {
		t.Fatal("fresh gate failed")
	}
	f := newPublicationFixture(t)
	f.root["overview"] = strings.Repeat("synthetic ", 125000)
	data := publicationJSON(t, f.root)
	if len(data) <= 1<<20 || len(data) >= MaxManifestBytes {
		t.Fatal("fixture did not cross public inline limit")
	}
	if err := ValidateDramaContent(data, f.source.Manifest); err != nil {
		t.Fatal("large source schema invalid")
	}
	f.data[f.source.Manifest.Files[0].Content] = data
	f.data[f.source.ManifestRef] = publicationJSON(t, f.source.Manifest)
	content := &integrationArtifact{versions: map[ContentRef]ContentVersion{}, bytes: f.data}
	for _, file := range f.source.Manifest.Files {
		mime := "application/json"
		if file.ID == f.source.Manifest.EntryDocumentID {
			mime = "text/markdown"
		}
		content.versions[file.Content] = ContentVersion{ContentRef: file.Content, OrganizationID: f.principal.OrganizationID, Source: Source{Kind: "user_edit", ProjectID: f.source.ProjectID}, MIME: mime}
	}
	// Seed one immutable historical revision; all new work uses real Commit,
	// coordination, CAS, retention planning and recovery against this database.
	if _, err := db.Exec(ctx, "INSERT INTO studio.projects(project_id,organization_id,owner_id,name,category,status) VALUES($1,$2,$3,$4,$5,'active')", f.source.ProjectID, f.principal.OrganizationID, f.principal.SubjectID, f.source.Manifest.Name, f.source.Manifest.Category); err != nil {
		t.Fatal("seed project failed")
	}
	if _, err := db.Exec(ctx, "INSERT INTO studio.project_members(organization_id,project_id,subject_id,role) VALUES($1,$2,$3,'owner')", f.principal.OrganizationID, f.source.ProjectID, f.principal.SubjectID); err != nil {
		t.Fatal("seed owner failed")
	}
	if _, err := db.Exec(ctx, "INSERT INTO studio.project_commits(organization_id,project_id,commit_id,created_by,request_sha256,revision_id,state,plan,result) VALUES($1,$2,$3,$4,$5,$6,'committed','{}',$7)", f.principal.OrganizationID, f.source.ProjectID, id(90), f.principal.SubjectID, strings.Repeat("a", 64), f.source.RevisionID, publicationJSON(t, f.source.Result)); err != nil {
		t.Fatal("seed commit failed")
	}
	if _, err := db.Exec(ctx, "INSERT INTO studio.project_revisions(revision_id,organization_id,project_id,manifest_ref,manifest_json,created_by,commit_id) VALUES($1,$2,$3,$4,$5,$6,$7)", f.source.RevisionID, f.principal.OrganizationID, f.source.ProjectID, publicationJSON(t, f.source.ManifestRef), publicationJSON(t, f.source.Manifest), f.principal.SubjectID, id(90)); err != nil {
		t.Fatal("seed revision failed")
	}
	if _, err := db.Exec(ctx, "UPDATE studio.projects SET head_revision_id=$1 WHERE project_id=$2", f.source.RevisionID, f.source.ProjectID); err != nil {
		t.Fatal("seed head failed")
	}
	assets := &publicationAssets{receipt: f.receipt}
	newService := func() *Service {
		t.Helper()
		store, err := coordination.NewStore(db, "project", 1, AuthorizeReservation)
		if err != nil {
			t.Fatal("coordination construction failed")
		}
		engine := &coordination.Engine{Store: store, Peers: map[string]coordination.Peer{"world": &integrationPeer{}}}
		s, err := NewService(ctx, db, content, assets, engine)
		if err != nil {
			t.Fatal("service construction failed")
		}
		return s
	}
	s := newService()
	if _, err := s.checkContents(ctx, f.principal, f.source.Manifest, f.source.RevisionID); err != nil {
		t.Fatalf("source content fixture invalid: %s", publicationErrorLabel(err))
	}
	r := PublicationPrepareRequest{ProjectID: f.source.ProjectID, OriginRevisionID: f.source.RevisionID, CandidateKey: f.locator.CandidateKey, CommitID: f.request.CommitID, PrepareCommitID: id(40)}
	content.fail = true
	preparing, err := s.PublicationPrepare(ctx, f.principal, r)
	if err != nil || preparing.State != "preparing" {
		t.Fatalf("large preparation was not durably pending: state=%q error=%s", preparing.State, publicationErrorLabel(err))
	}
	before, err := loadCommit(ctx, db, f.principal.OrganizationID, r.ProjectID, r.PrepareCommitID)
	if err != nil || len(before.Plan.Writes) != 1 || len(before.Plan.Writes[0].Text) <= 1<<20 {
		t.Fatal("large original write was not persisted")
	}
	writeID := before.Plan.Writes[0].WriteID
	content.fail = false
	s = newService()
	if err = s.RecoverOnce(ctx); err != nil {
		t.Fatalf("restart could not recover large domain document: %s", publicationErrorLabel(err))
	}
	prepared, err := s.PublicationPrepare(ctx, f.principal, r)
	if err != nil || prepared.State != "committed" || prepared.Result == nil || prepared.RequestHash != preparing.RequestHash {
		t.Fatalf("same preparation did not recover original result: state=%q error=%s", prepared.State, publicationErrorLabel(err))
	}
	after, err := loadCommit(ctx, db, f.principal.OrganizationID, r.ProjectID, r.PrepareCommitID)
	if err != nil || after.Plan.Writes[0].WriteID != writeID {
		t.Fatal("restart replaced original Artifact write intent")
	}
	read, err := s.publicationDocument(ctx, f.principal, r.ProjectID, prepared.Result.RevisionID)
	if err != nil {
		t.Fatal("prepared document unreadable")
	}
	candidate, err := read.candidate(r.CandidateKey)
	request, requestErr := publicationRegistration(candidate, r.ProjectID, r.CommitID)
	if err != nil || requestErr != nil || request.SourceRevisionID != f.source.RevisionID || !contentEqual(request, f.request) {
		t.Fatal("prepared request changed original fixed source")
	}
	association := PublicationAssociateRequest{PublicationReceiptRequest: f.locator, ExpectedRevisionID: prepared.Result.RevisionID, AssociationCommitID: id(41)}
	association.PreparedRevisionID = prepared.Result.RevisionID
	result, err := s.PublicationAssociate(ctx, f.principal, association)
	if err != nil || result.State != "committed" || result.Result == nil {
		t.Fatalf("association failed: state=%q error=%s", result.State, publicationErrorLabel(err))
	}
	assets.err = ErrDependency
	calls := assets.calls
	s = newService()
	retry, err := s.PublicationAssociate(ctx, f.principal, association)
	if err != nil || retry.State != "committed" || retry.Result == nil || retry.Result.RevisionID != result.Result.RevisionID || retry.RequestHash != result.RequestHash || assets.calls != calls {
		t.Fatal("completed association could not recover while World was offline")
	}
	foreign := association
	foreign.VersionID = id(999)
	if _, err = s.PublicationAssociate(ctx, f.principal, foreign); !errors.Is(err, ErrIdempotency) {
		t.Fatal("same association ID accepted another asset version")
	}
	stranger := f.principal
	stranger.SubjectID = id(999)
	if _, err = s.PublicationAssociate(ctx, stranger, association); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign actor recovered association")
	}
	var count int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM studio.project_revisions WHERE project_id=$1", r.ProjectID).Scan(&count); err != nil || count != 3 {
		t.Fatal("retry duplicated a project revision")
	}
}

func publicationErrorLabel(err error) string {
	if err == nil {
		return "none"
	}
	for _, known := range []error{ErrInvalid, ErrForbidden, ErrNotFound, ErrDependency, ErrNotReady, ErrIdempotency, ErrConflict, coordination.ErrFenced} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return "other_error"
}
