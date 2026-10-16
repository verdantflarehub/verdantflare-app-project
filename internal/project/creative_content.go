package project

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"mime"
	"strings"
	"time"
)

func creativeFiles(value any, files map[string]File, path string) error {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if key == "music_source" || key == "media_ref" || key == "basis_brief_ref" {
				continue
			}
			if (key == "file_id" || strings.HasSuffix(key, "_file_id")) && child != nil {
				if _, ok := files[str(child)]; !ok {
					return contentError(path+"/"+key, "file not in current manifest")
				}
			}
			if key == "file_ids" || strings.HasSuffix(key, "_file_ids") {
				seen := map[string]bool{}
				for _, id := range listOf(child) {
					if _, ok := files[str(id)]; !ok || seen[str(id)] {
						return contentError(path+"/"+key, "missing or duplicate file")
					}
					seen[str(id)] = true
				}
			}
			if err := creativeFiles(child, files, path+"/"+key); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range node {
			if err := creativeFiles(child, files, fmt.Sprintf("%s/%d", path, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func creativeShape(data []byte, m Manifest) (contentObject, error) {
	var root contentObject
	if err := json.Unmarshal(data, &root); err != nil || root == nil || root["category"] != m.Category {
		return nil, contentError("project-content", "invalid category or JSON")
	}
	schema, err := contentSchema()
	if err != nil {
		return nil, fmt.Errorf("compile project-content schema: %w", err)
	}
	if err := schema.Validate(root); err != nil {
		return nil, contentError("project-content", "schema validation failed")
	}
	for key := range root {
		if key != "schema_version" && key != "kind" && key != "category" && key != "overview" && key != "cover_file_id" && key != "creative_video" {
			return nil, contentError("project-content/"+key, "unknown field")
		}
	}
	files := map[string]File{}
	for _, file := range m.Files {
		files[file.ID] = file
	}
	if err := creativeFiles(root, files, "project-content"); err != nil {
		return nil, err
	}
	creative := objectOf(root["creative_video"])
	if _, err := indexObjects(creative["rights"], "rights"); err != nil {
		return nil, err
	}
	concepts, err := indexObjects(creative["concepts"], "concepts")
	if err != nil {
		return nil, err
	}
	sequences, err := indexObjects(creative["sequences"], "sequences")
	if err != nil {
		return nil, err
	}
	shots, err := indexObjects(creative["shots"], "shots")
	if err != nil {
		return nil, err
	}
	takes, err := indexObjects(creative["takes"], "takes")
	if err != nil {
		return nil, err
	}
	edits, err := indexObjects(creative["edits"], "edits")
	if err != nil {
		return nil, err
	}
	variants, err := indexObjects(creative["delivery_variants"], "delivery_variants")
	if err != nil {
		return nil, err
	}
	reviews, err := indexObjects(creative["reviews"], "reviews")
	if err != nil {
		return nil, err
	}
	packages, err := indexObjects(creative["packages"], "packages")
	if err != nil {
		return nil, err
	}
	selectedConcept := str(creative["selected_concept_key"])
	if selectedConcept != "" && concepts[selectedConcept] == nil {
		return nil, contentError("selected_concept_key", "missing concept")
	}
	for key, concept := range concepts {
		if (concept["status"] == "selected") != (key == selectedConcept) {
			return nil, contentError("concepts/"+key, "selection state differs")
		}
	}
	for key, sequence := range sequences {
		for _, shot := range listOf(sequence["shot_keys"]) {
			if shots[str(shot)] == nil {
				return nil, contentError("sequences/"+key, "missing shot")
			}
		}
	}
	for key, shot := range shots {
		selectedTakes := 0
		for _, take := range listOf(shot["take_keys"]) {
			if t := takes[str(take)]; t == nil || str(t["shot_key"]) != key {
				return nil, contentError("shots/"+key, "missing or foreign take")
			} else if t["status"] == "selected" {
				selectedTakes++
			}
		}
		if selectedTakes > 1 {
			return nil, contentError("shots/"+key, "multiple selected takes")
		}
	}
	for key, take := range takes {
		shot := shots[str(take["shot_key"])]
		if shot == nil {
			return nil, contentError("takes/"+key, "missing shot")
		}
		found := false
		for _, listed := range listOf(shot["take_keys"]) {
			if str(listed) == key {
				found = true
			}
		}
		if !found {
			return nil, contentError("takes/"+key, "not listed by shot")
		}
		if run := objectOf(take["run_ref"]); run != nil {
			found := false
			for _, listed := range m.RunRefs {
				if listed.ServiceID == str(run["service_id"]) && listed.RunID == str(run["run_id"]) {
					found = true
				}
			}
			if !found {
				return nil, contentError("takes/"+key, "run not in manifest")
			}
		}
	}
	for key, edit := range edits {
		if parent := str(edit["parent_key"]); parent != "" && edits[parent] == nil {
			return nil, contentError("edits/"+key, "missing parent")
		}
		cursor := float64(0)
		for _, rawItem := range listOf(edit["items"]) {
			item := objectOf(rawItem)
			take := takes[str(item["take_key"])]
			if take == nil || str(take["file_id"]) == "" {
				return nil, contentError("edits/"+key, "missing take")
			}
			if str(objectOf(item["media_ref"])["file_id"]) != str(take["file_id"]) {
				return nil, contentError("edits/"+key, "media differs from selected take")
			}
			start, in, out := numeric(item["timeline_in_frame"]), numeric(item["source_in_frame"]), numeric(item["source_out_frame"])
			if start != cursor || out <= in || out-in > math.MaxInt64-cursor {
				return nil, contentError("edits/"+key, "invalid timeline")
			}
			cursor += out - in
		}
	}
	for key, variant := range variants {
		if edit := str(variant["edit_key"]); edit != "" {
			target := edits[edit]
			if target == nil {
				return nil, contentError("delivery_variants/"+key, "missing edit")
			}
			end := float64(0)
			for _, item := range listOf(target["items"]) {
				clip := objectOf(item)
				end += numeric(clip["source_out_frame"]) - numeric(clip["source_in_frame"])
			}
			if end != numeric(variant["target_frames"]) {
				return nil, contentError("delivery_variants/"+key, "duration differs from edit")
			}
		}
	}
	for key, review := range reviews {
		targets := edits
		if review["target_kind"] == "package" {
			targets = packages
		}
		if targets[str(review["target_key"])] == nil {
			return nil, contentError("reviews/"+key, "missing target")
		}
	}
	for key, pack := range packages {
		if variants[str(pack["variant_key"])] == nil || edits[str(pack["edit_key"])] == nil {
			return nil, contentError("packages/"+key, "missing variant or edit")
		}
		if str(variants[str(pack["variant_key"])]["edit_key"]) != str(pack["edit_key"]) {
			return nil, contentError("packages/"+key, "variant edit differs")
		}
		if pack["state"] == "delivered" && len(listOf(pack["attachment_file_ids"])) == 0 {
			return nil, contentError("packages/"+key, "delivery receipt is missing")
		}
		approved, err := creativeLatestApprovals(key, pack, reviews)
		if err != nil {
			return nil, err
		}
		if pack["state"] == "ready" || pack["state"] == "delivered" {
			for _, kind := range []string{"creative", "rights", "technical", "client"} {
				if !approved[kind] {
					return nil, contentError("packages/"+key, "missing approved "+kind+" review")
				}
			}
		}
	}
	return creative, nil
}

func creativeLatestApprovals(key string, pack contentObject, reviews map[string]contentObject) (map[string]bool, error) {
	approved := map[string]bool{}
	for _, review := range listOf(pack["review_keys"]) {
		r := reviews[str(review)]
		if r == nil || r["target_kind"] != "package" || r["target_key"] != key {
			return nil, contentError("packages/"+key, "missing review")
		}
		approved[str(r["kind"])] = r["state"] == "approved"
	}
	return approved, nil
}

func numeric(value any) float64 {
	n, _ := value.(float64)
	return n
}

func (s *Service) checkCreativeFixedFile(ctx context.Context, p Principal, m Manifest, raw any) error {
	ref := objectOf(raw)
	basis, fileID := str(ref["basis_revision_id"]), str(ref["file_id"])
	snapshot, _, err := revision(ctx, s.db, p.OrganizationID, m.ProjectID, basis)
	if err != nil {
		return err
	}
	content := objectOf(ref["content_ref"])
	want := ContentRef{StoreID: str(content["store_id"]), ArtifactID: str(content["artifact_id"]), VersionID: str(content["version_id"])}
	for _, file := range snapshot.Files {
		if file.ID == fileID && file.Content == want {
			return nil
		}
	}
	return contentError("fixed_file", "file or content version differs from basis revision")
}

func (s *Service) readCreativeRevision(ctx context.Context, p Principal, m Manifest, revisionID string) (contentObject, error) {
	for _, document := range m.DomainDocuments {
		if document.Type != "project-content" {
			continue
		}
		for _, file := range m.Files {
			if file.ID == document.FileID {
				data, err := s.content.Read(ctx, p, file.Content, Access{ProjectID: m.ProjectID, RevisionID: revisionID}, MaxManifestBytes)
				if err != nil {
					return nil, err
				}
				var root contentObject
				if json.Unmarshal(data, &root) != nil {
					return nil, ErrInvalid
				}
				return objectOf(root["creative_video"]), nil
			}
		}
	}
	return nil, nil
}

func creativeHistory(prior, current contentObject) error {
	oldVariants, err := indexObjects(prior["delivery_variants"], "delivery_variants")
	if err != nil {
		return err
	}
	newVariants, err := indexObjects(current["delivery_variants"], "delivery_variants")
	if err != nil {
		return err
	}
	for _, raw := range listOf(prior["packages"]) {
		key := str(objectOf(raw)["variant_key"])
		if oldVariants[key] == nil || newVariants[key] == nil || !contentEqual(oldVariants[key], newVariants[key]) {
			return contentError("delivery_variants/"+key, "referenced delivery variant was rewritten")
		}
	}
	for _, name := range []string{"takes", "edits", "packages"} {
		old, err := indexObjects(prior[name], name)
		if err != nil {
			return err
		}
		now, err := indexObjects(current[name], name)
		if err != nil {
			return err
		}
		for key, item := range old {
			next := now[key]
			if next == nil {
				return contentError(name+"/"+key, "fixed version was removed or rewritten")
			}
			field := "state"
			if name == "takes" {
				field = "status"
			}
			if !creativeStateProgress(name, str(item[field]), str(next[field])) {
				return contentError(name+"/"+key, "state cannot move backward or change decision")
			}
			before, after := copyContent(item), copyContent(next)
			if name == "takes" && before["file_id"] == nil && after["file_id"] != nil && str(item["status"]) == "candidate" {
				// The first completed candidate may fill its previously empty media slot.
				before["file_id"] = after["file_id"]
			}
			delete(before, "status")
			delete(after, "status")
			delete(before, "state")
			delete(after, "state")
			if name == "packages" {
				previousReviews, newReviews := listOf(before["review_keys"]), listOf(after["review_keys"])
				if len(newReviews) < len(previousReviews) || !contentEqual(previousReviews, newReviews[:len(previousReviews)]) || len(newReviews) > len(previousReviews) && str(item["state"]) != "candidate" {
					return contentError(name+"/"+key, "review history was removed or reordered")
				}
				delete(before, "review_keys")
				delete(after, "review_keys")
				previousAttachments, newAttachments := listOf(before["attachment_file_ids"]), listOf(after["attachment_file_ids"])
				if len(newAttachments) < len(previousAttachments) || !contentEqual(previousAttachments, newAttachments[:len(previousAttachments)]) || len(newAttachments) > len(previousAttachments) && (str(item["state"]) != "ready" || str(next["state"]) != "delivered") {
					return contentError(name+"/"+key, "delivery evidence was removed or changed outside delivery")
				}
				delete(before, "attachment_file_ids")
				delete(after, "attachment_file_ids")
			}
			if !contentEqual(before, after) {
				return contentError(name+"/"+key, "fixed version was removed or rewritten")
			}
		}
	}
	oldReviews, err := indexObjects(prior["reviews"], "reviews")
	if err != nil {
		return err
	}
	newReviews, err := indexObjects(current["reviews"], "reviews")
	if err != nil {
		return err
	}
	priorPackages, err := indexObjects(prior["packages"], "packages")
	if err != nil {
		return err
	}
	for key, previous := range oldReviews {
		next := newReviews[key]
		if next == nil {
			return contentError("reviews/"+key, "review history was removed")
		}
		if pack := priorPackages[str(previous["target_key"])]; previous["target_kind"] == "package" && pack != nil && pack["state"] != "candidate" && !contentEqual(previous, next) {
			return contentError("reviews/"+key, "ready package review changed")
		}
		if previous["state"] == "approved" || previous["state"] == "changes_requested" || previous["state"] == "rejected" {
			if !contentEqual(previous, next) {
				return contentError("reviews/"+key, "decision was rewritten")
			}
			continue
		}
		before, after := copyContent(previous), copyContent(next)
		delete(before, "state")
		delete(after, "state")
		delete(before, "decision_by")
		delete(after, "decision_by")
		delete(before, "record_file_id")
		delete(after, "record_file_id")
		if !contentEqual(before, after) || previous["state"] == "in_review" && next["state"] == "pending" {
			return contentError("reviews/"+key, "review target or state was rewritten")
		}
	}
	return nil
}

func creativeFixedMediaFiles(prior contentObject, before, after Manifest) error {
	oldFiles, newFiles := map[string]ContentRef{}, map[string]ContentRef{}
	for _, file := range before.Files {
		oldFiles[file.ID] = file.Content
	}
	for _, file := range after.Files {
		newFiles[file.ID] = file.Content
	}
	check := func(id, path string) error {
		if id == "" {
			return nil
		}
		oldRef, foundOld := oldFiles[id]
		newRef, foundNew := newFiles[id]
		if !foundOld || !foundNew || oldRef != newRef {
			return contentError(path, "fixed file content was replaced")
		}
		return nil
	}
	for _, raw := range listOf(prior["takes"]) {
		item := objectOf(raw)
		if err := check(str(item["file_id"]), "takes/"+str(item["key"])); err != nil {
			return err
		}
	}
	for _, raw := range listOf(prior["edits"]) {
		item := objectOf(raw)
		if err := check(str(item["project_file_id"]), "edits/"+str(item["key"])); err != nil {
			return err
		}
	}
	for _, raw := range listOf(prior["packages"]) {
		item := objectOf(raw)
		path := "packages/" + str(item["key"])
		if err := check(str(item["media_file_id"]), path); err != nil {
			return err
		}
		for _, attachment := range listOf(item["attachment_file_ids"]) {
			if err := check(str(attachment), path); err != nil {
				return err
			}
		}
	}
	for _, raw := range listOf(prior["reviews"]) {
		item := objectOf(raw)
		if err := check(str(item["record_file_id"]), "reviews/"+str(item["key"])); err != nil {
			return err
		}
	}
	return nil
}

func creativeDecisionActor(prior, current contentObject, actor string) error {
	old, err := indexObjects(prior["reviews"], "reviews")
	if err != nil {
		return err
	}
	now, err := indexObjects(current["reviews"], "reviews")
	if err != nil {
		return err
	}
	for key, review := range now {
		state := str(review["state"])
		if state != "approved" && state != "changes_requested" && state != "rejected" {
			continue
		}
		if contentEqual(old[key], review) {
			continue
		}
		if str(review["decision_by"]) != actor || str(review["record_file_id"]) == "" {
			return contentError("reviews/"+key, "decision must be recorded by the current actor")
		}
	}
	return nil
}

func creativeStateProgress(name, before, after string) bool {
	if before == after {
		return true
	}
	switch name {
	case "takes":
		return before == "candidate" && (after == "selected" || after == "rejected" || after == "failed") || before == "selected" && after == "rejected"
	case "edits":
		return before == "rough" && (after == "review" || after == "picture_locked") || before == "review" && after == "picture_locked"
	case "packages":
		return before == "candidate" && (after == "ready" || after == "delivered") || before == "ready" && after == "delivered"
	}
	return false
}

func copyContent(source contentObject) contentObject {
	copy := make(contentObject, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func (s *Service) checkCreativeRevision(ctx context.Context, p Principal, m Manifest, base string, data []byte) error {
	if m.Category != "creative_video" {
		return nil
	}
	if len(data) == 0 {
		if base != "" {
			old, _, err := revision(ctx, s.db, p.OrganizationID, m.ProjectID, base)
			if err != nil {
				return err
			}
			for _, document := range old.DomainDocuments {
				if document.Type == "project-content" {
					return contentError("project-content", "existing creative document cannot be removed")
				}
			}
		}
		return nil
	}
	creative, err := creativeShape(data, m)
	if err != nil {
		return err
	}
	var prior contentObject
	if base != "" {
		old, _, err := revision(ctx, s.db, p.OrganizationID, m.ProjectID, base)
		if err != nil {
			return err
		}
		prior, err = s.readCreativeRevision(ctx, p, old, base)
		if err != nil {
			return err
		}
		if err := creativeHistory(prior, creative); err != nil {
			return err
		}
		if err := creativeFixedMediaFiles(prior, old, m); err != nil {
			return err
		}
		if err := creativeDecisionActor(prior, creative, p.SubjectID); err != nil {
			return err
		}
	} else if err := creativeDecisionActor(nil, creative, p.SubjectID); err != nil {
		return err
	}
	if err := s.checkCreativeReviewBases(ctx, p, m, creative); err != nil {
		return err
	}
	oldEdits, err := indexObjects(prior["edits"], "edits")
	if err != nil {
		return err
	}
	if err := creativeNewEditTakes(creative, oldEdits); err != nil {
		return err
	}
	for _, raw := range listOf(creative["concepts"]) {
		if fixed := objectOf(objectOf(raw)["basis_brief_ref"]); fixed != nil {
			if err := s.checkCreativeFixedFile(ctx, p, m, fixed); err != nil {
				return err
			}
		}
	}
	for _, raw := range listOf(creative["edits"]) {
		edit := objectOf(raw)
		for _, rawItem := range listOf(edit["items"]) {
			if err := s.checkCreativeFixedFile(ctx, p, m, objectOf(rawItem)["media_ref"]); err != nil {
				return err
			}
		}
		music := objectOf(edit["music_source"])
		if oldEdits[str(edit["key"])] != nil {
			continue
		}
		if !creativeMusicCleared(creative, music, "", "") {
			return contentError("edits/"+str(edit["key"]), "music use has no cleared right")
		}
		if err := s.checkCreativeMusicSource(ctx, p, m, music, true); err != nil {
			return err
		}
	}
	oldPackages, err := indexObjects(prior["packages"], "packages")
	if err != nil {
		return err
	}
	edits, err := indexObjects(creative["edits"], "edits")
	if err != nil {
		return err
	}
	variants, err := indexObjects(creative["delivery_variants"], "delivery_variants")
	if err != nil {
		return err
	}
	for _, raw := range listOf(creative["packages"]) {
		pack := objectOf(raw)
		state := str(pack["state"])
		if state != "ready" && state != "delivered" || oldPackages[str(pack["key"])] != nil && oldPackages[str(pack["key"])]["state"] == pack["state"] {
			continue
		}
		music := objectOf(edits[str(pack["edit_key"])]["music_source"])
		variant := variants[str(pack["variant_key"])]
		channel, territory := str(variant["channel"]), str(variant["territory"])
		if music != nil && (channel == "" || territory == "") {
			return contentError("packages/"+str(pack["key"]), "music delivery needs a channel and territory")
		}
		if !creativeMusicCleared(creative, music, channel, territory) {
			return contentError("packages/"+str(pack["key"]), "music use has no current cleared right")
		}
		if err := s.checkCreativeMusicSource(ctx, p, m, music, true); err != nil {
			return err
		}
	}
	if contentEqual(prior["music_source"], creative["music_source"]) {
		return nil
	}
	return s.checkCreativeMusicSource(ctx, p, m, objectOf(creative["music_source"]), false)
}

func creativeNewEditTakes(creative contentObject, oldEdits map[string]contentObject) error {
	takes, err := indexObjects(creative["takes"], "takes")
	if err != nil {
		return err
	}
	for _, raw := range listOf(creative["edits"]) {
		edit := objectOf(raw)
		if oldEdits[str(edit["key"])] != nil {
			continue
		}
		for _, rawItem := range listOf(edit["items"]) {
			if take := takes[str(objectOf(rawItem)["take_key"])]; take == nil || take["status"] != "selected" {
				return contentError("edits/"+str(edit["key"]), "new edit requires selected takes")
			}
		}
	}
	return nil
}

func creativeMusicRightKey(source contentObject) string {
	if source == nil {
		return ""
	}
	ref := objectOf(source["content_ref"])
	parts := []string{str(source["kind"]), str(source["project_id"]), str(source["revision_id"]), str(source["asset_id"]), str(source["asset_version_id"]), str(source["file_id"]), str(ref["store_id"]), str(ref["artifact_id"]), str(ref["version_id"])}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(parts, "|"))))
}

func creativeMusicCleared(creative, source contentObject, channel, territory string) bool {
	if source == nil {
		return true
	}
	identity := creativeMusicRightKey(source)
	today := time.Now().UTC().Format("2006-01-02")
	for _, raw := range listOf(creative["rights"]) {
		right := objectOf(raw)
		expiry := str(right["expires_on"])
		if expiry != "" {
			parsed, err := time.Parse("2006-01-02", expiry)
			if err != nil || parsed.Format("2006-01-02") != expiry || expiry < today {
				continue
			}
		}
		if right["subject_kind"] == "music" && str(right["subject_key"]) == identity && right["status"] == "cleared" && str(right["evidence_file_id"]) != "" && creativeContains(right["uses"], "music_sync") && len(listOf(right["territories"])) > 0 && len(listOf(right["channels"])) > 0 && (channel == "" || creativeContains(right["channels"], channel)) && (territory == "" || creativeContains(right["territories"], territory)) {
			return true
		}
	}
	return false
}

func creativeContains(values any, wanted string) bool {
	for _, value := range listOf(values) {
		if str(value) == wanted {
			return true
		}
	}
	return false
}

func creativeMusicSelected(source contentObject, origin Manifest) bool {
	fileID, purpose := str(source["file_id"]), str(source["selection_purpose"])
	if purpose == "" {
		return false
	}
	for _, selection := range origin.Selections {
		if selection.Purpose == purpose {
			for _, selected := range selection.FileIDs {
				if selected == fileID {
					return true
				}
			}
		}
	}
	return false
}

func (s *Service) checkCreativeReviewBases(ctx context.Context, p Principal, m Manifest, creative contentObject) error {
	cache := map[string]contentObject{}
	for _, raw := range listOf(creative["reviews"]) {
		review := objectOf(raw)
		basis := str(review["basis_revision_id"])
		prior, found := cache[basis]
		if !found {
			snapshot, _, err := revision(ctx, s.db, p.OrganizationID, m.ProjectID, basis)
			if err != nil {
				return err
			}
			if snapshot.Category != "creative_video" {
				return contentError("reviews/"+str(review["key"]), "basis project type differs")
			}
			prior, err = s.readCreativeRevision(ctx, p, snapshot, basis)
			if err != nil {
				return err
			}
			cache[basis] = prior
		}
		if !creativeReviewTargetExists(review, prior) {
			return contentError("reviews/"+str(review["key"]), "target absent from basis revision")
		}
	}
	return nil
}

func creativeReviewTargetExists(review, prior contentObject) bool {
	collection := "edits"
	if review["target_kind"] == "package" {
		collection = "packages"
	}
	for _, raw := range listOf(prior[collection]) {
		if str(objectOf(raw)["key"]) == str(review["target_key"]) {
			return true
		}
	}
	return false
}

func (s *Service) checkCreativeMusicSource(ctx context.Context, p Principal, m Manifest, source contentObject, requireSelected bool) error {
	if source == nil {
		return nil
	}
	if source["kind"] == "world_asset" {
		for _, asset := range m.AssetRefs {
			if asset.AssetID == str(source["asset_id"]) && asset.VersionID == str(source["asset_version_id"]) {
				if requireSelected {
					return contentError("music_source", "world music file cannot yet be verified for an edit")
				}
				return nil
			}
		}
		return contentError("music_source", "asset version not in manifest")
	}
	projectID, revisionID := str(source["project_id"]), str(source["revision_id"])
	if _, err := role(ctx, s.db, p, projectID); err != nil {
		return err
	}
	origin, _, err := revision(ctx, s.db, p.OrganizationID, projectID, revisionID)
	if err != nil {
		return err
	}
	if origin.Category != "music" {
		return contentError("music_source", "source project is not music")
	}
	if str(source["selection_purpose"]) != "" && !creativeMusicSelected(source, origin) {
		return contentError("music_source", "source file is not selected in its fixed revision")
	}
	ref := objectOf(source["content_ref"])
	want := ContentRef{StoreID: str(ref["store_id"]), ArtifactID: str(ref["artifact_id"]), VersionID: str(ref["version_id"])}
	for _, file := range origin.Files {
		if file.ID == str(source["file_id"]) && file.Content == want {
			version, err := s.content.Metadata(ctx, p, want, Access{ProjectID: projectID, RevisionID: revisionID})
			if err != nil {
				return err
			}
			kind, _, err := mime.ParseMediaType(version.MIME)
			if err != nil || !strings.HasPrefix(kind, "audio/") || version.Size <= 0 {
				return contentError("music_source", "source file is not audio")
			}
			return nil
		}
	}
	return contentError("music_source", "file or content version does not match source revision")
}
