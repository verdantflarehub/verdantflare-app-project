package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

type publicationFixture struct {
	principal Principal
	source    OpenResult
	root      contentObject
	data      map[ContentRef][]byte
	request   WorldRegisterRequest
	locator   PublicationReceiptRequest
	receipt   WorldReceipt
}

func publicationJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func newPublicationFixture(t *testing.T) publicationFixture {
	t.Helper()
	p := Principal{OrganizationID: id(80), SubjectID: id(81), RequestID: id(82)}
	m := Manifest{SchemaVersion: 1, Kind: "project", ProjectID: id(1), Name: "Synthetic publication", Category: "ai_short_drama", Status: "active", EntryDocumentID: id(14), Selections: []Selection{{Purpose: "drama/globals/hall/night/lighting", FileIDs: []string{id(11)}}}, AssetRefs: []AssetRef{}, RunRefs: []RunRef{}, DomainDocuments: []DomainDocument{{Type: "project-content", FileID: id(10)}}}
	for n := 10; n <= 14; n++ {
		m.Files = append(m.Files, File{ID: id(n), Path: fmt.Sprintf("files/%d.json", n), Role: "document", Content: ContentRef{id(100), id(n + 100), id(n + 200)}})
	}
	m.Files[4].Path = "review.md"
	fileRef := func(i int, purpose string) contentObject {
		f := m.Files[i]
		return contentObject{"file_id": f.ID, "content_ref": f.Content, "purpose": purpose}
	}
	locator := contentObject{"kind": "global", "key": "hall"}
	version := contentObject{"key": "v1", "label": "L1", "state_key": "night", "basis_revision_id": id(2), "change_note": "Synthetic fixture", "file_refs": []any{fileRef(1, "lighting")}, "lighting_record_file_id": id(12), "generation_record_file_ids": []any{}, "review": contentObject{"state": "unreviewed"}}
	owner := contentObject{"key": "hall", "kind": "location", "name": "Synthetic hall", "states": []any{contentObject{"key": "night", "name": "Night", "definition_refs": []any{}, "marks": []any{}}}, "versions": []any{version}, "selected_versions": []any{contentObject{"state_key": "night", "purpose": "lighting", "version_key": "v1"}}}
	candidate := contentObject{"key": "hall-package", "name": "Synthetic package", "source": locator, "publication_record_file_id": id(13), "review": contentObject{"state": "approved", "record_file_id": id(14)}}
	root := contentObject{"schema_version": 1, "kind": "project-content", "category": "ai_short_drama", "ai_short_drama": contentObject{"globals": []any{owner}, "episodes": []any{}, "world_candidates": []any{candidate}}}
	// Round-trip structs into the same JSON object form used by Artifact reads.
	var decoded contentObject
	if err := json.Unmarshal(publicationJSON(t, root), &decoded); err != nil {
		t.Fatal(err)
	}
	root = decoded
	publication := contentObject{"schema_version": 1, "kind": "short-drama-publication", "source": locator, "versions": []any{contentObject{"version_key": "v1", "file_refs": []any{fileRef(1, "lighting")}}}, "supporting_file_refs": []any{fileRef(2, "lighting_record")}}
	data := map[ContentRef][]byte{}
	for _, f := range m.Files {
		data[f.Content] = []byte("synthetic fixture")
	}
	data[m.Files[0].Content] = publicationJSON(t, root)
	data[m.Files[3].Content] = publicationJSON(t, publication)
	source := OpenResult{Result: Result{ProjectID: id(1), RevisionID: id(2), ManifestRef: ContentRef{id(100), id(500), id(501)}, Manifest: m, Invalidated: []string{}}, HeadRevisionID: id(2)}
	request, err := normalizeWorld(WorldRegisterRequest{CommitID: id(20), SourceProjectID: id(1), SourceRevisionID: id(2), FileIDs: []string{id(11), id(12), id(13)}, Name: "Synthetic package", AssetType: "location", Subjects: []string{"Synthetic hall"}, Relation: "produced_in", ReviewFileID: id(14)})
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := digest(request)
	result := WorldResult{AssetID: id(30), VersionID: id(31), ManifestRef: ContentRef{id(100), id(510), id(511)}, Manifest: AssetManifest{SchemaVersion: 1, Kind: "asset-version", AssetID: id(30), Name: request.Name, AssetType: request.AssetType, Subjects: request.Subjects, Source: AssetSource{ProjectID: id(1), RevisionID: id(2), Relation: request.Relation, ReviewFileID: id(14)}, Files: append([]File{}, m.Files[1:4]...), DependsOn: []AssetRef{}}}
	return publicationFixture{principal: p, source: source, root: root, data: data, request: request, locator: PublicationReceiptRequest{ProjectID: id(1), PreparedRevisionID: id(3), CandidateKey: "hall-package", CommitID: id(20), AssetID: id(30), VersionID: id(31)}, receipt: WorldReceipt{Actor: p.SubjectID, Hash: hash, State: "committed", Version: id(31), Result: &result}}
}
func (f publicationFixture) prepared(t *testing.T) (OpenResult, contentObject) {
	var root contentObject
	_ = json.Unmarshal(publicationJSON(t, f.root), &root)
	c := objectOf(listOf(objectOf(root["ai_short_drama"])["world_candidates"])[0])
	c["registration_request"] = f.request
	opened := f.source
	opened.RevisionID = f.locator.PreparedRevisionID
	return opened, objectOf(root["ai_short_drama"])
}

