package project

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"

	"github.com/verdantflarehub/verdantflare-app-project/internal/strictjson"
)

// Spatial files remain ordinary fixed project artifacts. This optional closed
// document supplies explicit evidence mapping; names and extensions never do.
type reviewEvidenceDocument struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	Coverage      struct {
		CameraIDs        []string `json:"camera_ids"`
		RegionIDs        []string `json:"region_ids"`
		SpaceBasisFileID *string  `json:"space_basis_file_id"`
		Cameras          []struct {
			CameraID         string   `json:"camera_id"`
			ParametersFileID string   `json:"parameters_file_id"`
			EvidenceFileIDs  []string `json:"evidence_file_ids"`
			RegionIDs        []string `json:"region_ids"`
		} `json:"cameras"`
	} `json:"coverage"`
	Evidence []struct {
		FileID     string            `json:"file_id"`
		Role       string            `json:"role"`
		CameraIDs  []string          `json:"camera_ids"`
		RegionIDs  []string          `json:"region_ids"`
		FrameRange *ReviewFrameRange `json:"frame_range"`
	} `json:"evidence"`
	Checks []struct {
		Claim           string   `json:"claim"`
		EvidenceFileIDs []string `json:"evidence_file_ids"`
	} `json:"checks"`
}

func reviewMaterial(document publicationDocument, locator ReviewLocator) (contentObject, contentObject, contentObject, error) {
	var material, state, version contentObject
	d := objectOf(document.Root["ai_short_drama"])
	for _, value := range listOf(d["globals"]) {
		o := objectOf(value)
		if o["key"] == locator.MaterialKey {
			if material != nil {
				return nil, nil, nil, ErrInvalid
			}
			material = o
		}
	}
	if material == nil || material["kind"] != "location" {
		return nil, nil, nil, ErrNotFound
	}
	for _, value := range listOf(material["states"]) {
		o := objectOf(value)
		if o["key"] == locator.StateKey {
			state = o
		}
	}
	for _, value := range listOf(material["versions"]) {
		o := objectOf(value)
		if o["key"] == locator.VersionKey {
			version = o
		}
	}
	if locator.StateKey == "" || locator.VersionKey == "" || len(listOf(material["states"])) == 0 || len(listOf(material["versions"])) == 0 {
		return material, state, version, ErrLegacyTarget
	}
	if state == nil || version == nil || str(version["state_key"]) != locator.StateKey {
		return nil, nil, nil, ErrNotFound
	}
	return material, state, version, nil
}
func reviewProductionDefinition(material, state, version contentObject) contentObject {
	m := contentObject{}
	for _, key := range []string{"key", "kind", "description", "definition_refs", "input_global_keys", "asset_id", "asset_version_id"} {
		if v, ok := material[key]; ok {
			m[key] = v
		}
	}
	v := contentObject{}
	for k, value := range version {
		if k != "review" && k != "label" {
			v[k] = value
		}
	}
	return contentObject{"material": m, "state": reviewProductionValue(state), "version": reviewProductionValue(v)}
}
func reviewEvidenceRole(purpose string) string {
	switch purpose {
	case "source_engineering", "space_record", "lighting_record", "camera_reference", "render", "photograph", "generated", "composite", "set_package", "supporting":
		return purpose
	}
	return "supporting"
}
func (s *Service) buildReviewTarget(ctx context.Context, p Principal, document publicationDocument, locator ReviewLocator) (ReviewTarget, string, error) {
	material, state, version, err := reviewMaterial(document, locator)
	legacy := errors.Is(err, ErrLegacyTarget)
	if err != nil && !legacy {
		return ReviewTarget{}, "", err
	}
	target := ReviewTarget{SchemaVersion: 1, TargetLocator: locator, Definition: reviewProductionDefinition(material, state, version), Evidence: []ReviewEvidence{}, Dependencies: []ReviewDependency{}, Coverage: ReviewCoverage{CameraIDs: []string{}, RegionIDs: []string{}, Cameras: []ReviewCamera{}}, Requirements: []ReviewRequirement{}}
	access := Access{ProjectID: locator.ProjectID, RevisionID: locator.ContentRevisionID}
	source, err := s.content.Metadata(ctx, p, document.Opened.ManifestRef, access)
	if err != nil {
		return target, "", err
	}
	if source.ContentRef != document.Opened.ManifestRef || source.OrganizationID != p.OrganizationID || !reviewHashPattern.MatchString(source.SHA256) || source.Size < 1 {
		return target, "", ErrNotReady
	}
	sourceBytes, err := s.content.Read(ctx, p, document.Opened.ManifestRef, access, MaxManifestBytes)
	if err != nil {
		return target, "", err
	}
	sourceSum := sha256.Sum256(sourceBytes)
	if hex.EncodeToString(sourceSum[:]) != source.SHA256 || int64(len(sourceBytes)) != source.Size {
		return target, "", ErrTargetMismatch
	}
	target.SourceManifest = ReviewSource{ContentRef: source.ContentRef, SHA256: source.SHA256, SizeBytes: source.Size}
	files := map[string]File{}
	for _, f := range document.Opened.Manifest.Files {
		files[f.ID] = f
	}
	roles := map[string]string{}
	if legacy {
		for _, value := range listOf(material["file_ids"]) {
			roles[str(value)] = "supporting"
		}
		for _, value := range listOf(material["reference_file_ids"]) {
			roles[str(value)] = "supporting"
		}
	}
	for _, value := range listOf(version["file_refs"]) {
		f := objectOf(value)
		id := str(f["file_id"])
		var ref ContentRef
		b, _ := json.Marshal(f["content_ref"])
		if json.Unmarshal(b, &ref) != nil || files[id].Content != ref {
			return target, "", ErrTargetMismatch
		}
		roles[id] = reviewEvidenceRole(str(f["purpose"]))
	}
	for _, object := range []contentObject{material, state} {
		for _, value := range listOf(object["definition_refs"]) {
			id := str(objectOf(value)["file_id"])
			if _, ok := roles[id]; !ok {
				roles[id] = "supporting"
			}
		}
	}
	for _, value := range listOf(version["generation_record_file_ids"]) {
		id := str(value)
		if _, ok := roles[id]; !ok {
			roles[id] = "supporting"
		}
	}
	spatialID := str(version["spatial_record_file_id"])
	if spatialID != "" {
		roles[spatialID] = "space_record"
	}
	if id := str(version["lighting_record_file_id"]); id != "" {
		roles[id] = "lighting_record"
	}
	inputs, err := reviewInputGlobals(objectOf(document.Root["ai_short_drama"]), material)
	if err != nil {
		return target, "", err
	}
	target.Definition["input_globals"] = inputs
	for _, input := range inputs {
		if err := validateReviewFileRefs(input, files); err != nil {
			return target, "", err
		}
		collectReviewFiles(input, roles)
	}
	ids := []string{}
	for id := range roles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		file, ok := files[id]
		if !ok {
			return target, "", ErrNotFound
		}
		v, e := s.content.Metadata(ctx, p, file.Content, access)
		if e != nil {
			return target, "", e
		}
		if v.ContentRef != file.Content || v.OrganizationID != p.OrganizationID || !reviewHashPattern.MatchString(v.SHA256) || v.Size < 0 || v.Size > 9007199254740991 {
			return target, "", ErrNotReady
		}
		target.Evidence = append(target.Evidence, ReviewEvidence{EvidenceID: id, FileID: id, ContentRef: file.Content, SHA256: v.SHA256, SizeBytes: v.Size, MIME: v.MIME, Role: roles[id], CameraIDs: []string{}, RegionIDs: []string{}})
	}
	// Only the location's explicit fixed asset is relevant; unrelated project
	// references must not change a target when another scene is edited.
	dependencyOwners := append([]contentObject{material}, inputs...)
	seenDependencies := map[AssetRef]bool{}
	for _, dependencyOwner := range dependencyOwners {
		if asset, versionID := str(dependencyOwner["asset_id"]), str(dependencyOwner["asset_version_id"]); asset != "" && versionID != "" {
			found := false
			for _, a := range document.Opened.Manifest.AssetRefs {
				if a.AssetID == asset && a.VersionID == versionID {
					found = true
					if !seenDependencies[a] {
						target.Dependencies = append(target.Dependencies, ReviewDependency{AssetID: a.AssetID, VersionID: a.VersionID, Purpose: a.Purpose})
						seenDependencies[a] = true
					}
				}
			}
			if !found {
				return target, "", ErrTargetMismatch
			}
		}
	}
	sort.Slice(target.Dependencies, func(i, j int) bool {
		a, b := target.Dependencies[i], target.Dependencies[j]
		if a.AssetID != b.AssetID {
			return a.AssetID < b.AssetID
		}
		if a.VersionID != b.VersionID {
			return a.VersionID < b.VersionID
		}
		return a.Purpose < b.Purpose
	})
	var mapping *reviewEvidenceDocument
	if spatialID != "" {
		for _, e := range target.Evidence {
			if e.FileID != spatialID || e.MIME != "application/json" {
				continue
			}
			raw, readErr := s.content.Read(ctx, p, e.ContentRef, access, 1<<20)
			if readErr != nil {
				return target, "", readErr
			}
			sum := sha256.Sum256(raw)
			if hex.EncodeToString(sum[:]) != e.SHA256 {
				return target, "", ErrTargetMismatch
			}
			var header struct {
				Kind string `json:"kind"`
			}
			if json.Unmarshal(raw, &header) == nil && header.Kind == "set-review-evidence" {
				var value reviewEvidenceDocument
				if decodeReviewEvidence(raw, &value) != nil || value.SchemaVersion != 1 {
					return target, "", ErrInvalid
				}
				mapping = &value
			}
		}
	}
	if err = applyReviewEvidence(&target, mapping); err != nil {
		return target, "", err
	}
	if err = s.verifyReviewCameras(ctx, p, &target); err != nil {
		return target, "", err
	}
	_, productionHash, err := reviewJSON(contentObject{"definition": target.Definition, "evidence": target.Evidence, "dependencies": target.Dependencies, "coverage": target.Coverage, "requirements": target.Requirements})
	if err != nil {
		return target, "", err
	}
	if legacy {
		return target, productionHash, ErrLegacyTarget
	}
	return target, productionHash, nil
}

