package migrations

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestLegacyEvidenceAndDomainOwnership(t *testing.T) {
	for name, want := range map[string]string{"0001_project_revisions.sql": "d223dcb2044805e6b3c1b4ca20e3ce2877a9ae57323116c631b8a616844cda6c", "0002_world_assets.sql": "288b06d279a5c9a81f0ed08ad257e7930676276afacb5f12d940f77fd1e83f7f"} {
		data, err := files.ReadFile("legacy/" + name)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != want {
			t.Fatalf("legacy evidence modified: %s", name)
		}
	}
	sql, _, err := source()
	if err != nil {
		t.Fatal(err)
	}
	for _, foreign := range []string{"studio.world_assets", "studio.world_commits", "studio.world_asset_versions", "studio.studio_events"} {
		if strings.Contains(sql, foreign) {
			t.Fatalf("foreign schema ownership: %s", foreign)
		}
	}
}
