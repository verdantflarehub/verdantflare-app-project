package project

import (
	"encoding/json"
	"testing"
)

func creativeFixture(t *testing.T) (contentObject, Manifest) {
	t.Helper()
	m := Manifest{ProjectID: dramaID(1), Category: "creative_video", Files: []File{{ID: dramaID(10)}}}
	var root contentObject
	if err := json.Unmarshal([]byte(`{"schema_version":1,"kind":"project-content","category":"creative_video","creative_video":{"direction":"music_video","brief":{"key":"main","file_id":null,"goal":"Release teaser"},"rights":[],"concepts":[],"selected_concept_key":null,"sequences":[],"shots":[],"takes":[],"edits":[],"delivery_variants":[],"reviews":[],"packages":[]}}`), &root); err != nil {
		t.Fatal(err)
	}
	return root, m
}

func TestCreativeContentShapeAndReferences(t *testing.T) {
	root, m := creativeFixture(t)
	if _, err := creativeShape(dramaBytes(t, root), m); err != nil {
		t.Fatal(err)
	}
	ref := contentObject{"store_id": dramaID(11), "artifact_id": dramaID(12), "version_id": dramaID(13)}
	creative := objectOf(root["creative_video"])
	creative["music_source"] = contentObject{"kind": "project_file", "project_id": dramaID(2), "revision_id": dramaID(3), "file_id": dramaID(4), "content_ref": ref}
	if _, err := creativeShape(dramaBytes(t, root), m); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(contentObject){
		"missing fixed music content": func(d contentObject) {
			delete(objectOf(objectOf(d["creative_video"])["music_source"]), "content_ref")
		},
		"mixed type content":       func(d contentObject) { d["music"] = contentObject{} },
		"missing selected concept": func(d contentObject) { objectOf(d["creative_video"])["selected_concept_key"] = "unknown" },
		"concept status differs": func(d contentObject) {
			objectOf(d["creative_video"])["concepts"] = []any{contentObject{"key": "concept", "file_id": nil, "status": "selected"}}
		},
		"dangling shot": func(d contentObject) {
			objectOf(d["creative_video"])["sequences"] = []any{contentObject{"key": "seq", "shot_keys": []any{"unknown"}}}
		},
		"duplicate shot key": func(d contentObject) {
			objectOf(d["creative_video"])["shots"] = []any{
				contentObject{"key": "shot", "planned_frames": 10, "take_keys": []any{}},
				contentObject{"key": "shot", "planned_frames": 20, "take_keys": []any{}},
			}
		},
		"take absent from shot list": func(d contentObject) {
			creative := objectOf(d["creative_video"])
			creative["shots"] = []any{contentObject{"key": "shot", "planned_frames": 10, "take_keys": []any{}}}
			creative["takes"] = []any{contentObject{"key": "take", "shot_key": "shot", "file_id": nil, "run_ref": nil, "status": "candidate"}}
		},
		"multiple selected takes": func(d contentObject) {
			creative := objectOf(d["creative_video"])
			creative["shots"] = []any{contentObject{"key": "shot", "planned_frames": 10, "take_keys": []any{"a", "b"}}}
			creative["takes"] = []any{contentObject{"key": "a", "shot_key": "shot", "file_id": nil, "run_ref": nil, "status": "selected"}, contentObject{"key": "b", "shot_key": "shot", "file_id": nil, "run_ref": nil, "status": "selected"}}
		},
		"missing current file": func(d contentObject) {
			objectOf(objectOf(d["creative_video"])["brief"])["file_id"] = dramaID(99)
		},
		"edit uses another take file": func(d contentObject) {
			creative := objectOf(d["creative_video"])
			creative["shots"] = []any{contentObject{"key": "shot", "planned_frames": 10, "take_keys": []any{"take"}}}
			creative["takes"] = []any{contentObject{"key": "take", "shot_key": "shot", "file_id": dramaID(10), "run_ref": nil, "status": "selected"}}
			creative["edits"] = []any{contentObject{"key": "edit", "parent_key": nil, "project_file_id": nil, "fps": contentObject{"num": 25, "den": 1}, "items": []any{contentObject{"take_key": "take", "media_ref": contentObject{"basis_revision_id": dramaID(2), "file_id": dramaID(11), "content_ref": contentObject{"store_id": dramaID(3), "artifact_id": dramaID(4), "version_id": dramaID(5)}}, "source_in_frame": 0, "source_out_frame": 10, "timeline_in_frame": 0}}}}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			var copy contentObject
			if err := json.Unmarshal(dramaBytes(t, root), &copy); err != nil {
				t.Fatal(err)
			}
			change(copy)
			if _, err := creativeShape(dramaBytes(t, copy), m); err == nil {
				t.Fatal("invalid creative content accepted")
			}
		})
	}
}