// Only collections have defaults; unknown fields and explicit null collections
// still fail the same closed-shape decoder used for externally supplied JSON.
func decodeReviewEvidence(raw []byte, value *reviewEvidenceDocument) error {
	data, err := strictjson.ReadValue(bytes.NewReader(raw), 1<<20)
	if err != nil {
		return ErrInvalid
	}
	if _, err = reviewCanonicalBytes(data); err != nil {
		return err
	}
	var root contentObject
	if json.Unmarshal(data, &root) != nil || root == nil {
		return ErrInvalid
	}
	valid := true
	defaults := func(object contentObject, names ...string) {
		for _, name := range names {
			if item, exists := object[name]; !exists {
				object[name] = []any{}
			} else if _, ok := item.([]any); !ok {
				valid = false
			}
		}
	}
	defaults(root, "evidence", "checks")
	if _, exists := root["coverage"]; !exists {
		root["coverage"] = contentObject{}
	}
	coverage := objectOf(root["coverage"])
	if coverage == nil {
		return ErrInvalid
	}
	defaults(coverage, "camera_ids", "region_ids", "cameras")
	if _, exists := coverage["space_basis_file_id"]; !exists {
		coverage["space_basis_file_id"] = nil
	}
	for _, item := range listOf(root["evidence"]) {
		object := objectOf(item)
		if object == nil {
			return ErrInvalid
		}
		defaults(object, "camera_ids", "region_ids")
		if _, exists := object["frame_range"]; !exists {
			object["frame_range"] = nil
		}
	}
	for _, item := range listOf(coverage["cameras"]) {
		object := objectOf(item)
		if object == nil {
			return ErrInvalid
		}
		defaults(object, "evidence_file_ids", "region_ids")
	}
	for _, item := range listOf(root["checks"]) {
		object := objectOf(item)
		if object == nil {
			return ErrInvalid
		}
		defaults(object, "evidence_file_ids")
	}
	if !valid {
		return ErrInvalid
	}
	data, err = json.Marshal(root)
	if err != nil {
		return ErrInvalid
	}
	return decodeReview(data, value)
}
func applyReviewEvidence(target *ReviewTarget, mapping *reviewEvidenceDocument) error {
	checks := map[string][]string{}
	if mapping != nil {
		var err error
		if mapping.Coverage.CameraIDs, err = reviewIDs(mapping.Coverage.CameraIDs); err != nil {
			return err
		}
		if mapping.Coverage.RegionIDs, err = reviewIDs(mapping.Coverage.RegionIDs); err != nil {
			return err
		}
		target.Coverage = ReviewCoverage{CameraIDs: mapping.Coverage.CameraIDs, RegionIDs: mapping.Coverage.RegionIDs, Cameras: []ReviewCamera{}}
		seen := map[string]bool{}
		for _, mapped := range mapping.Evidence {
			if seen[mapped.FileID] || reviewEvidenceRole(mapped.Role) != mapped.Role {
				return ErrInvalid
			}
			seen[mapped.FileID] = true
			cameras, e := reviewIDs(mapped.CameraIDs)
			if e != nil {
				return e
			}
			regions, e := reviewIDs(mapped.RegionIDs)
			if e != nil {
				return e
			}
			if !subsetReview(cameras, target.Coverage.CameraIDs) || !subsetReview(regions, target.Coverage.RegionIDs) {
				return ErrInvalid
			}
			found := false
			for i, item := range target.Evidence {
				if item.FileID == mapped.FileID {
					if f := mapped.FrameRange; f != nil && (!strings.HasPrefix(item.MIME, "video/") || f.Start < 0 || f.End < f.Start || f.FPSNumerator < 1 || f.FPSDenominator < 1) {
						return ErrInvalid
					}
					found = true
					item.Role = mapped.Role
					item.CameraIDs = cameras
					item.RegionIDs = regions
					item.FrameRange = mapped.FrameRange
					target.Evidence[i] = item
				}
			}
			if !found {
				return ErrInvalid
			}
		}
		for _, check := range mapping.Checks {
			if check.Claim != "space_layout" && check.Claim != "source_recovery" && check.Claim != "new_camera" {
				return ErrInvalid
			}
			if _, ok := checks[check.Claim]; ok {
				return ErrInvalid
			}
			ids, e := reviewIDs(check.EvidenceFileIDs)
			if e != nil || len(ids) == 0 {
				return ErrInvalid
			}
			checks[check.Claim] = ids
		}
		files := map[string]ReviewEvidence{}
		for _, e := range target.Evidence {
			files[e.FileID] = e
		}
		if mapping.Coverage.SpaceBasisFileID != nil {
			basis, ok := files[*mapping.Coverage.SpaceBasisFileID]
			if !ok {
				return ErrInvalid
			}
			target.Coverage.SpaceBasis = &ReviewSpaceBasis{EvidenceID: basis.EvidenceID, SHA256: basis.SHA256}
		}
		seenCameras := map[string]bool{}
		for _, camera := range mapping.Coverage.Cameras {
			if !includes(target.Coverage.CameraIDs, camera.CameraID) || seenCameras[camera.CameraID] {
				return ErrInvalid
			}
			seenCameras[camera.CameraID] = true
			parameters, ok := files[camera.ParametersFileID]
			if !ok || parameters.MIME != "application/json" {
				return ErrInvalid
			}
			ids, e := reviewIDs(camera.EvidenceFileIDs)
			if e != nil || len(ids) == 0 {
				return ErrInvalid
			}
			regions, e := reviewIDs(camera.RegionIDs)
			if e != nil || !subsetReview(regions, target.Coverage.RegionIDs) {
				return ErrInvalid
			}
			for _, id := range ids {
				e, ok := files[id]
				if !ok || !includes(e.CameraIDs, camera.CameraID) {
					return ErrInvalid
				}
			}
			target.Coverage.Cameras = append(target.Coverage.Cameras, ReviewCamera{CameraID: camera.CameraID, ParametersEvidenceID: parameters.EvidenceID, ParametersSHA256: parameters.SHA256, EvidenceIDs: ids, RegionIDs: regions, Verified: false})
		}
		sort.Slice(target.Coverage.Cameras, func(i, j int) bool { return target.Coverage.Cameras[i].CameraID < target.Coverage.Cameras[j].CameraID })
	}
	all := []string{}
	images := []string{}
	sources := []string{}
	spaces := []string{}
	cameras := []string{}
	for _, e := range target.Evidence {
		all = append(all, e.EvidenceID)
		if strings.HasPrefix(e.MIME, "image/") || strings.HasPrefix(e.MIME, "video/") {
			images = append(images, e.EvidenceID)
		}
		if e.Role == "source_engineering" {
			sources = append(sources, e.EvidenceID)
		}
		if e.Role == "space_record" {
			spaces = append(spaces, e.EvidenceID)
		}
		cameras = append(cameras, e.CameraIDs...)
	}
	for _, ids := range checks {
		if !subsetReview(ids, all) {
			return ErrInvalid
		}
	}
	for _, claim := range []string{"image_reading", "space_layout", "source_recovery", "new_camera", "full_set"} {
		r := ReviewRequirement{RequirementID: claim, Claim: claim, EvidenceIDs: []string{}, State: "missing", ReasonCode: "EVIDENCE_MISSING"}
		switch claim {
		case "image_reading":
			r.EvidenceIDs = images
			if len(images) > 0 {
				r.State = "satisfied"
				r.ReasonCode = ""
			}
		case "space_layout":
			r.EvidenceIDs = checks[claim]
			if len(spaces) > 0 {
				r.State = "unverified"
				r.ReasonCode = "SPACE_MAPPING_UNVERIFIED"
			}
		case "source_recovery":
			r.EvidenceIDs = checks[claim]
			if len(sources) > 0 {
				r.State = "unverified"
				r.ReasonCode = "RECOVERY_REPORT_UNVERIFIED"
			}
		case "new_camera":
			r.EvidenceIDs = checks[claim]
			if len(r.EvidenceIDs) > 0 && len(target.Coverage.CameraIDs) > 0 && subsetReview(target.Coverage.CameraIDs, cameras) {
				r.State = "unverified"
				r.ReasonCode = "NEW_CAMERA_REPORT_UNVERIFIED"
			}
		case "full_set":
			r.EvidenceIDs = all
			ready := len(target.Coverage.CameraIDs) > 0 && subsetReview(target.Coverage.CameraIDs, cameras) && len(target.Dependencies) == 0
			for _, required := range target.Requirements {
				if required.Claim != "new_camera" && required.State != "satisfied" {
					ready = false
				}
			}
			if ready {
				r.State = "satisfied"
				r.ReasonCode = ""
			} else {
				r.State = "unverified"
				r.ReasonCode = "FULL_SET_UNVERIFIED"
			}
		}
		if r.EvidenceIDs == nil {
			r.EvidenceIDs = []string{}
		}
		target.Requirements = append(target.Requirements, r)
	}
	sort.Slice(target.Requirements, func(i, j int) bool {
		return target.Requirements[i].RequirementID < target.Requirements[j].RequirementID
	})
	return nil
}

