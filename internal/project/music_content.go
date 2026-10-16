package project

import (
	"context"
	"encoding/json"
)

func musicOwners(music contentObject) map[string]contentObject {
	out := map[string]contentObject{}
	for _, group := range []string{"models", "songs", "mvs"} {
		values := music[group]
		if group == "models" {
			values = objectOf(music["voice"])[group]
		}
		for _, value := range listOf(values) {
			item := objectOf(value)
			out[group+"/"+str(item["key"])] = item
		}
	}
	return out
}

// Legacy music documents remain readable. Production metadata opts into the
// central schema and fixed version checks; sources belong to their own revision.
func decodeMusicContent(data []byte, m Manifest) (contentObject, error) {
	var root contentObject
	if json.Unmarshal(data, &root) != nil {
		return nil, contentError("music", "invalid JSON")
	}
	music := objectOf(root["music"])
	owners := musicOwners(music)
	hasProduction := false
	for _, owner := range owners {
		if _, ok := owner["production"]; ok {
			hasProduction = true
		}
	}
	if !hasProduction {
		return music, nil
	}
	for _, group := range []string{"models", "songs", "mvs", "recordings"} {
		values := music[group]
		if group == "models" || group == "recordings" {
			values = objectOf(music["voice"])[group]
		}
		if _, err := indexObjects(values, "music/"+group); err != nil {
			return nil, err
		}
	}
	if root["category"] != "music" {
		return nil, contentError("category", "does not match project")
	}
	schema, err := contentSchema()
	if err != nil {
		return nil, err
	}
	if schema.Validate(root) != nil {
		return nil, contentError("music", "schema validation failed")
	}
	files := map[string]File{}
	for _, f := range m.Files {
		files[f.ID] = f
	}
	for path, owner := range owners {
		p := objectOf(owner["production"])
		if p == nil {
			continue
		}
		versions, err := indexObjects(p["versions"], path)
		if err != nil {
			return nil, err
		}
		if selected := str(p["selected_version_key"]); selected != "" && versions[selected] == nil {
			return nil, contentError(path, "selected version missing")
		}
		if err := checkGraph(versions, "parent_version_key", path); err != nil {
			return nil, err
		}
		for _, v := range versions {
			kind := str(v["kind"])
			if (len(path) >= 7 && path[:7] == "models/" && kind != "model") || (len(path) >= 4 && path[:4] == "mvs/" && kind != "video") || (len(path) >= 6 && path[:6] == "songs/" && kind != "demo" && kind != "audio" && kind != "master") {
				return nil, contentError(path, "wrong version kind")
			}
			seen, output := map[string]bool{}, false
			for _, ref := range listOf(v["file_refs"]) {
				f := objectOf(ref)
				id := str(f["file_id"])
				if seen[id] {
					return nil, contentError(path, "duplicate version file")
				}
				seen[id] = true
				if err := checkCurrentFiles(f, files, path); err != nil {
					return nil, err
				}
				output = output || str(f["purpose"]) == "output"
			}
			if !output {
				return nil, contentError(path, "version has no output")
			}
			if err := checkCurrentFiles(v["review"], files, path); err != nil {
				return nil, err
			}
			for _, value := range listOf(v["sources"]) {
				s := objectOf(value)
				sourceKind := str(s["kind"])
				if (kind == "model" && sourceKind != "recording") || (kind == "video" && sourceKind != "song") || (kind != "model" && kind != "video" && sourceKind == "song") {
					return nil, contentError(path, "wrong source kind")
				}
			}
		}
	}
	return music, nil
}

func checkMusicHistory(old, current contentObject) error {
	after := musicOwners(current)
	for path, item := range musicOwners(old) {
		versions, _ := indexObjects(objectOf(after[path]["production"])["versions"], path)
		for _, value := range listOf(objectOf(item["production"])["versions"]) {
			v := objectOf(value)
			next := versions[str(v["key"])]
			if next == nil {
				return contentError(path, "registered version cannot be removed")
			}
			for k, field := range v {
				if k != "label" && k != "review" && !contentEqual(field, next[k]) {
					return contentError(path, "registered version is immutable")
				}
			}
		}
	}
	return nil
}

func (s *Service) readMusicRevision(ctx context.Context, p Principal, m Manifest, revisionID string) (contentObject, error) {
	for _, d := range m.DomainDocuments {
		if d.Type == "project-content" {
			for _, f := range m.Files {
				if f.ID == d.FileID {
					data, err := s.content.Read(ctx, p, f.Content, Access{ProjectID: m.ProjectID, RevisionID: revisionID}, MaxManifestBytes)
					if err != nil {
						return nil, err
					}
					return decodeMusicContent(data, m)
				}
			}
		}
	}
	return nil, nil
}

func musicSourceItem(music, source contentObject) contentObject {
	kind, item := str(source["kind"]), str(source["item_key"])
	if kind != "recording" {
		group := "models"
		if kind == "song" {
			group = "songs"
		}
		return musicOwners(music)[group+"/"+item]
	}
	for _, value := range listOf(objectOf(music["voice"])["recordings"]) {
		if objectOf(value)["key"] == item {
			return objectOf(value)
		}
	}
	return nil
}

func (s *Service) checkMusicRevision(ctx context.Context, p Principal, m Manifest, base string, data []byte) error {
	var current contentObject
	var err error
	if len(data) > 0 {
		current, err = decodeMusicContent(data, m)
		if err != nil {
			return err
		}
	}
	var prior contentObject
	if base != "" {
		old, _, e := revision(ctx, s.db, p.OrganizationID, m.ProjectID, base)
		if e != nil {
			return e
		}
		prior, err = s.readMusicRevision(ctx, p, old, base)
		if err != nil {
			return err
		}
	}
	if err := checkMusicHistory(prior, current); err != nil {
		return err
	}
	before := musicOwners(prior)
	for path, item := range musicOwners(current) {
		old, _ := indexObjects(objectOf(before[path]["production"])["versions"], path)
		for _, value := range listOf(objectOf(item["production"])["versions"]) {
			v := objectOf(value)
			if old[str(v["key"])] != nil {
				continue
			}
			if base == "" || str(v["basis_revision_id"]) != base || before[path] == nil {
				return contentError(path, "new version must use opened base revision")
			}
			for _, value := range listOf(v["sources"]) {
				source := objectOf(value)
				opened, e := s.Open(ctx, p, str(source["project_id"]), str(source["revision_id"]))
				if e != nil {
					return e
				}
				if opened.Manifest.Category != "music" {
					return contentError(path, "source is not music")
				}
				music, e := s.readMusicRevision(ctx, p, opened.Manifest, opened.RevisionID)
				if e != nil {
					return e
				}
				owner := musicSourceItem(music, source)
				if owner == nil {
					return contentError(path, "source object missing")
				}
				if versionKey := str(source["version_key"]); versionKey != "" {
					versions, _ := indexObjects(objectOf(owner["production"])["versions"], path)
					if versions[versionKey] == nil {
						return contentError(path, "source version missing")
					}
				} else if !hasString(owner["result_file_ids"], str(source["file_id"])) && !hasString(owner["file_ids"], str(source["file_id"])) {
					return contentError(path, "source file does not belong to object")
				}
				if fileID := str(source["file_id"]); fileID != "" {
					found := false
					for _, f := range opened.Manifest.Files {
						if f.ID == fileID {
							found = true
							break
						}
					}
					if !found {
						return contentError(path, "source file missing from fixed manifest")
					}
				}
			}
		}
	}
	return nil
}
