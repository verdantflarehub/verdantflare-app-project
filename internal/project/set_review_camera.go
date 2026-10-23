package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// This validates a fixed still-frame record, not a renderer or recovery report.
type reviewCameraDocument struct {
	SchemaVersion    int    `json:"schema_version"`
	Kind             string `json:"kind"`
	SpaceBasisSHA256 string `json:"space_basis_sha256"`
	CoordinateSystem string `json:"coordinate_system"`
	Camera           struct {
		Projection   string    `json:"projection"`
		Location     []float64 `json:"location_blender_m"`
		Rotation     []float64 `json:"rotation_euler_xyz_rad"`
		Lens         float64   `json:"lens_mm"`
		SensorWidth  float64   `json:"sensor_width_mm"`
		SensorHeight float64   `json:"sensor_height_mm"`
		SensorFit    string    `json:"sensor_fit"`
		ClipStart    float64   `json:"clip_start_m"`
		ClipEnd      float64   `json:"clip_end_m"`
		OrthoScale   float64   `json:"ortho_scale"`
		ShiftX       float64   `json:"shift_x"`
		ShiftY       float64   `json:"shift_y"`
	} `json:"camera"`
	Render struct {
		ResolutionX  int64   `json:"resolution_x"`
		ResolutionY  int64   `json:"resolution_y"`
		PixelAspectX float64 `json:"pixel_aspect_x"`
		PixelAspectY float64 `json:"pixel_aspect_y"`
		Frame        int64   `json:"frame"`
	} `json:"render"`
}

func verifyReviewCamera(raw []byte, target ReviewTarget, camera ReviewCamera) bool {
	if target.Coverage.SpaceBasis == nil || !reviewHashPattern.MatchString(target.Coverage.SpaceBasis.SHA256) || len(camera.EvidenceIDs) == 0 {
		return false
	}
	var document reviewCameraDocument
	if decodeReview(raw, &document) != nil || document.SchemaVersion != 1 || document.Kind != "set-review-camera" || document.CoordinateSystem != "blender_z_up_m" || document.SpaceBasisSHA256 != target.Coverage.SpaceBasis.SHA256 {
		return false
	}
	if _, _, err := reviewJSON(document); err != nil {
		return false
	}
	c := document.Camera
	r := document.Render
	if c.Projection != "PERSP" && c.Projection != "ORTHO" || len(c.Location) != 3 || len(c.Rotation) != 3 || c.Lens <= 0 || c.SensorWidth <= 0 || c.SensorHeight <= 0 || c.ClipStart <= 0 || c.ClipEnd <= c.ClipStart || c.OrthoScale <= 0 || c.SensorFit != "AUTO" && c.SensorFit != "HORIZONTAL" && c.SensorFit != "VERTICAL" || r.ResolutionX < 1 || r.ResolutionY < 1 || r.PixelAspectX <= 0 || r.PixelAspectY <= 0 {
		return false
	}
	files := map[string]ReviewEvidence{}
	for _, e := range target.Evidence {
		files[e.EvidenceID] = e
	}
	basis, ok := files[target.Coverage.SpaceBasis.EvidenceID]
	if !ok || basis.SHA256 != document.SpaceBasisSHA256 {
		return false
	}
	parameters, ok := files[camera.ParametersEvidenceID]
	if !ok || parameters.MIME != "application/json" || parameters.SHA256 != camera.ParametersSHA256 {
		return false
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != parameters.SHA256 {
		return false
	}
	for _, id := range camera.EvidenceIDs {
		e, ok := files[id]
		if !ok || !strings.HasPrefix(e.MIME, "image/") || e.FrameRange != nil || !includes(e.CameraIDs, camera.CameraID) {
			return false
		}
	}
	return true
}
func (s *Service) verifyReviewCameras(ctx context.Context, p Principal, target *ReviewTarget) error {
	for i, camera := range target.Coverage.Cameras {
		var parameters ReviewEvidence
		for _, e := range target.Evidence {
			if e.EvidenceID == camera.ParametersEvidenceID {
				parameters = e
			}
		}
		if !parameters.ContentRef.Valid() {
			return ErrInvalid
		}
		raw, err := s.content.Read(ctx, p, parameters.ContentRef, Access{ProjectID: target.TargetLocator.ProjectID, RevisionID: target.TargetLocator.ContentRevisionID}, 1<<20)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != parameters.SHA256 {
			return ErrTargetMismatch
		}
		target.Coverage.Cameras[i].Verified = verifyReviewCamera(raw, *target, camera)
	}
	for i, r := range target.Requirements {
		if r.Claim == "new_camera" && r.State == "satisfied" {
			verified := len(target.Coverage.Cameras) > 0
			for _, camera := range target.Coverage.Cameras {
				verified = verified && camera.Verified
			}
			if !verified {
				target.Requirements[i].State = "unverified"
				target.Requirements[i].ReasonCode = "CAMERA_PARAMETERS_UNVERIFIED"
			}
		}
	}
	return nil
}