func (s *Service) reviewAvailability(ctx context.Context, p Principal, target ReviewTarget) ([]ReviewIssue, error) {
	issues := []ReviewIssue{}
	access := Access{ProjectID: target.TargetLocator.ProjectID, RevisionID: target.TargetLocator.ContentRevisionID}
	if err := s.reviewVerify(ctx, p, target.SourceManifest.ContentRef, access, target.SourceManifest.SHA256, target.SourceManifest.SizeBytes); err != nil {
		if !errors.Is(err, ErrForbidden) {
			return nil, err
		}
		issues = append(issues, ReviewIssue{Code: "RESTRICTED"})
	}
	for _, e := range target.Evidence {
		v, err := s.content.Metadata(ctx, p, e.ContentRef, access)
		if errors.Is(err, ErrForbidden) {
			issues = append(issues, ReviewIssue{Code: "RESTRICTED", EvidenceID: e.EvidenceID})
			continue
		}
		if err != nil {
			return nil, err
		}
		if v.ContentRef != e.ContentRef || v.OrganizationID != p.OrganizationID || v.SHA256 != e.SHA256 || v.Size != e.SizeBytes {
			return nil, ErrTargetMismatch
		}
		if err = s.reviewVerify(ctx, p, e.ContentRef, access, e.SHA256, e.SizeBytes); err != nil {
			if errors.Is(err, ErrForbidden) {
				issues = append(issues, ReviewIssue{Code: "RESTRICTED", EvidenceID: e.EvidenceID})
				continue
			}
			return nil, err
		}
	}
	dependencyIssues, err := s.reviewDependencyAvailability(ctx, p, target)
	if err != nil {
		return nil, err
	}
	return append(issues, dependencyIssues...), nil
}

