package project

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"github.com/verdantflarehub/verdantflare-app-project/internal/strictjson"
	"golang.org/x/text/unicode/norm"
)

var (
	ErrReviewConflict = errors.New("REVIEW_CONFLICT")
	ErrTargetMismatch = errors.New("TARGET_MISMATCH")
	ErrReviewScope    = errors.New("REVIEW_SCOPE_INCOMPLETE")
	ErrLegacyTarget   = errors.New("LEGACY_TARGET_UNMAPPED")
	reviewHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	reviewKeyPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
)

type ReviewLocator struct {
	Kind              string `json:"kind"`
	ProjectID         string `json:"project_id"`
	ContentRevisionID string `json:"content_revision_id"`
	MaterialKey       string `json:"material_key"`
	StateKey          string `json:"state_key"`
	VersionKey        string `json:"version_key"`
}

func (l ReviewLocator) valid() bool {
	return l.Kind == "project" && ValidID(l.ProjectID) && ValidID(l.ContentRevisionID) && reviewKeyPattern.MatchString(l.MaterialKey) && (l.StateKey == "") == (l.VersionKey == "") && (l.StateKey == "" || reviewKeyPattern.MatchString(l.StateKey)) && (l.VersionKey == "" || reviewKeyPattern.MatchString(l.VersionKey))
}