func TestCreativeHistoryAllowsProgressWithoutReplacingFixedContent(t *testing.T) {
	prior := contentObject{
		"delivery_variants": []any{contentObject{"key": "variant-1", "channel": "web", "territory": "CN"}},
		"takes":             []any{contentObject{"key": "take-1", "shot_key": "shot-1", "file_id": dramaID(10), "status": "candidate"}},
		"edits":             []any{contentObject{"key": "edit-1", "items": []any{contentObject{"take_key": "take-1"}}, "state": "rough"}},
		"packages":          []any{contentObject{"key": "pack-1", "variant_key": "variant-1", "media_file_id": dramaID(11), "review_keys": []any{"review-1"}, "state": "candidate"}},
	}
	current := contentObject{
		"delivery_variants": []any{contentObject{"key": "variant-1", "channel": "web", "territory": "CN"}},
		"takes":             []any{contentObject{"key": "take-1", "shot_key": "shot-1", "file_id": dramaID(10), "status": "selected"}},
		"edits":             []any{contentObject{"key": "edit-1", "items": []any{contentObject{"take_key": "take-1"}}, "state": "picture_locked"}},
		"packages":          []any{contentObject{"key": "pack-1", "variant_key": "variant-1", "media_file_id": dramaID(11), "review_keys": []any{"review-1", "review-2"}, "state": "ready"}},
	}
	if err := creativeHistory(prior, current); err != nil {
		t.Fatal(err)
	}
	objectOf(listOf(current["takes"])[0])["file_id"] = dramaID(12)
	if err := creativeHistory(prior, current); err == nil {
		t.Fatal("take media replacement accepted")
	}
	objectOf(listOf(current["takes"])[0])["file_id"] = dramaID(10)
	objectOf(listOf(current["edits"])[0])["music_source"] = contentObject{"kind": "project_file"}
	if err := creativeHistory(prior, current); err == nil {
		t.Fatal("edit music replacement accepted")
	}
	delete(objectOf(listOf(current["edits"])[0]), "music_source")
	objectOf(listOf(current["packages"])[0])["review_keys"] = []any{"review-2"}
	if err := creativeHistory(prior, current); err == nil {
		t.Fatal("package review removal accepted")
	}
}

func TestCreativeDeliveryEvidenceOnlyAppendsOnDelivery(t *testing.T) {
	variants := []any{contentObject{"key": "variant", "channel": "web", "territory": "CN"}}
	before := contentObject{"delivery_variants": variants, "packages": []any{contentObject{"key": "pack", "variant_key": "variant", "state": "ready", "review_keys": []any{}, "attachment_file_ids": []any{}}}}
	after := contentObject{"delivery_variants": variants, "packages": []any{contentObject{"key": "pack", "variant_key": "variant", "state": "delivered", "review_keys": []any{}, "attachment_file_ids": []any{dramaID(10)}}}}
	if err := creativeHistory(before, after); err != nil {
		t.Fatal(err)
	}
	objectOf(listOf(after["packages"])[0])["state"] = "ready"
	if err := creativeHistory(before, after); err == nil {
		t.Fatal("receipt was appended before delivery")
	}
}

func TestCreativeReadyPackageCannotAppendReview(t *testing.T) {
	variants := []any{contentObject{"key": "variant", "channel": "web", "territory": "CN"}}
	before := contentObject{"delivery_variants": variants, "packages": []any{contentObject{"key": "pack", "variant_key": "variant", "state": "ready", "review_keys": []any{"review-1"}, "attachment_file_ids": []any{}}}}
	after := contentObject{"delivery_variants": variants, "packages": []any{contentObject{"key": "pack", "variant_key": "variant", "state": "ready", "review_keys": []any{"review-1", "review-2"}, "attachment_file_ids": []any{}}}}
	if err := creativeHistory(before, after); err == nil {
		t.Fatal("review was added after package became ready")
	}
	before["reviews"] = []any{contentObject{"key": "review-1", "target_kind": "package", "target_key": "pack", "state": "pending"}}
	after["packages"] = before["packages"]
	after["reviews"] = []any{contentObject{"key": "review-1", "target_kind": "package", "target_key": "pack", "state": "approved"}}
	if err := creativeHistory(before, after); err == nil {
		t.Fatal("review decision changed after package became ready")
	}
}