func (s *Service) reviewDependencyAvailability(ctx context.Context, p Principal, target ReviewTarget) ([]ReviewIssue, error) {
	issues := []ReviewIssue{}
	for _, d := range target.Dependencies {
		_, err := s.assets.References(ctx, p, AssetRef{AssetID: d.AssetID, VersionID: d.VersionID, Purpose: d.Purpose})
		if errors.Is(err, ErrForbidden) || errors.Is(err, ErrNotFound) {
			issues = append(issues, ReviewIssue{Code: "DEPENDENCY_RESTRICTED"})
			continue
		}
		if err != nil {
			return nil, err
		}
	}
	return issues, nil
}

type reviewByteCounter struct{ n int64 }

func (w *reviewByteCounter) Write(data []byte) (int, error) {
	w.n += int64(len(data))
	return len(data), nil
}

// Download streams the exact immutable bytes using the original user's access.
// Compare against the target's first metadata snapshot, not metadata fetched by
// the transport while opening the stream.
func (s *Service) reviewVerify(ctx context.Context, p Principal, ref ContentRef, access Access, expected string, size int64) error {
	if !ref.Valid() || !reviewHashPattern.MatchString(expected) || size < 0 {
		return ErrNotReady
	}
	hash := sha256.New()
	count := &reviewByteCounter{}
	if downloader, ok := s.content.(interface {
		Download(context.Context, Principal, ContentRef, Access, io.Writer, int64) error
	}); ok {
		if err := downloader.Download(ctx, p, ref, access, io.MultiWriter(hash, count), size); err != nil {
			return err
		}
	} else {
		if size > MaxManifestBytes {
			return ErrNotReady
		}
		raw, err := s.content.Read(ctx, p, ref, access, MaxManifestBytes)
		if err != nil {
			return err
		}
		_, _ = io.MultiWriter(hash, count).Write(raw)
	}
	if count.n != size || hex.EncodeToString(hash.Sum(nil)) != expected {
		return ErrTargetMismatch
	}
	return nil
}

