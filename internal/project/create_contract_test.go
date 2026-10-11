package project

import "testing"

func TestCreateRequestRecoveryDigest(t *testing.T) {
	// Shared with the Dashboard: creation recovery must hash the complete original
	// CreateRequest using Go's field order and JSON escaping, not a CommitRequest.
	r := CreateRequest{
		CommitID:  id(4),
		Name:      "新建 <&>",
		Category:  "ai_short_drama",
		EntryPath: "docs/review.md",
		EntryText: "# 初稿\n内容 <tag>&\u2028next\u2029",
	}
	const expected = "788489772901d511a56df66cbc4c46f8034ce17212d027640604ee43f4952303"
	got, err := digest(r)
	if err != nil || got != expected {
		t.Fatal("create recovery digest differs from the shared Dashboard vector")
	}
	plan, err := initial(r, id(1))
	if err != nil || len(plan.Writes) != 1 || len(plan.Manifest.Files) != 1 {
		t.Fatal("initial project changed shape", err)
	}
	entry, write := plan.Manifest.Files[0], plan.Writes[0]
	if plan.Manifest.ProjectID != id(1) || plan.Manifest.Name != r.Name || plan.Manifest.Category != r.Category || plan.Manifest.Status != "draft" ||
		entry.ID == r.CommitID || !ValidID(entry.ID) || entry.ID != plan.Manifest.EntryDocumentID || entry.ID != write.FileID ||
		entry.Path != r.EntryPath || entry.Role != "review" || write.Text != r.EntryText || write.MIME != "text/markdown" {
		t.Fatal("initial project does not preserve the original creation request")
	}
	for _, change := range []func(*CreateRequest){
		func(v *CreateRequest) { v.CommitID = id(5) },
		func(v *CreateRequest) { v.Name += "changed" },
		func(v *CreateRequest) { v.Category = "image" },
		func(v *CreateRequest) { v.EntryPath = "other.md" },
		func(v *CreateRequest) { v.EntryText += "changed" },
	} {
		changed := r
		change(&changed)
		if other, err := digest(changed); err != nil || other == expected {
			t.Fatal("creation recovery ignored a changed request field")
		}
	}
}
