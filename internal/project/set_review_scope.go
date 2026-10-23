package project

import (
	"sort"
	"strings"
)

func normalizeReviewScope(scope ReviewScope) (ReviewScope, error) {
	if scope.Kind != "evidence" && scope.Kind != "camera" && scope.Kind != "region" && scope.Kind != "full_target" {
		return scope, ErrInvalid
	}
	var err error
	if scope.EvidenceIDs, err = reviewIDs(scope.EvidenceIDs); err != nil {
		return scope, err
	}
	if scope.CameraIDs, err = reviewIDs(scope.CameraIDs); err != nil {
		return scope, err
	}
	if scope.RegionIDs, err = reviewIDs(scope.RegionIDs); err != nil {
		return scope, err
	}
	if scope.Claims, err = reviewIDs(scope.Claims); err != nil || len(scope.Claims) == 0 {
		return scope, ErrInvalid
	}
	for _, claim := range scope.Claims {
		if claim != "image_reading" && claim != "space_layout" && claim != "source_recovery" && claim != "new_camera" && claim != "full_set" {
			return scope, ErrInvalid
		}
		if claim == "full_set" && scope.Kind != "full_target" {
			return scope, ErrInvalid
		}
	}
	if scope.Kind == "evidence" && len(scope.EvidenceIDs) == 0 || scope.Kind == "camera" && len(scope.CameraIDs) == 0 || scope.Kind == "region" && len(scope.RegionIDs) == 0 {
		return scope, ErrInvalid
	}
	if rect := scope.ImageRect; rect != nil {
		if len(scope.EvidenceIDs) != 1 || rect.X < 0 || rect.Y < 0 || rect.W <= 0 || rect.H <= 0 || rect.X+rect.W > 1 || rect.Y+rect.H > 1 {
			return scope, ErrInvalid
		}
	}
	if f := scope.FrameRange; f != nil {
		if len(scope.EvidenceIDs) != 1 || f.Start < 0 || f.End < f.Start || f.FPSNumerator < 1 || f.FPSDenominator < 1 {
			return scope, ErrInvalid
		}
	}
	return scope, nil
}
func includes(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func subsetReview(values, allowed []string) bool {
	for _, v := range values {
		if !includes(allowed, v) {
			return false
		}
	}
	return true
}
func validateReviewScope(target ReviewTarget, scope ReviewScope, action string) error {
	normal, err := normalizeReviewScope(scope)
	if err != nil {
		return err
	}
	scope = normal
	ids := []string{}
	images := map[string]bool{}
	evidence := map[string]ReviewEvidence{}
	for _, e := range target.Evidence {
		ids = append(ids, e.EvidenceID)
		evidence[e.EvidenceID] = e
		images[e.EvidenceID] = strings.HasPrefix(e.MIME, "image/") || strings.HasPrefix(e.MIME, "video/")
	}
	if !subsetReview(scope.EvidenceIDs, ids) || !subsetReview(scope.CameraIDs, target.Coverage.CameraIDs) || !subsetReview(scope.RegionIDs, target.Coverage.RegionIDs) {
		return ErrInvalid
	}
	if scope.Kind == "camera" || scope.Kind == "region" {
		selected := scope.CameraIDs
		if scope.Kind == "region" {
			selected = scope.RegionIDs
		}
		covered := []string{}
		if len(scope.EvidenceIDs) == 0 {
			return ErrInvalid
		}
		for _, id := range scope.EvidenceIDs {
			mapped := evidence[id].CameraIDs
			if scope.Kind == "region" {
				mapped = evidence[id].RegionIDs
			}
			matched := false
			for _, name := range selected {
				if includes(mapped, name) {
					covered = append(covered, name)
					matched = true
				}
			}
			if !matched {
				return ErrInvalid
			}
		}
		if !subsetReview(selected, covered) {
			return ErrInvalid
		}
	}
	if scope.ImageRect != nil && !strings.HasPrefix(evidence[scope.EvidenceIDs[0]].MIME, "image/") {
		return ErrInvalid
	}
	if f := scope.FrameRange; f != nil {
		source := evidence[scope.EvidenceIDs[0]].FrameRange
		if source == nil || f.Start < source.Start || f.End > source.End || f.FPSNumerator != source.FPSNumerator || f.FPSDenominator != source.FPSDenominator {
			return ErrInvalid
		}
	}
	if action != "approve" {
		return nil
	}
	if includes(scope.Claims, "image_reading") {
		if len(scope.EvidenceIDs) == 0 {
			return ErrReviewScope
		}
		for _, id := range scope.EvidenceIDs {
			if !images[id] && scope.Kind != "full_target" {
				return ErrReviewScope
			}
		}
	}
	for _, claim := range scope.Claims {
		if claim == "image_reading" {
			continue
		}
		found := false
		for _, requirement := range target.Requirements {
			if requirement.Claim == claim {
				found = true
				if requirement.State != "satisfied" || !subsetReview(requirement.EvidenceIDs, scope.EvidenceIDs) {
					return ErrReviewScope
				}
			}
		}
		if !found {
			return ErrReviewScope
		}
	}
	if scope.Kind == "full_target" {
		if !subsetReview([]string{"space_layout", "source_recovery", "full_set"}, scope.Claims) || !subsetReview(ids, scope.EvidenceIDs) || !subsetReview(target.Coverage.CameraIDs, scope.CameraIDs) || !subsetReview(target.Coverage.RegionIDs, scope.RegionIDs) {
			return ErrReviewScope
		}
		for _, r := range target.Requirements {
			if r.Claim != "new_camera" && r.State != "satisfied" {
				return ErrReviewScope
			}
		}
	}
	return nil
}
func requiredReviewScope(scope ReviewScope) bool {
	if scope.Kind == "full_target" {
		return true
	}
	for _, claim := range scope.Claims {
		if claim != "new_camera" {
			return true
		}
	}
	return false
}

// Records are ordered oldest to newest. A later comment never erases a decision,
// and resolving a local objection does not revive an older whole-set approval.
func reviewProjection(records []ReviewRecord) (string, []ReviewDecision) {
	state := "unreviewed"
	decisions := map[string]ReviewDecision{}
	latestRequiredRejection := -1
	latestFullApproval := -1
	for i, r := range records {
		if r.Actor.Origin != "human_interactive" {
			continue
		}
		if state == "unreviewed" {
			state = "in_review"
		}
		if r.Action == "comment" {
			continue
		}
		scope, err := normalizeReviewScope(r.Scope)
		if err != nil {
			continue
		}
		_, hash, err := reviewJSON(scope)
		if err != nil {
			continue
		}
		decisions[hash] = ReviewDecision{ScopeHash: hash, Scope: scope, Action: r.Action, RecordID: r.RecordID}
		if r.Action == "request_changes" && requiredReviewScope(scope) {
			latestRequiredRejection = i
		}
		if r.Action == "approve" && scope.Kind == "full_target" {
			latestFullApproval = i
		}
	}
	out := []ReviewDecision{}
	blocked := false
	for _, d := range decisions {
		out = append(out, d)
		if d.Action == "request_changes" && requiredReviewScope(d.Scope) {
			blocked = true
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ScopeHash < out[j].ScopeHash })
	if blocked {
		state = "rejected"
	} else if latestFullApproval >= 0 && latestFullApproval > latestRequiredRejection {
		state = "approved"
	}
	return state, out
}
func validateReviewDecision(records []ReviewRecord, r ReviewAppendRequest) error {
	supersedes, err := r.supersedes()
	if err != nil {
		return err
	}
	_, decisions := reviewProjection(records)
	scope, err := normalizeReviewScope(r.Scope)
	if err != nil {
		return err
	}
	_, hash, err := reviewJSON(scope)
	if err != nil {
		return err
	}
	var previous string
	for _, d := range decisions {
		if d.ScopeHash == hash {
			previous = d.RecordID
		}
	}
	if r.Action == "comment" {
		if supersedes != nil {
			return ErrInvalid
		}
		return nil
	}
	if previous != optionalID(supersedes) {
		return ErrReviewConflict
	}
	if r.Action == "approve" && scope.Kind == "full_target" {
		for _, d := range decisions {
			if d.Action == "request_changes" && requiredReviewScope(d.Scope) && d.ScopeHash != hash {
				return ErrReviewScope
			}
		}
	}
	return nil
}
