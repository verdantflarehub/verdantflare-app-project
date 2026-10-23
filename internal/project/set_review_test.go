package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func reviewTestScope(kind string) ReviewScope {
	return ReviewScope{Kind: kind, EvidenceIDs: []string{id(11)}, CameraIDs: []string{}, RegionIDs: []string{}, Claims: []string{"image_reading"}}
}

func reviewTestRequest() ReviewAppendRequest {
	return ReviewAppendRequest{TargetLocator: ReviewLocator{Kind: "project", ProjectID: id(1), ContentRevisionID: id(2), MaterialKey: "hall", StateKey: "night", VersionKey: "v1"}, ExpectedTargetHash: strings.Repeat("a", 64), CommitID: id(3), ExpectedRevisionID: id(2), Action: "comment", Scope: reviewTestScope("evidence"), Body: "Inspect the fixed view"}
}

func TestReviewOriginalRequestFingerprint(t *testing.T) {
	base := reviewTestRequest()
	seen := map[string]bool{}
	for _, kind := range []string{"omitted", "null supersedes", "empty technical refs", "both"} {
		r := base
		if kind == "null supersedes" || kind == "both" {
			r.SupersedesRecordID = json.RawMessage("null")
		}
		if kind == "empty technical refs" || kind == "both" {
			refs := []string{}
			r.TechnicalRefs = &refs
		}
		raw, hash, err := reviewJSON(r)
		if err != nil || seen[hash] {
			t.Fatalf("original optional field identity collapsed for %s: %v", kind, err)
		}
		seen[hash] = true
		var decoded ReviewAppendRequest
		if err := decodeReview(raw, &decoded); err != nil {
			t.Fatalf("valid request rejected for %s: %v", kind, err)
		}
		_, again, err := reviewJSON(decoded)
		if err != nil || hash != again {
			t.Fatalf("request fingerprint changed after durable roundtrip: %s", kind)
		}
	}
	raw, _, _ := reviewJSON(base)
	for _, suffix := range []string{`,"actor":{}}`, `,"body":"changed"}`, `,"technical_refs":null}`, `,"supersedes_record_id":42}`} {
		var decoded ReviewAppendRequest
		err := decodeReview([]byte(strings.TrimSuffix(string(raw), "}")+suffix), &decoded)
		if suffix == `,"supersedes_record_id":42}` && err == nil {
			_, err = decoded.supersedes()
		}
		if err == nil {
			t.Fatalf("accepted injected/invalid field: %s", suffix)
		}
	}
	var decoded ReviewAppendRequest
	if decodeReview([]byte(strings.Replace(string(raw), `"expected_record_id":null,`, "", 1)), &decoded) == nil {
		t.Fatal("accepted omitted required first-record CAS")
	}
}

func TestReviewCanonicalJSONAndLegacyLocator(t *testing.T) {
	a, ah, err := reviewJSON(map[string]any{"b": 1.0, "a": "é"})
	b, bh, other := reviewJSON(map[string]any{"a": "é", "b": 1})
	if err != nil || other != nil || string(a) != string(b) || ah != bh || string(a) != `{"a":"é","b":1}` {
		t.Fatal("JCS did not normalize object keys/numbers")
	}
	if _, _, err = reviewJSON(map[string]any{"body": "e\u0301"}); err == nil {
		t.Fatal("accepted non-NFC source")
	}
	if _, _, err = reviewJSON(map[string]any{"frame": int64(9007199254740992)}); err == nil {
		t.Fatal("accepted unsafe integer")
	}
	l := reviewTestRequest().TargetLocator
	l.StateKey, l.VersionKey = "", ""
	if !l.valid() {
		t.Fatal("legacy empty pair should be inspectable")
	}
	l.VersionKey = "v1"
	if l.valid() {
		t.Fatal("accepted partial state/version locator")
	}
}

