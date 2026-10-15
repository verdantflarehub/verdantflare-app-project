package project

import "testing"

func TestFixedMusicSourceSchema(t *testing.T) {
	fixture := func() (contentObject, Manifest, contentObject) {
		root, manifest := dramaFixture(t)
		g := objectOf(listOf(objectOf(root["ai_short_drama"])["globals"])[0])
		g["kind"] = "music"
		g["music_source"] = contentObject{"project_id": dramaID(101), "revision_id": dramaID(102), "song_key": "song_one"}
		return root, manifest, g
	}
	root, manifest, _ := fixture()
	if err := ValidateDramaContent(dramaBytes(t, root), manifest); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(contentObject){
		"wrong kind":       func(g contentObject) { g["kind"] = "prop" },
		"missing revision": func(g contentObject) { delete(objectOf(g["music_source"]), "revision_id") },
		"invalid project":  func(g contentObject) { objectOf(g["music_source"])["project_id"] = "foreign" },
		"empty song":       func(g contentObject) { objectOf(g["music_source"])["song_key"] = " " },
		"unknown field":    func(g contentObject) { objectOf(g["music_source"])["head"] = dramaID(103) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r, m, g := fixture()
			mutate(g)
			if err := ValidateDramaContent(dramaBytes(t, r), m); err == nil {
				t.Fatal("accepted invalid fixed music source")
			}
		})
	}
}