func TestCreativeReferencedDeliveryVariantIsImmutable(t *testing.T) {
	before := contentObject{
		"delivery_variants": []any{contentObject{"key": "variant", "channel": "web", "territory": "CN"}},
		"packages":          []any{contentObject{"key": "pack", "variant_key": "variant", "state": "candidate", "review_keys": []any{}, "attachment_file_ids": []any{}}},
	}
	after := contentObject{
		"delivery_variants": []any{contentObject{"key": "variant", "channel": "tv", "territory": "CN"}},
		"packages":          []any{contentObject{"key": "pack", "variant_key": "variant", "state": "candidate", "review_keys": []any{}, "attachment_file_ids": []any{}}},
	}
	if err := creativeHistory(before, after); err == nil {
		t.Fatal("variant channel was rewritten after export")
	}
	objectOf(listOf(after["delivery_variants"])[0])["channel"] = "web"
	if err := creativeHistory(before, after); err != nil {
		t.Fatal(err)
	}
}

func TestCreativeFixedFilesKeepContentVersion(t *testing.T) {
	mediaID, recordID := dramaID(10), dramaID(11)
	mediaRef := ContentRef{StoreID: dramaID(20), ArtifactID: dramaID(21), VersionID: dramaID(22)}
	recordRef := ContentRef{StoreID: dramaID(20), ArtifactID: dramaID(23), VersionID: dramaID(24)}
	before := Manifest{Files: []File{{ID: mediaID, Content: mediaRef}, {ID: recordID, Content: recordRef}}}
	after := Manifest{Files: append([]File(nil), before.Files...)}
	prior := contentObject{
		"takes":    []any{contentObject{"key": "take", "file_id": mediaID}},
		"packages": []any{contentObject{"key": "pack", "media_file_id": mediaID, "attachment_file_ids": []any{recordID}}},
		"reviews":  []any{contentObject{"key": "review", "record_file_id": recordID}},
	}
	if err := creativeFixedMediaFiles(prior, before, after); err != nil {
		t.Fatal(err)
	}
	after.Files[0].Content.VersionID = dramaID(25)
	if err := creativeFixedMediaFiles(prior, before, after); err == nil {
		t.Fatal("fixed media file ID was allowed to point to new bytes")
	}
	after.Files[0] = before.Files[0]
	after.Files[1].Content.VersionID = dramaID(26)
	if err := creativeFixedMediaFiles(prior, before, after); err == nil {
		t.Fatal("fixed review or delivery record was allowed to point to new bytes")
	}
}

func TestCreativeLatestReviewSupersedesEarlierApproval(t *testing.T) {
	pack := contentObject{"review_keys": []any{"first", "second"}}
	reviews := map[string]contentObject{
		"first":  {"target_kind": "package", "target_key": "pack", "kind": "client", "state": "approved"},
		"second": {"target_kind": "package", "target_key": "pack", "kind": "client", "state": "changes_requested"},
	}
	approved, err := creativeLatestApprovals("pack", pack, reviews)
	if err != nil || approved["client"] {
		t.Fatal("later review did not supersede earlier approval", err)
	}
}

func TestCreativeNewEditRequiresSelectedTake(t *testing.T) {
	creative := contentObject{"takes": []any{contentObject{"key": "take", "status": "candidate"}}, "edits": []any{contentObject{"key": "edit", "items": []any{contentObject{"take_key": "take"}}}}}
	if err := creativeNewEditTakes(creative, nil); err == nil {
		t.Fatal("candidate take entered a new edit")
	}
	objectOf(listOf(creative["takes"])[0])["status"] = "selected"
	if err := creativeNewEditTakes(creative, nil); err != nil {
		t.Fatal(err)
	}
	objectOf(listOf(creative["takes"])[0])["status"] = "rejected"
	if err := creativeNewEditTakes(creative, map[string]contentObject{"edit": {"key": "edit"}}); err != nil {
		t.Fatal("historical edit was blocked after a take changed status:", err)
	}
}

func TestCreativeHistoryAcceptsFirstTakeMediaOnly(t *testing.T) {
	prior := contentObject{"takes": []any{contentObject{"key": "take-1", "shot_key": "shot-1", "file_id": nil, "status": "candidate"}}}
	current := contentObject{"takes": []any{contentObject{"key": "take-1", "shot_key": "shot-1", "file_id": dramaID(10), "status": "candidate"}}}
	if err := creativeHistory(prior, current); err != nil {
		t.Fatal(err)
	}
	if err := creativeHistory(current, contentObject{"takes": []any{contentObject{"key": "take-1", "shot_key": "shot-1", "file_id": dramaID(11), "status": "candidate"}}}); err == nil {
		t.Fatal("filled take media was replaced")
	}
}

