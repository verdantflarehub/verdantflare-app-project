package project

import "testing"

func TestUseAssetCommitKeepsOtherReferencesAndSharedRecoveryHash(t *testing.T) {
	id := func(n string) string { return "01900000-0000-7000-8000-00000000000" + n }
	request := UseAssetRequest{ProjectID: id("1"), ExpectedRevisionID: id("2"), CommitID: id("3"), AssetID: id("4"), VersionID: id("5"), Purpose: "配乐<&\u2028"}
	previous := Manifest{AssetRefs: []AssetRef{{AssetID: id("6"), VersionID: id("7"), Purpose: "旁白"}, {AssetID: id("4"), VersionID: id("8"), Purpose: request.Purpose}}}
	commit := useAssetCommit(previous, request)
	refs := *commit.Changes.AssetRefs
	if len(refs) != 2 || refs[0] != previous.AssetRefs[0] || refs[1].VersionID != id("5") || previous.AssetRefs[1].VersionID != id("8") {
		t.Fatal("asset replacement changed unrelated or original references")
	}
	// Shared with dashboard/tests/asset-import.test.ts, including non-ASCII and
	// HTML/line separator escaping in the existing CommitRequest digest.
	got, err := digest(commit)
	if err != nil || got != "2f959ca57040652db2ecb009cd1dbd3a0655ff98af07702c45c0f4e34114cebe" {
		t.Fatalf("recovery fingerprint drift: %s %v", got, err)
	}
	previous.AssetRefs = append(previous.AssetRefs, AssetRef{AssetID: id("4"), VersionID: id("8"), Purpose: "其他用途"})
	refs = *useAssetCommit(previous, request).Changes.AssetRefs
	if len(refs) != 3 || refs[1].Purpose != "其他用途" || refs[1].VersionID != id("8") {
		t.Fatal("same asset in another purpose was replaced")
	}
}
