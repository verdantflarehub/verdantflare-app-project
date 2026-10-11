package project

import (
	"encoding/json"
	"sort"
)

func normalizeWorld(r WorldRegisterRequest) (WorldRegisterRequest, error) {
	if !ValidID(r.CommitID) || !ValidID(r.SourceProjectID) || !ValidID(r.SourceRevisionID) || !validText(r.Name) || !tokenPattern.MatchString(r.AssetType) || r.Subjects == nil || len(r.Subjects) > 256 || len(r.FileIDs) == 0 || len(r.FileIDs) > MaxFiles || (r.Relation != "produced_in" && r.Relation != "curated_in") {
		return r, ErrInvalid
	}
	if (r.AssetID == "") != (r.ExpectedVersionID == "") || (r.AssetID != "" && (!ValidID(r.AssetID) || !ValidID(r.ExpectedVersionID))) || (r.ReviewFileID != "" && !ValidID(r.ReviewFileID)) {
		return r, ErrInvalid
	}
	r.FileIDs = append([]string{}, r.FileIDs...)
	sort.Strings(r.FileIDs)
	for i, v := range r.FileIDs {
		if !ValidID(v) || (i > 0 && r.FileIDs[i-1] == v) {
			return r, ErrInvalid
		}
	}
	r.Subjects = append([]string{}, r.Subjects...)
	sort.Strings(r.Subjects)
	for i, v := range r.Subjects {
		if !validText(v) || (i > 0 && r.Subjects[i-1] == v) {
			return r, ErrInvalid
		}
	}
	deps := []AssetRef{}
	if r.DependsOn != nil {
		deps = append(deps, (*r.DependsOn)...)
	}
	sort.Slice(deps, func(i, j int) bool {
		a, b := deps[i], deps[j]
		if a.AssetID != b.AssetID {
			return a.AssetID < b.AssetID
		}
		if a.VersionID != b.VersionID {
			return a.VersionID < b.VersionID
		}
		return a.Purpose < b.Purpose
	})
	r.DependsOn = &deps
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > MaxManifestBytes {
		return r, ErrInvalid
	}
	return r, nil
}