func TestCreativeReviewDecisionIsAttributedAndImmutable(t *testing.T) {
	actor := dramaID(20)
	pending := contentObject{"reviews": []any{contentObject{"key": "review-1", "target_kind": "edit", "target_key": "edit-1", "basis_revision_id": dramaID(2), "kind": "creative", "state": "pending", "decision_by": nil, "record_file_id": nil}}}
	approved := contentObject{"reviews": []any{contentObject{"key": "review-1", "target_kind": "edit", "target_key": "edit-1", "basis_revision_id": dramaID(2), "kind": "creative", "state": "approved", "decision_by": actor, "record_file_id": dramaID(10)}}}
	if err := creativeHistory(pending, approved); err != nil {
		t.Fatal(err)
	}
	if err := creativeDecisionActor(pending, approved, actor); err != nil {
		t.Fatal(err)
	}
	if err := creativeDecisionActor(pending, approved, dramaID(21)); err == nil {
		t.Fatal("another actor's decision was accepted")
	}
	changed := contentObject{"reviews": []any{contentObject{"key": "review-1", "target_kind": "edit", "target_key": "edit-1", "basis_revision_id": dramaID(2), "kind": "creative", "state": "rejected", "decision_by": actor, "record_file_id": dramaID(11)}}}
	if err := creativeHistory(approved, changed); err == nil {
		t.Fatal("approved decision was rewritten")
	}
	if err := creativeHistory(approved, contentObject{"reviews": []any{}}); err == nil {
		t.Fatal("approved decision was removed")
	}
}

func TestCreativeReviewBasisRequiresTargetInFixedRevision(t *testing.T) {
	review := contentObject{"target_kind": "package", "target_key": "pack-1"}
	prior := contentObject{"packages": []any{contentObject{"key": "pack-1"}}}
	if !creativeReviewTargetExists(review, prior) {
		t.Fatal("fixed package target was not found")
	}
	if creativeReviewTargetExists(review, contentObject{"packages": []any{}}) {
		t.Fatal("missing fixed package target was accepted")
	}
	review["target_kind"] = "edit"
	if creativeReviewTargetExists(review, prior) {
		t.Fatal("target in a different collection was accepted")
	}
}

func TestCreativeMusicUseNeedsSourceBoundLocalClearance(t *testing.T) {
	fileID := dramaID(30)
	source := contentObject{"kind": "project_file", "file_id": fileID, "selection_purpose": "master"}
	origin := Manifest{Selections: []Selection{{Purpose: "master", FileIDs: []string{fileID}}}}
	if !creativeMusicSelected(source, origin) {
		t.Fatal("fixed music selection was not recognized")
	}
	source["selection_purpose"] = "other"
	if creativeMusicSelected(source, origin) {
		t.Fatal("wrong selection purpose was accepted")
	}
	source["selection_purpose"] = "master"
	creative := contentObject{"rights": []any{contentObject{"subject_kind": "music", "subject_key": creativeMusicRightKey(source), "status": "cleared", "evidence_file_id": dramaID(31), "channels": []any{"web"}, "territories": []any{"CN"}, "uses": []any{"music_sync"}}}}
	if !creativeMusicCleared(creative, source, "web", "CN") {
		t.Fatal("evidenced music clearance was not recognized")
	}
	if creativeMusicCleared(creative, source, "tv", "CN") {
		t.Fatal("music clearance for another channel was accepted")
	}
	if creativeMusicCleared(creative, source, "web", "US") {
		t.Fatal("music clearance for another territory was accepted")
	}
	source["revision_id"] = dramaID(32)
	if creativeMusicCleared(creative, source, "web", "CN") {
		t.Fatal("music clearance for another source revision was accepted")
	}
	delete(source, "revision_id")
	objectOf(listOf(creative["rights"])[0])["expires_on"] = "2000-01-01"
	if creativeMusicCleared(creative, source, "web", "CN") {
		t.Fatal("expired music clearance was accepted")
	}
	delete(objectOf(listOf(creative["rights"])[0]), "expires_on")
	objectOf(listOf(creative["rights"])[0])["expires_on"] = "2099-99-99"
	if creativeMusicCleared(creative, source, "web", "CN") {
		t.Fatal("invalid music clearance expiry was accepted")
	}
	delete(objectOf(listOf(creative["rights"])[0]), "expires_on")
	objectOf(listOf(creative["rights"])[0])["evidence_file_id"] = nil
	if creativeMusicCleared(creative, source, "web", "CN") {
		t.Fatal("music clearance without evidence was accepted")
	}
}