func TestReviewRejectsRawNumericPrecisionLoss(t *testing.T) {
	r := reviewTestRequest()
	r.Scope.ImageRect = &ReviewImageRect{X: .1, Y: 0, W: .5, H: 1}
	raw, _, err := reviewJSON(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []string{"0.10000000000000000001", "1e-999999999", "9007199254740991.1"} {
		input := strings.Replace(string(raw), `"x":0.1`, `"x":`+x, 1)
		var decoded ReviewAppendRequest
		if decodeReview([]byte(input), &decoded) == nil {
			t.Fatal("accepted lossy original number", x)
		}
	}
	for _, x := range []string{"0.10", "1e-1", "0e-999999999"} {
		input := strings.Replace(string(raw), `"x":0.1`, `"x":`+x, 1)
		var decoded ReviewAppendRequest
		if err := decodeReview([]byte(input), &decoded); err != nil {
			t.Fatal("exact decimal value rejected", x, err)
		}
	}
}

func TestReviewProjectionAndExplicitSupersession(t *testing.T) {
	local := reviewTestScope("evidence")
	full := reviewTestScope("full_target")
	full.Claims = []string{"space_layout", "source_recovery", "full_set"}
	optional := reviewTestScope("camera")
	optional.CameraIDs, optional.Claims = []string{"C1"}, []string{"new_camera"}
	record := func(n int, origin, action string, scope ReviewScope) ReviewRecord {
		return ReviewRecord{RecordID: id(n), Actor: ReviewActor{Origin: origin}, Action: action, Scope: scope}
	}
	agent := record(20, "agent", "comment", local)
	approved := record(21, "human_interactive", "approve", full)
	rejected := record(22, "human_interactive", "request_changes", local)
	resolved := record(23, "human_interactive", "approve", local)
	cases := []struct {
		name, want string
		records    []ReviewRecord
	}{
		{"agent only", "unreviewed", []ReviewRecord{agent}},
		{"local approval", "in_review", []ReviewRecord{resolved}},
		{"full approval", "approved", []ReviewRecord{approved}},
		{"comment preserves approval", "approved", []ReviewRecord{approved, record(24, "human_interactive", "comment", local)}},
		{"local rejection blocks", "rejected", []ReviewRecord{approved, rejected}},
		{"resolution needs new full approval", "in_review", []ReviewRecord{approved, rejected, resolved}},
		{"fresh full approval", "approved", []ReviewRecord{rejected, resolved, approved}},
		{"optional rejection", "approved", []ReviewRecord{approved, record(25, "human_interactive", "request_changes", optional)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := reviewProjection(tc.records); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
	r := reviewTestRequest()
	r.Action = "approve"
	if err := validateReviewDecision([]ReviewRecord{rejected}, r); !errors.Is(err, ErrReviewConflict) {
		t.Fatal("decision replacement omitted required supersedes", err)
	}
	r.SupersedesRecordID, _ = json.Marshal(rejected.RecordID)
	if err := validateReviewDecision([]ReviewRecord{rejected}, r); err != nil {
		t.Fatal("exact current scope supersession rejected", err)
	}
	r.Scope, r.SupersedesRecordID = full, nil
	if err := validateReviewDecision([]ReviewRecord{rejected}, r); !errors.Is(err, ErrReviewScope) {
		t.Fatal("full approval swallowed unresolved local rejection", err)
	}
}

func TestReviewScopeChecksFixedEvidence(t *testing.T) {
	target := ReviewTarget{Evidence: []ReviewEvidence{{EvidenceID: id(11), MIME: "image/png"}, {EvidenceID: id(12), MIME: "application/octet-stream"}}, Coverage: ReviewCoverage{CameraIDs: []string{"C1"}, RegionIDs: []string{"R1"}}, Requirements: []ReviewRequirement{{Claim: "source_recovery", State: "unverified"}}}
	scope := reviewTestScope("evidence")
	if err := validateReviewScope(target, scope, "approve"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"duplicate", "dangling", "non-image", "unverified", "rectangle", "full"} {
		x := reviewTestScope("evidence")
		switch kind {
		case "duplicate":
			x.EvidenceIDs = append(x.EvidenceIDs, id(11))
		case "dangling":
			x.EvidenceIDs = []string{id(99)}
		case "non-image":
			x.EvidenceIDs = []string{id(12)}
		case "unverified":
			x.Claims = []string{"source_recovery"}
		case "rectangle":
			x.ImageRect = &ReviewImageRect{X: .8, Y: 0, W: .5, H: 1}
		case "full":
			x.Kind = "full_target"
		}
		if err := validateReviewScope(target, x, "approve"); err == nil {
			t.Fatalf("accepted %s scope", kind)
		}
	}
}

func TestReviewScopeCannotElevateOrMislabelEvidence(t *testing.T) {
	target := ReviewTarget{Evidence: []ReviewEvidence{{EvidenceID: id(11), MIME: "image/png", CameraIDs: []string{"C1"}, RegionIDs: []string{"R1"}}}, Coverage: ReviewCoverage{CameraIDs: []string{"C1", "C2"}, RegionIDs: []string{"R1", "R2"}}}
	for _, kind := range []string{"camera", "region"} {
		scope := reviewTestScope(kind)
		if kind == "camera" {
			scope.CameraIDs = []string{"C1"}
		} else {
			scope.RegionIDs = []string{"R1"}
		}
		if err := validateReviewScope(target, scope, "comment"); err != nil {
			t.Fatal("valid mapping rejected", err)
		}
		if kind == "camera" {
			scope.CameraIDs = []string{"C2"}
		} else {
			scope.RegionIDs = []string{"R2"}
		}
		if err := validateReviewScope(target, scope, "comment"); !errors.Is(err, ErrInvalid) {
			t.Fatal("unrelated evidence accepted for selected scope", kind, err)
		}
	}
	scope := reviewTestScope("evidence")
	scope.Claims = []string{"full_set"}
	if _, err := normalizeReviewScope(scope); !errors.Is(err, ErrInvalid) {
		t.Fatal("local scope accepted full_set claim")
	}
}

func TestReviewIDsUseSchemaCharacterLimit(t *testing.T) {
	for _, value := range []string{strings.Repeat("机", 128), strings.Repeat("🎥", 128)} {
		if _, err := reviewIDs([]string{value}); err != nil {
			t.Fatal("128 Unicode characters rejected", err)
		}
	}
	for _, value := range []string{strings.Repeat("机", 129), strings.Repeat("🎥", 129), string([]byte{0xff})} {
		if _, err := reviewIDs([]string{value}); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid or overlong Unicode ID accepted", err)
		}
	}
}

func TestReviewReservedFilesSurviveCategoryChanges(t *testing.T) {
	file := File{ID: id(11), Path: "reviews/set/" + id(12) + ".json", Role: "record", Content: ContentRef{id(1), id(2), id(3)}}
	old := Manifest{Category: "ai_short_drama", Files: []File{file}}
	for _, kind := range []string{"category", "delete", "replace", "rename", "new outside drama", "new generic"} {
		plan := preparePlan{Manifest: Manifest{Category: old.Category, Files: []File{file}}}
		base := old
		switch kind {
		case "category":
			plan.Manifest.Category = "image"
		case "delete":
			plan.Manifest.Files = nil
		case "replace":
			plan.Manifest.Files[0].Content.VersionID = id(9)
		case "rename":
			plan.Manifest.Files[0].Path = "hidden.json"
		case "new outside drama":
			base.Files = nil
			base.Category = "image"
			plan.Manifest.Category = "image"
			plan.Review = &reviewPlan{FileID: file.ID, Record: ReviewRecord{RecordID: id(12)}}
		case "new generic":
			base.Files = nil
		}
		if err := guardReviewFiles(base, plan); !errors.Is(err, ErrForbidden) {
			t.Fatalf("reserved namespace bypass: %s: %v", kind, err)
		}
	}
	if err := guardReviewFiles(old, preparePlan{Manifest: old}); err != nil {
		t.Fatal("unchanged records rejected", err)
	}
}

func reviewCameraFixture(t *testing.T) ([]byte, ReviewTarget, ReviewCamera) {
	t.Helper()
	basis := strings.Repeat("b", 64)
	raw := []byte(`{"schema_version":1,"kind":"set-review-camera","space_basis_sha256":"` + basis + `","coordinate_system":"blender_z_up_m","camera":{"projection":"PERSP","location_blender_m":[1,2,3],"rotation_euler_xyz_rad":[0,0,0],"lens_mm":50,"sensor_width_mm":36,"sensor_height_mm":24,"sensor_fit":"AUTO","clip_start_m":0.1,"clip_end_m":100,"ortho_scale":6,"shift_x":0,"shift_y":0},"render":{"resolution_x":1920,"resolution_y":1080,"pixel_aspect_x":1,"pixel_aspect_y":1,"frame":1}}`)
	sum := sha256.Sum256(raw)
	params := hex.EncodeToString(sum[:])
	camera := ReviewCamera{CameraID: "C1", ParametersEvidenceID: id(12), ParametersSHA256: params, EvidenceIDs: []string{id(13)}, RegionIDs: []string{}}
	target := ReviewTarget{Coverage: ReviewCoverage{SpaceBasis: &ReviewSpaceBasis{EvidenceID: id(11), SHA256: basis}}, Evidence: []ReviewEvidence{{EvidenceID: id(11), SHA256: basis}, {EvidenceID: id(12), SHA256: params, MIME: "application/json"}, {EvidenceID: id(13), MIME: "image/png", CameraIDs: []string{"C1"}}}}
	return raw, target, camera
}

func TestReviewCameraVerifiesExactStaticMapping(t *testing.T) {
	for _, kind := range []string{"valid", "basis", "parameter bytes", "parameter hash", "video", "frame range", "unknown evidence", "unmapped camera", "unknown field", "invalid dimensions"} {
		t.Run(kind, func(t *testing.T) {
			raw, target, camera := reviewCameraFixture(t)
			switch kind {
			case "basis":
				target.Coverage.SpaceBasis.SHA256 = strings.Repeat("c", 64)
			case "parameter bytes":
				raw = append(raw, '\n')
			case "parameter hash":
				camera.ParametersSHA256 = strings.Repeat("c", 64)
			case "video":
				target.Evidence[2].MIME = "video/mp4"
			case "frame range":
				target.Evidence[2].FrameRange = &ReviewFrameRange{Start: 1, End: 1, FPSNumerator: 24, FPSDenominator: 1}
			case "unknown evidence":
				camera.EvidenceIDs = []string{id(99)}
			case "unmapped camera":
				target.Evidence[2].CameraIDs = []string{"C2"}
			case "unknown field":
				raw = []byte(strings.TrimSuffix(string(raw), "}") + `,"pass":true}`)
			case "invalid dimensions":
				raw = []byte(strings.Replace(string(raw), `"resolution_x":1920`, `"resolution_x":0`, 1))
			}
			if verifyReviewCamera(raw, target, camera) != (kind == "valid") {
				t.Fatal("incorrect camera verification")
			}
		})
	}
}

func TestReviewRestrictedProjectionMasksEmbeddedTargetAndBody(t *testing.T) {
	target := ReviewTarget{Definition: contentObject{"secret": "private"}}
	out := ReviewOpenResult{TargetView: &target, TargetManifest: &target, Availability: []ReviewIssue{{Code: "RESTRICTED", EvidenceID: "private-evidence"}}, ScopeDecisions: []ReviewDecision{{RecordID: id(1)}}, NextCursor: "private", Capabilities: ReviewCapabilities{CanComment: true, CanDecide: true}}
	restrictReviewResult(&out, []ReviewRecord{{RecordID: id(2), Body: "private body", TargetManifest: target, Actor: ReviewActor{SubjectID: "private actor"}}})
	raw, err := json.Marshal(out)
	if err != nil || strings.Contains(string(raw), "private") || out.TargetView != nil || out.TargetManifest != nil || out.Capabilities.CanComment || len(out.Records) != 1 {
		t.Fatal("restricted response leaked review data", string(raw), err)
	}
	if !strings.Contains(string(raw), `"record_view":null`) {
		t.Fatal("restricted union lost explicit empty view")
	}
}

func TestReviewProductionAndPreparedDocumentRemainIndependent(t *testing.T) {
	f := newPublicationFixture(t)
	document := publicationDocument{Opened: f.source, File: f.source.Manifest.Files[0], Root: f.root}
	r := reviewTestRequest()
	before := publicationJSON(t, document.Root)
	material, state, version, err := reviewMaterial(document, r.TargetLocator)
	if err != nil {
		t.Fatal(err)
	}
	_, oldHash, _ := reviewJSON(reviewProductionDefinition(material, state, version))
	plan, err := prepareReview(document, r, ReviewRecord{RecordID: id(90)}, "{}", strings.Repeat("a", 64), oldHash, "in_review", true)
	if err != nil || len(plan.Writes) != 2 {
		t.Fatal("prepare review", err)
	}
	if string(before) != string(publicationJSON(t, document.Root)) {
		t.Fatal("preparation mutated original fixed document")
	}
	version["review"] = contentObject{"state": "approved", "record_file_id": id(99)}
	state["name"] = "Renamed"
	_, unchanged, _ := reviewJSON(reviewProductionDefinition(material, state, version))
	if oldHash != unchanged {
		t.Fatal("derived review/label affected production hash")
	}
	version["change_note"] = "Actual production changed"
	_, changed, _ := reviewJSON(reviewProductionDefinition(material, state, version))
	if changed == oldHash {
		t.Fatal("production change did not change signature")
	}
	if !protectedReviewChange(contentObject{"state": "approved", "record_file_id": id(99)}, contentObject{"state": "unreviewed"}) {
		t.Fatal("generic clearing of managed review accepted")
	}
}

type reviewReadArtifact struct {
	Artifact
	data []byte
	err  error
}

func (a reviewReadArtifact) Read(context.Context, Principal, ContentRef, Access, int64) ([]byte, error) {
	return a.data, a.err
}

type reviewStreamArtifact struct {
	reviewReadArtifact
	stream []byte
}

func (a reviewStreamArtifact) Download(_ context.Context, _ Principal, _ ContentRef, _ Access, dst io.Writer, _ int64) error {
	_, err := dst.Write(a.stream)
	return err
}

func TestReviewByteVerificationDoesNotTrustMetadata(t *testing.T) {
	raw := []byte("immutable content")
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	ref := ContentRef{id(1), id(2), id(3)}
	for _, tc := range []struct {
		name     string
		artifact Artifact
		size     int64
		want     error
	}{
		{"small read", reviewReadArtifact{data: raw}, int64(len(raw)), nil},
		{"corrupt bytes", reviewReadArtifact{data: []byte("changed")}, int64(len(raw)), ErrTargetMismatch},
		{"wrong size", reviewReadArtifact{data: raw}, int64(len(raw) + 1), ErrTargetMismatch},
		{"large without stream", reviewReadArtifact{data: raw}, MaxManifestBytes + 1, ErrNotReady},
		{"stream", reviewStreamArtifact{stream: raw}, int64(len(raw)), nil},
		{"corrupt stream", reviewStreamArtifact{stream: []byte("changed")}, int64(len(raw)), ErrTargetMismatch},
		{"denied", reviewReadArtifact{err: ErrForbidden}, int64(len(raw)), ErrForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Service{content: tc.artifact}
			if err := s.reviewVerify(context.Background(), Principal{}, ref, Access{}, hash, tc.size); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestReviewEvidenceDefaultsAndNoSelfCertifiedRecovery(t *testing.T) {
	var mapping reviewEvidenceDocument
	if err := decodeReviewEvidence([]byte(`{"schema_version":1,"kind":"set-review-evidence"}`), &mapping); err != nil {
		t.Fatal("omitted collections", err)
	}
	for _, raw := range []string{`{"schema_version":1,"kind":"set-review-evidence","evidence":null}`, `{"schema_version":1,"kind":"set-review-evidence","coverage":{"camera_ids":["C1"],"camera_ids":[]}}`, `{"schema_version":1,"kind":"set-review-evidence","checks":[{"claim":"source_recovery","evidence_file_ids":[],"pass":true}]}`} {
		if decodeReviewEvidence([]byte(raw), &mapping) == nil {
			t.Fatal("accepted invalid mapping", raw)
		}
	}
	target := ReviewTarget{Evidence: []ReviewEvidence{{EvidenceID: id(11), FileID: id(11), MIME: "application/octet-stream", Role: "source_engineering"}}, Coverage: ReviewCoverage{CameraIDs: []string{}, RegionIDs: []string{}, Cameras: []ReviewCamera{}}}
	if err := applyReviewEvidence(&target, nil); err != nil {
		t.Fatal(err)
	}
	for _, requirement := range target.Requirements {
		if (requirement.Claim == "source_recovery" || requirement.Claim == "full_set") && requirement.State == "satisfied" {
			t.Fatal("file presence certified recovery")
		}
	}
}

func TestReviewHTTPOriginAndMCPRouteBoundary(t *testing.T) {
	token := strings.Repeat("s", 32)
	s := &Service{}
	handler, err := s.HTTPHandler(token)
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"", "agent", "human_interactive", "forged", "duplicate"} {
		r := httptest.NewRequest("POST", "/internal/v1/project/review/open", strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Organization-Id", id(1))
		r.Header.Set("X-User-Id", id(2))
		r.Header.Set("X-Request-Id", id(3))
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("X-Review-Origin", origin)
		}
		if origin == "duplicate" {
			r.Header.Add("X-Review-Origin", "human_interactive")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		want := 400
		if origin == "forged" || origin == "duplicate" {
			want = 403
		}
		if w.Code != want {
			t.Fatalf("origin %q: got %d, want %d", origin, w.Code, want)
		}
	}
	mcp, err := s.MCPHandler(token)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"project.review_open","arguments":{}}}`))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Organization-Id", id(1))
	r.Header.Set("X-User-Id", id(2))
	r.Header.Set("X-Request-Id", id(3))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Review-Origin", "forged")
	w := httptest.NewRecorder()
	mcp.ServeHTTP(w, r)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "INVALID_ARGUMENT") {
		t.Fatalf("MCP did not override origin and use exact route: %d %s", w.Code, w.Body.String())
	}
}