type publicationAssets struct {
	AssetResolver
	receipt       WorldReceipt
	err           error
	calls         int
	principal     Principal
	asset, commit string
}

func (a *publicationAssets) Receipt(_ context.Context, p Principal, asset, commit string) (WorldReceipt, error) {
	a.calls++
	a.principal = p
	a.asset = asset
	a.commit = commit
	return a.receipt, a.err
}
func TestPublicationReceiptBindsFixedScopeAndPrincipal(t *testing.T) {
	f := newPublicationFixture(t)
	for _, kind := range []string{"valid", "hash", "pending", "asset", "version", "source", "extra file", "content", "denied", "wrong candidate", "unknown commit"} {
		t.Run(kind, func(t *testing.T) {
			f := newPublicationFixture(t)
			r := f.locator
			a := &publicationAssets{receipt: f.receipt}
			prepared, drama := f.prepared(t)
			switch kind {
			case "hash":
				a.receipt.Hash = strings.Repeat("a", 64)
			case "pending":
				a.receipt.State = "preparing"
				a.receipt.Result = nil
			case "asset":
				a.receipt.Result.AssetID = id(999)
			case "version":
				r.VersionID = id(999)
			case "source":
				a.receipt.Result.Manifest.Source.RevisionID = id(999)
			case "extra file":
				a.receipt.Result.Manifest.Files = append(a.receipt.Result.Manifest.Files, f.source.Manifest.Files[0])
			case "content":
				a.receipt.Result.Manifest.Files[0].Content.VersionID = id(999)
			case "denied":
				a.err = ErrForbidden
			case "wrong candidate":
				r.CandidateKey = "other"
			case "unknown commit":
				r.CommitID = id(999)
			}
			read := func(_ context.Context, p Principal, project, revision string) (OpenResult, contentObject, error) {
				if p != f.principal || project != id(1) {
					t.Fatal("identity changed")
				}
				if revision == f.locator.PreparedRevisionID {
					return prepared, drama, nil
				}
				if revision == id(2) {
					return f.source, objectOf(f.root["ai_short_drama"]), nil
				}
				t.Fatal("read moving or foreign revision")
				return OpenResult{}, nil, ErrInvalid
			}
			out, err := publicationReceipt(context.Background(), f.principal, r, read, a)
			if kind == "valid" {
				if err != nil || out.State != "committed" || out.RequestHash != f.receipt.Hash || a.principal != f.principal || a.asset != r.AssetID || a.commit != r.CommitID {
					t.Fatal("fixed receipt failed", err)
				}
			} else if err == nil {
				t.Fatal("invalid receipt accepted")
			}
		})
	}
	if _, err := publicationReceipt(context.Background(), Principal{}, f.locator, nil, nil); !errors.Is(err, ErrForbidden) {
		t.Fatal("invalid principal accepted")
	}
}
func TestPublicationHTTPRejectsAuthorityAndUnknownFields(t *testing.T) {
	f := newPublicationFixture(t)
	s := &Service{}
	h, err := s.HTTPHandler(strings.Repeat("s", 32), strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"publication_prepare", "publication_associate", "publication_receipt"} {
		for _, authorized := range []bool{false, true} {
			r := httptest.NewRequest("POST", "/internal/v1/project/"+operation, bytes.NewBufferString(`{"endpoint":"https://untrusted.invalid"}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+strings.Repeat("s", 32))
			r.Header.Set("X-Organization-Id", f.principal.OrganizationID)
			r.Header.Set("X-Request-Id", f.principal.RequestID)
			if authorized {
				r.Header.Set("X-User-Id", f.principal.SubjectID)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			want := 403
			if authorized {
				want = 400
			}
			if w.Code != want {
				t.Fatal("authority or body bypass", operation, w.Code)
			}
		}
	}
}
func TestPublicationAssociationRecoveryRejectsChangedIntent(t *testing.T) {
	for _, kind := range []string{"valid", "actor", "expected", "asset", "version", "commit", "unrelated content", "extra write", "hash"} {
		t.Run(kind, func(t *testing.T) {
			f := newPublicationFixture(t)
			opened, drama := f.prepared(t)
			root := contentObject{"schema_version": 1, "kind": "project-content", "category": "ai_short_drama", "ai_short_drama": drama}
			doc := publicationDocument{Opened: opened, File: f.source.Manifest.Files[0], Root: root}
			candidate, _ := doc.candidate(f.locator.CandidateKey)
			status := WorldStatus{State: "committed", RequestHash: f.receipt.Hash, Result: f.receipt.Result}
			candidate["registration_status"] = status
			r := PublicationAssociateRequest{PublicationReceiptRequest: f.locator, ExpectedRevisionID: opened.RevisionID, AssociationCommitID: id(50)}
			commit, _ := doc.commit(r.AssociationCommitID)
			hash, _ := digest(commit)
			write := TextWrite{FileID: doc.File.ID, Text: *(*commit.Changes.UpsertFiles)[0].Text, MIME: "application/json"}
			prior := commitRecord{ProjectID: r.ProjectID, CommitID: r.AssociationCommitID, Actor: f.principal.SubjectID, Expected: r.ExpectedRevisionID, Hash: hash, Plan: preparePlan{Writes: []TextWrite{write}}}
			delete(candidate, "registration_status")
			switch kind {
			case "actor":
				prior.Actor = id(999)
			case "expected":
				r.ExpectedRevisionID = id(999)
			case "asset":
				r.AssetID = id(999)
			case "version":
				r.VersionID = id(999)
			case "commit":
				r.CommitID = id(999)
			case "unrelated content":
				doc.Root["overview"] = "changed"
			case "extra write":
				prior.Plan.Writes = append(prior.Plan.Writes, write)
			case "hash":
				prior.Hash = strings.Repeat("a", 64)
			}
			out, err := priorAssociation(doc, candidate, f.request, r, f.principal, prior)
			if kind == "valid" {
				h, _ := digest(out)
				if err != nil || h != hash {
					t.Fatal("exact recovery failed", err)
				}
			} else if err == nil {
				t.Fatal("changed intent accepted")
			}
		})
	}
}
func TestPublicationLargeDocumentAllowanceIsNarrow(t *testing.T) {
	f := newPublicationFixture(t)
	opened, drama := f.prepared(t)
	d := publicationDocument{Opened: opened, File: f.source.Manifest.Files[0], Root: contentObject{"schema_version": 1, "kind": "project-content", "category": "ai_short_drama", "overview": strings.Repeat("x", (1<<20)+200), "ai_short_drama": drama}}
	r, _ := d.commit(id(50))
	if err := publicationChange(f.source.Manifest, r); err != nil {
		t.Fatal(err)
	}
	if _, err := changed(f.source.Manifest, r); err == nil {
		t.Fatal("public inline bound expanded")
	}
	if _, err := changedWithInlineLimit(f.source.Manifest, r, MaxManifestBytes); err != nil {
		t.Fatal("internal preparation rejected", err)
	}
	(*r.Changes.UpsertFiles)[0].ID = id(11)
	if publicationChange(f.source.Manifest, r) == nil {
		t.Fatal("non-domain write got larger allowance")
	}
}
