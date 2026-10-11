package project

import "context"

func (s *Service) UseAsset(ctx context.Context, p Principal, r UseAssetRequest) (Result, error) {
	if !ValidID(r.AssetID) || !ValidID(r.VersionID) || !ValidID(r.ExpectedRevisionID) || !ValidID(r.CommitID) || !validText(r.Purpose) {
		return Result{}, ErrInvalid
	}
	if e := writable(ctx, s.db, p, r.ProjectID); e != nil {
		return Result{}, e
	}
	previous, _, e := revision(ctx, s.db, p.OrganizationID, r.ProjectID, r.ExpectedRevisionID)
	if e != nil {
		return Result{}, e
	}
	return s.Commit(ctx, p, useAssetCommit(previous, r))
}

func useAssetCommit(previous Manifest, r UseAssetRequest) CommitRequest {
	refs := []AssetRef{}
	for _, a := range previous.AssetRefs {
		if a.AssetID == r.AssetID && a.Purpose == r.Purpose {
			continue
		}
		refs = append(refs, a)
	}
	refs = append(refs, AssetRef{AssetID: r.AssetID, VersionID: r.VersionID, Purpose: r.Purpose})
	return CommitRequest{ProjectID: r.ProjectID, ExpectedRevisionID: r.ExpectedRevisionID, CommitID: r.CommitID, Changes: &Changes{AssetRefs: &refs}}
}