type ReviewSource struct {
	ContentRef ContentRef `json:"content_ref"`
	SHA256     string     `json:"sha256"`
	SizeBytes  int64      `json:"size_bytes"`
}
type ReviewFrameRange struct {
	Start          int64 `json:"start"`
	End            int64 `json:"end"`
	FPSNumerator   int64 `json:"fps_numerator"`
	FPSDenominator int64 `json:"fps_denominator"`
}
type ReviewEvidence struct {
	EvidenceID string            `json:"evidence_id"`
	FileID     string            `json:"file_id"`
	ContentRef ContentRef        `json:"content_ref"`
	SHA256     string            `json:"sha256"`
	SizeBytes  int64             `json:"size_bytes"`
	MIME       string            `json:"mime"`
	Role       string            `json:"role"`
	CameraIDs  []string          `json:"camera_ids"`
	RegionIDs  []string          `json:"region_ids"`
	FrameRange *ReviewFrameRange `json:"frame_range"`
}
type ReviewDependency struct {
	AssetID        string      `json:"asset_id"`
	VersionID      string      `json:"asset_version_id"`
	Purpose        string      `json:"purpose"`
	ManifestRef    *ContentRef `json:"manifest_ref"`
	ManifestSHA256 *string     `json:"manifest_sha256"`
}
type ReviewCoverage struct {
	CameraIDs  []string          `json:"camera_ids"`
	RegionIDs  []string          `json:"region_ids"`
	SpaceBasis *ReviewSpaceBasis `json:"space_basis"`
	Cameras    []ReviewCamera    `json:"cameras"`
}
type ReviewSpaceBasis struct {
	EvidenceID string `json:"evidence_id"`
	SHA256     string `json:"sha256"`
}
type ReviewCamera struct {
	CameraID             string   `json:"camera_id"`
	ParametersEvidenceID string   `json:"parameters_evidence_id"`
	ParametersSHA256     string   `json:"parameters_sha256"`
	EvidenceIDs          []string `json:"evidence_ids"`
	RegionIDs            []string `json:"region_ids"`
	Verified             bool     `json:"verified"`
}
type ReviewRequirement struct {
	RequirementID string   `json:"requirement_id"`
	Claim         string   `json:"claim"`
	EvidenceIDs   []string `json:"evidence_ids"`
	State         string   `json:"state"`
	ReasonCode    string   `json:"reason_code"`
}
type ReviewTarget struct {
	SchemaVersion  int                 `json:"schema_version"`
	TargetLocator  ReviewLocator       `json:"target_locator"`
	SourceManifest ReviewSource        `json:"source_manifest"`
	Definition     contentObject       `json:"definition"`
	Evidence       []ReviewEvidence    `json:"evidence"`
	Dependencies   []ReviewDependency  `json:"dependencies"`
	Coverage       ReviewCoverage      `json:"coverage"`
	Requirements   []ReviewRequirement `json:"requirements"`
}
type ReviewIssue struct {
	Code       string `json:"code"`
	EvidenceID string `json:"evidence_id,omitempty"`
}
type ReviewCapabilities struct {
	CanComment  bool     `json:"can_comment"`
	CanDecide   bool     `json:"can_decide"`
	ReasonCodes []string `json:"reason_codes"`
}
type ReviewScope struct {
	Kind        string            `json:"kind"`
	EvidenceIDs []string          `json:"evidence_ids"`
	CameraIDs   []string          `json:"camera_ids"`
	RegionIDs   []string          `json:"region_ids"`
	Claims      []string          `json:"claims"`
	ImageRect   *ReviewImageRect  `json:"image_rect,omitempty"`
	FrameRange  *ReviewFrameRange `json:"frame_range,omitempty"`
}
type ReviewImageRect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}
type ReviewActor struct {
	SubjectID      string `json:"subject_id"`
	OrganizationID string `json:"organization_id"`
	Origin         string `json:"origin"`
}
type ReviewRecord struct {
	SchemaVersion      int          `json:"schema_version"`
	RecordID           string       `json:"record_id"`
	TargetHash         string       `json:"target_hash"`
	TargetManifest     ReviewTarget `json:"target_manifest"`
	PreviousRecordID   *string      `json:"previous_record_id"`
	Action             string       `json:"action"`
	Scope              ReviewScope  `json:"scope"`
	Body               string       `json:"body"`
	Actor              ReviewActor  `json:"actor"`
	CreatedAt          time.Time    `json:"created_at"`
	SupersedesRecordID *string      `json:"supersedes_record_id"`
	TechnicalRefs      []string     `json:"technical_refs"`
}
type ReviewDecision struct {
	ScopeHash string      `json:"scope_hash"`
	Scope     ReviewScope `json:"scope"`
	Action    string      `json:"action"`
	RecordID  string      `json:"record_id"`
}
type ReviewOpenRequest struct {
	TargetLocator      ReviewLocator `json:"target_locator"`
	ExpectedTargetHash string        `json:"expected_target_hash,omitempty"`
	RecordsRevisionID  string        `json:"records_revision_id,omitempty"`
	Cursor             string        `json:"cursor,omitempty"`
}
type ReviewOpenResult struct {
	TargetLocator       ReviewLocator      `json:"target_locator"`
	TargetHash          *string            `json:"target_hash"`
	TargetManifest      *ReviewTarget      `json:"target_manifest,omitempty"`
	TargetView          *ReviewTarget      `json:"target_view"`
	Availability        []ReviewIssue      `json:"availability"`
	MappingState        string             `json:"mapping_state"`
	RecordsHeadRecordID *string            `json:"records_head_record_id"`
	ReviewState         string             `json:"review_state"`
	ScopeDecisions      []ReviewDecision   `json:"scope_decisions"`
	Records             []any              `json:"records"`
	NextCursor          string             `json:"next_cursor"`
	Capabilities        ReviewCapabilities `json:"capabilities"`
	Missing             []ReviewIssue      `json:"missing"`
	HeadRevisionID      string             `json:"head_revision_id"`
	RecordsRevisionID   string             `json:"records_revision_id"`
}
type ReviewAppendRequest struct {
	TargetLocator      ReviewLocator   `json:"target_locator"`
	ExpectedTargetHash string          `json:"expected_target_hash"`
	CommitID           string          `json:"commit_id"`
	ExpectedRecordID   *string         `json:"expected_record_id"`
	ExpectedRevisionID string          `json:"expected_revision_id"`
	Action             string          `json:"action"`
	Scope              ReviewScope     `json:"scope"`
	Body               string          `json:"body"`
	SupersedesRecordID json.RawMessage `json:"supersedes_record_id,omitempty"`
	TechnicalRefs      *[]string       `json:"technical_refs,omitempty"`
}
type ReviewAppendResult struct {
	State               string     `json:"state"`
	CommitID            string     `json:"commit_id"`
	RecordID            string     `json:"record_id"`
	TargetHash          string     `json:"target_hash"`
	ReviewState         string     `json:"review_state"`
	RecordsHeadRecordID string     `json:"records_head_record_id"`
	RevisionID          string     `json:"revision_id"`
	ManifestRef         ContentRef `json:"manifest_ref"`
	ProjectionApplied   bool       `json:"projection_applied"`
}
type ReviewStatusResult struct {
	State         string              `json:"state"`
	RequestSHA256 string              `json:"request_sha256"`
	Result        *ReviewAppendResult `json:"result,omitempty"`
	ErrorCode     string              `json:"error_code,omitempty"`
}
type reviewPlan struct {
	Request           ReviewAppendRequest `json:"request"`
	Record            ReviewRecord        `json:"record"`
	FileID            string              `json:"file_id"`
	RecordSHA256      string              `json:"record_sha256"`
	ProductionHash    string              `json:"production_hash"`
	ReviewState       string              `json:"review_state"`
	ProjectionApplied bool                `json:"projection_applied"`
}

func reviewJSON(v any) ([]byte, string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, "", ErrInvalid
	}
	b, err = reviewCanonicalBytes(b)
	if err != nil {
		return nil, "", err
	}
	h := sha256.Sum256(b)
	return b, hex.EncodeToString(h[:]), nil
}

func reviewCanonicalBytes(b []byte) ([]byte, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil || !reviewCanonicalValue(value) {
		return nil, ErrInvalid
	}
	canonical, err := jsoncanonicalizer.Transform(b)
	if err != nil {
		return nil, ErrInvalid
	}
	var normalized any
	decoder = json.NewDecoder(bytes.NewReader(canonical))
	decoder.UseNumber()
	if decoder.Decode(&normalized) != nil || !reviewSameNumbers(value, normalized) {
		return nil, ErrInvalid
	}
	return canonical, nil
}

