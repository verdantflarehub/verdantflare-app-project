package project

import "testing"

func imageFixture(t *testing.T) (contentObject, Manifest) {
	_, m := dramaFixture(t)
	m.Category = "image"
	ref := m.Files[0]
	v := contentObject{"key": "v1", "label": "First", "parent_version_key": nil, "change_note": "Initial output", "file_refs": []any{contentObject{"file_id": ref.ID, "content_ref": contentObject{"store_id": ref.Content.StoreID, "artifact_id": ref.Content.ArtifactID, "version_id": ref.Content.VersionID}, "purpose": "output"}}, "reference_file_ids": []any{m.Files[1].ID}, "prompt_file_ids": []any{m.Files[2].ID}}
	return contentObject{"schema_version": 1, "kind": "project-content", "category": "image", "image": contentObject{"works": []any{contentObject{"key": "poster", "title": "Poster", "display_version_key": "v1", "versions": []any{v}}}}}, m
}
func TestImageVersionReferences(t *testing.T) {
	root, m := imageFixture(t)
	if _, err := imageShape(dramaBytes(t, root), m); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(contentObject){
		"missing parent":    func(w contentObject) { objectOf(listOf(w["versions"])[0])["parent_version_key"] = "missing" },
		"cycle":             func(w contentObject) { objectOf(listOf(w["versions"])[0])["parent_version_key"] = "v1" },
		"display missing":   func(w contentObject) { w["display_version_key"] = "missing" },
		"duplicate version": func(w contentObject) { w["versions"] = append(listOf(w["versions"]), listOf(w["versions"])[0]) },
		"fixed bytes mismatch": func(w contentObject) {
			objectOf(objectOf(listOf(objectOf(listOf(w["versions"])[0])["file_refs"])[0])["content_ref"])["version_id"] = dramaID(999)
		},
		"missing prompt": func(w contentObject) { objectOf(listOf(w["versions"])[0])["prompt_file_ids"] = []any{dramaID(999)} },
		"duplicate output": func(w contentObject) {
			v := objectOf(listOf(w["versions"])[0])
			v["file_refs"] = append(listOf(v["file_refs"]), listOf(v["file_refs"])[0])
		},
		"no output": func(w contentObject) {
			objectOf(listOf(objectOf(listOf(w["versions"])[0])["file_refs"])[0])["purpose"] = "record"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			root, m := imageFixture(t)
			mutate(objectOf(listOf(objectOf(root["image"])["works"])[0]))
			if _, err := imageShape(dramaBytes(t, root), m); err == nil {
				t.Fatal("accepted invalid image version")
			}
		})
	}
}
func TestImageHistoryPinsVersionsAndInputs(t *testing.T) {
	root, old := imageFixture(t)
	prior := objectOf(root["image"])
	nextRoot, next := imageFixture(t)
	current := objectOf(nextRoot["image"])
	if err := imageHistory(prior, current, old, next); err != nil {
		t.Fatal(err)
	}
	work := objectOf(listOf(current["works"])[0])
	work["display_version_key"] = nil
	if err := imageHistory(prior, current, old, next); err != nil {
		t.Fatal("display change must be allowed", err)
	}
	v := objectOf(listOf(work["versions"])[0])
	v["change_note"] = "rewrite"
	if imageHistory(prior, current, old, next) == nil {
		t.Fatal("rewritten history accepted")
	}
	v["change_note"] = "Initial output"
	next.Files[2].Content.VersionID = dramaID(999)
	if imageHistory(prior, current, old, next) == nil {
		t.Fatal("replaced prompt bytes accepted")
	}
	if imageHistory(prior, nil, old, next) == nil {
		t.Fatal("removed history accepted")
	}
}
