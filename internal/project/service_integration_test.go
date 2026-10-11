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

type integrationArtifact struct {
	fail     bool
	versions map[ContentRef]ContentVersion
	bytes    map[ContentRef][]byte
}

func (a *integrationArtifact) Write(_ context.Context, p Principal, project string, w TextWrite) (ContentVersion, error) {
	if a.fail {
		return ContentVersion{}, ErrDependency
	}
	r := ContentRef{id(3), newID(), newID()}
	v := ContentVersion{ContentRef: r, OrganizationID: p.OrganizationID, Source: Source{Kind: "user_edit", ProjectID: project}, MIME: w.MIME}
	a.versions[r] = v
	a.bytes[r] = []byte(w.Text)
	return v, nil
}
func (a *integrationArtifact) WriteAssetManifest(context.Context, Principal, string, string, TextWrite) (ContentVersion, error) {
	return ContentVersion{}, ErrForbidden
}
func (a *integrationArtifact) Metadata(_ context.Context, _ Principal, r ContentRef, _ Access) (ContentVersion, error) {
	v, ok := a.versions[r]
	if !ok {
		return v, ErrNotFound
	}
	return v, nil
}
func (a *integrationArtifact) Read(_ context.Context, _ Principal, r ContentRef, _ Access, _ int64) ([]byte, error) {
	v, ok := a.bytes[r]
	if !ok {
		return nil, ErrNotFound
	}
	return v, nil
}
func (a *integrationArtifact) Retain(context.Context, Principal, RetentionOwner, []ContentRef) error {
	return nil
}

type integrationAssets struct{}

func (integrationAssets) References(context.Context, Principal, AssetRef) ([]ContentRef, error) {
	return nil, ErrNotFound
}
func (integrationAssets) Resolve(context.Context, Principal, AssetRef) (AssetResolution, error) {
	return AssetResolution{}, ErrNotFound
}
func (integrationAssets) Receipt(context.Context, Principal, string, string) (WorldReceipt, error) {
	return WorldReceipt{}, ErrNotFound
}

type integrationPeer struct{ proof coordination.Status }

func (*integrationPeer) Prepare(context.Context, coordination.Operation, []coordination.Grant) error {
	return nil
}
func (*integrationPeer) Finalize(context.Context, coordination.Operation) error { return nil }
func (p *integrationPeer) State(context.Context, coordination.Operation) (coordination.Status, error) {
	if p.proof.State == "" {
		return coordination.Status{}, coordination.ErrUnknown
	}
	return p.proof, nil
}

// Execute only on an explicitly configured 5090 development PostgreSQL endpoint.
// The local compile check does not claim to validate SQL or crash recovery.
func TestProjectDurableEpochAndPendingMembership(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if err := migrations.ApplyFresh(ctx, db, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := migrations.ActivateFresh(ctx, db); err != nil {
		t.Fatal(err)
	}
	store, err := coordination.NewStore(db, "project", 1, AuthorizeReservation)
	if err != nil {
		t.Fatal(err)
	}
	peer := &integrationPeer{}
	engine := &coordination.Engine{Store: store, Peers: map[string]coordination.Peer{"world": peer}}
	content := &integrationArtifact{fail: true, versions: map[ContentRef]ContentVersion{}, bytes: map[ContentRef][]byte{}}
	svc, err := NewService(ctx, db, content, integrationAssets{}, engine)
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{OrganizationID: id(30), SubjectID: id(31), RequestID: id(32)}
	r := CreateRequest{CommitID: id(33), Name: "Prepared project", Category: "image", EntryPath: "review.md", EntryText: "# Review"}
	if _, err = svc.Create(ctx, p, r); !errors.Is(err, ErrDependency) {
		t.Fatal("dependency failure lost preparation", err)
	}
	var projectID string
	if err = db.QueryRow(ctx, "SELECT project_id::text FROM studio.project_create_requests WHERE organization_id=$1 AND subject_id=$2 AND commit_id=$3", p.OrganizationID, p.SubjectID, r.CommitID).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	c, err := loadCommit(ctx, db, p.OrganizationID, projectID, r.CommitID)
	if err != nil || c.State != "preparing" || !svc.validOperation(p, c) || c.Plan.Operation.Fence != 1 {
		t.Fatal("first business record did not preserve admitted epoch", err)
	}
	store.Fence = 2
	if _, err = svc.Create(ctx, p, r); !errors.Is(err, coordination.ErrFenced) {
		t.Fatal("new epoch adopted old preparation", err)
	}
	store.Fence = 1
	content.fail = false
	published, err := svc.Create(ctx, p, r)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := svc.Create(ctx, p, r)
	if err != nil || retry.RevisionID != published.RevisionID || retry.ManifestRef != published.ManifestRef {
		t.Fatal("retry republished", err)
	}
	store.Fence = 2
	if retry, err = svc.Create(ctx, p, r); err != nil || retry.RevisionID != published.RevisionID {
		t.Fatal("epoch rollover hid immutable completed result", err)
	}
	store.Fence = 1
	guest := id(60)
	grant := MemberChangeRequest{ProjectID: projectID, SubjectID: guest, ChangeID: id(61), Role: "editor"}
	if out, err := svc.ChangeMember(ctx, p, grant); err != nil || out.State != "completed" {
		t.Fatal("grant", err)
	}
	op := coordination.Operation{Coordinator: "world", OrganizationID: p.OrganizationID, SubjectID: guest, ResourceID: id(62), CommitID: id(63), RequestHash: strings.Repeat("a", 64), Fence: 1}
	if err = store.Prepare(ctx, op, []coordination.Grant{{ResourceID: projectID}}); err != nil {
		t.Fatal(err)
	}
	revoke := MemberChangeRequest{ProjectID: projectID, SubjectID: guest, ChangeID: id(64), Role: "none"}
	if out, err := svc.ChangeMember(ctx, p, revoke); err != nil || out.State != "pending" {
		t.Fatal("active reservation did not block revoke", err)
	}
	if value, err := role(ctx, db, Principal{OrganizationID: p.OrganizationID, SubjectID: guest, RequestID: p.RequestID}, projectID); err != nil || value != "editor" {
		t.Fatal("pending changed permission", err)
	}
	next := op
	next.CommitID = id(65)
	if err = store.Prepare(ctx, next, []coordination.Grant{{ResourceID: projectID}}); !errors.Is(err, coordination.ErrDenied) {
		t.Fatal("barrier admitted new reservation", err)
	}
	if err = engine.Finalize(ctx, op); !errors.Is(err, coordination.ErrUnknown) {
		t.Fatal("unknown terminal released reservation", err)
	}
	peer.proof = coordination.Status{Operation: op, State: "cancelled"}
	if err = engine.Finalize(ctx, op); err != nil {
		t.Fatal(err)
	}
	if out, err := svc.ChangeMember(ctx, p, revoke); err != nil || out.State != "completed" {
		t.Fatal("authorized retry did not complete revoke", err)
	}
	if err = store.Prepare(ctx, op, []coordination.Grant{{ResourceID: projectID}}); err == nil {
		t.Fatal("cancelled operation resurrected")
	}
}