func reviewSameNumbers(a, b any) bool {
	switch x := a.(type) {
	case json.Number:
		y, ok := b.(json.Number)
		if !ok {
			return false
		}
		left, le, lok := reviewDecimal(string(x))
		right, re, rok := reviewDecimal(string(y))
		return lok && rok && left == right && le == re
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for key, value := range x {
			if !reviewSameNumbers(value, y[key]) {
				return false
			}
		}
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i, value := range x {
			if !reviewSameNumbers(value, y[i]) {
				return false
			}
		}
	}
	return true
}

// Compare decimal values without constructing powers of ten from untrusted
// exponents. This also rejects JCS underflow and rounded high precision input.
func reviewDecimal(number string) (string, int64, bool) {
	mantissa, exponent := number, "0"
	if i := strings.IndexAny(number, "eE"); i >= 0 {
		mantissa, exponent = number[:i], number[i+1:]
	}
	negative := strings.HasPrefix(mantissa, "-")
	mantissa = strings.TrimPrefix(mantissa, "-")
	fraction := 0
	if i := strings.IndexByte(mantissa, '.'); i >= 0 {
		fraction = len(mantissa) - i - 1
		mantissa = mantissa[:i] + mantissa[i+1:]
	}
	mantissa = strings.TrimLeft(mantissa, "0")
	if mantissa == "" {
		return "0", 0, true
	}
	exp, err := strconv.ParseInt(exponent, 10, 64)
	if err != nil || exp > 1<<50 || exp < -(1<<50) {
		return "", 0, false
	}
	trimmed := strings.TrimRight(mantissa, "0")
	exp += int64(len(mantissa) - len(trimmed) - fraction)
	if negative {
		trimmed = "-" + trimmed
	}
	return trimmed, exp, true
}
func reviewCanonicalValue(v any) bool {
	switch x := v.(type) {
	case string:
		return norm.NFC.IsNormalString(x)
	case float64:
		return !math.IsNaN(x) && !math.IsInf(x, 0) && math.Abs(x) <= 9007199254740991
	case json.Number:
		n, err := x.Float64()
		return err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) && math.Abs(n) <= 9007199254740991
	case []any:
		for _, item := range x {
			if !reviewCanonicalValue(item) {
				return false
			}
		}
	case map[string]any:
		for key, item := range x {
			if !norm.NFC.IsNormalString(key) || !reviewCanonicalValue(item) {
				return false
			}
		}
	}
	return true
}

// The general internal decoder intentionally disallows null. Review's first
// expected_record_id is explicitly null; retain exact fields and duplicate-key
// rejection while allowing null only where these closed structs marshal it.
func decodeReview(data []byte, value any) error {
	b, err := strictjson.ReadValue(bytes.NewReader(data), 1<<20)
	if err != nil {
		return ErrInvalid
	}
	if _, err = reviewCanonicalBytes(b); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil {
		return ErrInvalid
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ErrInvalid
	}
	var original, shape any
	if json.Unmarshal(b, &original) != nil || json.Unmarshal(encoded, &shape) != nil || !reviewShape(original, shape) {
		return ErrInvalid
	}
	return nil
}
func reviewShape(a, b any) bool {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !reviewShape(v, w) {
				return false
			}
		}
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i, v := range x {
			if !reviewShape(v, y[i]) {
				return false
			}
		}
	case nil:
		return b == nil
	case string:
		_, ok := b.(string)
		return ok
	case float64:
		_, ok := b.(float64)
		return ok
	case bool:
		_, ok := b.(bool)
		return ok
	default:
		return false
	}
	return true
}
func reviewOrigin(p Principal) string {
	if p.ReviewOrigin == "human_interactive" {
		return p.ReviewOrigin
	}
	return "agent"
}
func optionalID(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func reviewIDs(values []string) ([]string, error) {
	if values == nil || len(values) > 1000 {
		return nil, ErrInvalid
	}
	out := append([]string{}, values...)
	sort.Strings(out)
	for i, v := range out {
		if v == "" || !utf8.ValidString(v) || utf8.RuneCountInString(v) > 128 || !norm.NFC.IsNormalString(v) || i > 0 && out[i-1] == v {
			return nil, ErrInvalid
		}
	}
	return out, nil
}
func reviewBodyValid(body string) bool {
	return utf8.ValidString(body) && norm.NFC.IsNormalString(body) && strings.TrimSpace(body) != "" && utf8.RuneCountInString(body) <= 20000
}

func (r ReviewAppendRequest) supersedes() (*string, error) {
	if len(r.SupersedesRecordID) == 0 || bytes.Equal(bytes.TrimSpace(r.SupersedesRecordID), []byte("null")) {
		return nil, nil
	}
	var id string
	if json.Unmarshal(r.SupersedesRecordID, &id) != nil || !ValidID(id) {
		return nil, ErrInvalid
	}
	return &id, nil
}
func (r ReviewAppendRequest) technicalRefs() []string {
	if r.TechnicalRefs == nil {
		return []string{}
	}
	return *r.TechnicalRefs
}
