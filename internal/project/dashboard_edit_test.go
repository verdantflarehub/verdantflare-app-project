package project

import "testing"

func TestDashboardEditCannotModifyOtherProjectData(t *testing.T) {
	name := "项目<&>"
	request := CommitRequest{ProjectID: id(1), ExpectedRevisionID: id(2), CommitID: id(3), Changes: &Changes{Metadata: &MetadataChange{Name: &name}}}
	if err := dashboardEditShape(request); err != nil {
		t.Fatal(err)
	}
	category := "video"
	request.Changes.Metadata.Category = &category
	if dashboardEditShape(request) == nil {
		t.Fatal("category update accepted")
	}
	request.Changes.Metadata.Category = nil
	refs := []AssetRef{}
	request.Changes.AssetRefs = &refs
	if dashboardEditShape(request) == nil {
		t.Fatal("asset references accepted")
	}
}

func TestDashboardEditPreservesDocumentIdentityAndOtherDomainDocuments(t *testing.T) {
	text := `{"schema_version":1}`
	m := example()
	m.DomainDocuments = []DomainDocument{{Type: "existing", FileID: id(2)}}
	docs := append(append([]DomainDocument{}, m.DomainDocuments...), DomainDocument{Type: "project-content", FileID: id(9)})
	files := []FileUpdate{{ID: id(9), Path: "project-content.json", Role: "domain", Text: &text, MIME: "application/json"}}
	r := CommitRequest{ProjectID: id(1), ExpectedRevisionID: id(2), CommitID: id(3), Changes: &Changes{UpsertFiles: &files, DomainDocuments: &docs}}
	if err := dashboardEditShape(r); err != nil {
		t.Fatal(err)
	}
	if err := dashboardEditDocument(r, m); err != nil {
		t.Fatal(err)
	}
	docs[0].FileID = id(6)
	if dashboardEditDocument(r, m) == nil {
		t.Fatal("changed unrelated document accepted")
	}
	docs[0].FileID = id(2)
	m.DomainDocuments = []DomainDocument{{Type: "project-content", FileID: id(6)}}
	r.Changes.DomainDocuments = nil
	files[0].ID, files[0].Path, files[0].Role = id(6), m.Files[1].Path, m.Files[1].Role
	if err := dashboardEditDocument(r, m); err != nil {
		t.Fatal(err)
	}
	files[0].Path = "other.json"
	if dashboardEditDocument(r, m) == nil {
		t.Fatal("renamed domain file accepted")
	}
	files[0].Path, files[0].ID = m.Files[1].Path, id(2)
	if dashboardEditDocument(r, m) == nil {
		t.Fatal("overwriting review accepted")
	}
}

func TestDashboardFirstDocumentAllocatesServerIdentityWithoutChangingRequest(t *testing.T) {
	text := `{"schema_version":1,"kind":"project-content","category":"character","character":{}}`
	m := example()
	m.Category = "character"
	files := []FileUpdate{{ID: id(9), Path: "project-content.json", Role: "domain", Text: &text, MIME: "application/json"}}
	docs := []DomainDocument{{Type: "project-content", FileID: id(9)}}
	r := CommitRequest{ProjectID: id(1), ExpectedRevisionID: id(2), CommitID: id(3), Changes: &Changes{UpsertFiles: &files, DomainDocuments: &docs}}
	before, _ := digest(r)
	plan, err := changedDashboard(m, r)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := digest(r)
	if before != after || len(plan.Writes) != 1 || plan.Writes[0].FileID == id(9) || plan.Manifest.DomainDocuments[0].FileID != plan.Writes[0].FileID {
		t.Fatal("request hash or server file mapping changed")
	}
	if len(plan.Manifest.Files) != len(m.Files)+1 || plan.Manifest.EntryDocumentID != m.EntryDocumentID {
		t.Fatal("original inventory changed")
	}
}