func validateReviewFileRefs(value any, files map[string]File) error {
	switch v := value.(type) {
	case map[string]any:
		if raw, exists := v["content_ref"]; exists {
			id := str(v["file_id"])
			var ref ContentRef
			data, err := json.Marshal(raw)
			if err != nil || json.Unmarshal(data, &ref) != nil || !ref.Valid() || files[id].Content != ref {
				return ErrTargetMismatch
			}
		}
		for _, child := range v {
			if err := validateReviewFileRefs(child, files); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := validateReviewFileRefs(child, files); err != nil {
				return err
			}
		}
	}
	return nil
}
func cloneReviewTarget(target ReviewTarget) ReviewTarget {
	raw, _ := json.Marshal(target)
	var copy ReviewTarget
	_ = json.NewDecoder(bytes.NewReader(raw)).Decode(&copy)
	return copy
}

func reviewInputGlobals(drama, material contentObject) ([]contentObject, error) {
	owners, err := indexObjects(drama["globals"], "globals")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{str(material["key"]): true}
	queue := append([]any{}, listOf(material["input_global_keys"])...)
	out := []contentObject{}
	for len(queue) > 0 {
		key := str(queue[0])
		queue = queue[1:]
		if seen[key] {
			continue
		}
		seen[key] = true
		owner := owners[key]
		if owner == nil {
			return nil, ErrNotFound
		}
		out = append(out, objectOf(reviewProductionValue(owner)))
		queue = append(queue, listOf(owner["input_global_keys"])...)
	}
	sort.Slice(out, func(i, j int) bool { return str(out[i]["key"]) < str(out[j]["key"]) })
	return out, nil
}
func reviewProductionValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := contentObject{}
		for key, item := range v {
			if key != "review" && key != "definition_review" && key != "name" && key != "label" {
				out[key] = reviewProductionValue(item)
			}
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, reviewProductionValue(item))
		}
		return out
	}
	return value
}
func collectReviewFiles(value any, roles map[string]string) {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if key == "review" || key == "definition_review" {
				continue
			}
			if key == "file_id" {
				id := str(item)
				if id != "" {
					if _, ok := roles[id]; !ok {
						roles[id] = reviewEvidenceRole(str(v["purpose"]))
					}
				}
				continue
			}
			if strings.HasSuffix(key, "_file_id") {
				id := str(item)
				if id != "" {
					if _, ok := roles[id]; !ok {
						roles[id] = "supporting"
					}
				}
				continue
			}
			if key == "file_ids" || strings.HasSuffix(key, "_file_ids") {
				for _, value := range listOf(item) {
					id := str(value)
					if id != "" {
						if _, ok := roles[id]; !ok {
							roles[id] = "supporting"
						}
					}
				}
				continue
			}
			collectReviewFiles(item, roles)
		}
	case []any:
		for _, item := range v {
			collectReviewFiles(item, roles)
		}
	}
}
