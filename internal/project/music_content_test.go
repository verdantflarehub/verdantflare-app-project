package project

import "testing"

func musicFixture(t *testing.T) (contentObject, Manifest) {
	t.Helper()
	_, m := dramaFixture(t)
	m.Category = "music"
	v := contentObject{"key": "demo1", "label": "Demo", "kind": "demo", "basis_revision_id": dramaID(2), "parent_version_key": nil, "change_note": "修改歌词", "file_refs": []any{contentObject{"file_id": m.Files[0].ID, "content_ref": contentObject{"store_id": m.Files[0].Content.StoreID, "artifact_id": m.Files[0].Content.ArtifactID, "version_id": m.Files[0].Content.VersionID}, "purpose": "output"}}, "review": contentObject{"state": "unreviewed", "record_file_id": nil}, "sources": []any{}}
	return contentObject{"schema_version": 1, "kind": "project-content", "category": "music", "music": contentObject{"songs": []any{contentObject{"key": "song", "title": "歌曲", "production": contentObject{"versions": []any{v}, "selected_version_key": nil}}}}}, m
}
func TestMusicProductionScope(t *testing.T) {
	r, m := musicFixture(t)
	if err := ValidateDramaContent(dramaBytes(t, r), m); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing selection", "cycle", "foreign bytes", "foreign file", "no output", "wrong kind", "wrong source"} {
		t.Run(name, func(t *testing.T) {
			r, m := musicFixture(t)
			owner := musicOwners(objectOf(r["music"]))["songs/song"]
			p := objectOf(owner["production"])
			v := objectOf(listOf(p["versions"])[0])
			ref := objectOf(listOf(v["file_refs"])[0])
			switch name {
			case "missing selection":
				p["selected_version_key"] = "absent"
			case "cycle":
				v["parent_version_key"] = "demo1"
			case "foreign bytes":
				objectOf(ref["content_ref"])["version_id"] = dramaID(99)
			case "foreign file":
				ref["file_id"] = dramaID(99)
			case "no output":
				ref["purpose"] = "sample"
			case "wrong kind":
				v["kind"] = "video"
			case "wrong source":
				v["sources"] = []any{contentObject{"project_id": dramaID(30), "revision_id": dramaID(31), "kind": "song", "item_key": "song", "version_key": "one", "file_id": nil}}
			}
			if err := ValidateDramaContent(dramaBytes(t, r), m); err == nil {
				t.Fatal("accepted invalid music production")
			}
		})
	}
}
func TestMusicHistoryCannotBeRemovedOrRewritten(t *testing.T) {
	old, m := musicFixture(t)
	before, err := decodeMusicContent(dramaBytes(t, old), m)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := musicFixture(t)
	after := objectOf(current["music"])
	p := objectOf(musicOwners(after)["songs/song"]["production"])
	p["selected_version_key"] = "demo1"
	if err := checkMusicHistory(before, after); err != nil {
		t.Fatal(err)
	}
	v := objectOf(listOf(p["versions"])[0])
	v["change_note"] = "rewritten"
	if checkMusicHistory(before, after) == nil {
		t.Fatal("rewrote version")
	}
	p["versions"] = []any{}
	if checkMusicHistory(before, after) == nil {
		t.Fatal("removed version")
	}
	if checkMusicHistory(before, nil) == nil {
		t.Fatal("removed owner")
	}
}
