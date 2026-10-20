package project

import (
	"context"
	"encoding/json"
	"reflect"
)

// imageShape validates the same fixed references used by the Dashboard. Display
// selection is presentation metadata and never confers approval on an output.
func imageShape(data []byte, m Manifest) (contentObject, error) {
	var root contentObject
	if json.Unmarshal(data, &root) != nil {
		return nil, contentError("image", "invalid JSON")
	}
	schema, err := contentSchema()
	if err != nil {
		return nil, err
	}
	if root["category"] != "image" || m.Category != "image" {
		return nil, contentError("image", "category mismatch")
	}
	if err := schema.Validate(root); err != nil {
		return nil, contentError("image", "schema validation failed")
	}
	files := map[string]File{}
	for _, f := range m.Files {
		files[f.ID] = f
	}
	if err := checkCurrentFiles(root, files, "image"); err != nil {
		return nil, err
	}
	body := objectOf(root["image"])
	works, err := indexObjects(body["works"], "image/works")
	if err != nil {
		return nil, err
	}
	for _, work := range works {
		versions, err := indexObjects(work["versions"], "image/versions")
		if err != nil {
			return nil, err
		}
		if display := str(work["display_version_key"]); display != "" && versions[display] == nil {
			return nil, contentError("image", "unknown display version")
		}
		for k, version := range versions {
			seenFiles := map[string]bool{}
			for _, raw := range listOf(version["file_refs"]) {
				id := str(objectOf(raw)["file_id"])
				if seenFiles[id] {
					return nil, contentError("image", "duplicate version file")
				}
				seenFiles[id] = true
			}
			seen := map[string]bool{}
			for current := k; current != ""; current = str(versions[current]["parent_version_key"]) {
				if seen[current] || versions[current] == nil {
					return nil, contentError("image", "missing or cyclic parent version")
				}
				seen[current] = true
			}
		}
	}
	return body, nil
}

func imageHistory(prior, current contentObject, old, next Manifest) error {
	works, err := indexObjects(current["works"], "image/works")
	if err != nil {
		return err
	}
	before, after := map[string]ContentRef{}, map[string]ContentRef{}
	for _, f := range old.Files {
		before[f.ID] = f.Content
	}
	for _, f := range next.Files {
		after[f.ID] = f.Content
	}
	for _, raw := range listOf(prior["works"]) {
		work := objectOf(raw)
		versions, err := indexObjects(works[str(work["key"])]["versions"], "image/versions")
		if err != nil {
			return err
		}
		for _, raw := range listOf(work["versions"]) {
			v := objectOf(raw)
			if !reflect.DeepEqual(v, versions[str(v["key"])]) {
				return contentError("image", "registered version removed or rewritten")
			}
			ids := append(append([]any{}, listOf(v["reference_file_ids"])...), listOf(v["prompt_file_ids"])...)
			for _, raw := range listOf(v["file_refs"]) {
				ids = append(ids, objectOf(raw)["file_id"])
			}
			for _, raw := range ids {
				id := str(raw)
				if before[id] != after[id] {
					return contentError("image", "registered version bytes replaced")
				}
			}
		}
	}
	return nil
}

func (s *Service) checkImageRevision(ctx context.Context, p Principal, m Manifest, base string, data []byte) error {
	if m.Category != "image" {
		return nil
	}
	var current contentObject
	if len(data) > 0 {
		var err error
		current, err = imageShape(data, m)
		if err != nil {
			return err
		}
	}
	if base == "" {
		return nil
	}
	old, _, err := revision(ctx, s.db, p.OrganizationID, m.ProjectID, base)
	if err != nil {
		return err
	}
	for _, document := range old.DomainDocuments {
		if document.Type != "project-content" {
			continue
		}
		if len(data) == 0 {
			return contentError("image", "existing image document cannot be removed")
		}
		for _, f := range old.Files {
			if f.ID != document.FileID {
				continue
			}
			bytes, err := s.content.Read(ctx, p, f.Content, Access{ProjectID: m.ProjectID, RevisionID: base}, MaxManifestBytes)
			if err != nil {
				return err
			}
			prior, err := imageShape(bytes, old)
			if err != nil {
				return err
			}
			return imageHistory(prior, current, old, m)
		}
	}
	return nil
}
