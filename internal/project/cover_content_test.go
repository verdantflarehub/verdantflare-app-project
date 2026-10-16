package project

import "testing"

func TestCoverContentMetadata(t *testing.T) {
	for _, position := range []any{nil, contentObject{"x": 0, "y": 100}, contentObject{"x": 50.5, "y": 0}} {
		root, manifest := dramaFixture(t)
		root["cover_file_id"] = dramaID(10)
		root["cover_position"] = position
		if err := ValidateDramaContent(dramaBytes(t, root), manifest); err != nil {
			t.Fatalf("valid cover metadata: %v", err)
		}
	}
	cases := map[string]func(contentObject){
		"negative":                         func(r contentObject) { r["cover_position"] = contentObject{"x": -1, "y": 0} },
		"overflow":                         func(r contentObject) { r["cover_position"] = contentObject{"x": 50, "y": 101} },
		"missing axis":                     func(r contentObject) { r["cover_position"] = contentObject{"x": 50} },
		"unknown axis":                     func(r contentObject) { r["cover_position"] = contentObject{"x": 50, "y": 0, "z": 1} },
		"text":                             func(r contentObject) { r["cover_position"] = contentObject{"x": "50", "y": 0} },
		"placeholder conflicts with image": func(r contentObject) { r["cover_placeholder"] = true; r["cover_file_id"] = dramaID(10) },
		"placeholder not boolean":          func(r contentObject) { r["cover_placeholder"] = "true" },
		"foreign cover file":               func(r contentObject) { r["cover_file_id"] = dramaID(99) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			root, manifest := dramaFixture(t)
			mutate(root)
			if err := ValidateDramaContent(dramaBytes(t, root), manifest); err == nil {
				t.Fatal("accepted invalid cover metadata")
			}
		})
	}
	root, manifest := dramaFixture(t)
	root["cover_placeholder"] = true
	root["cover_file_id"] = nil
	if err := ValidateDramaContent(dramaBytes(t, root), manifest); err != nil {
		t.Fatal(err)
	}
}
