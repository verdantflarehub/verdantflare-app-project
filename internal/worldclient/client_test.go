package worldclient

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/verdantflarehub/verdantflare-app-project/internal/coordination"
	"github.com/verdantflarehub/verdantflare-app-project/internal/project"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func identifier(n int) string                                         { return fmt.Sprintf("01900000-0000-7000-8000-%012d", n) }
func TestWorldClosureRequiresPinnedRootAndReadOnlyGrants(t *testing.T) {
	ref := project.AssetRef{AssetID: identifier(1), VersionID: identifier(2), Purpose: "reference"}
	p := project.Principal{OrganizationID: identifier(3), SubjectID: identifier(4), RequestID: identifier(5)}
	for _, grants := range [][]coordination.Grant{{{ResourceID: ref.AssetID}}, {{ResourceID: ref.AssetID, Write: true}}, {{ResourceID: identifier(8)}}, nil} {
		c, err := New("https://world.internal", strings.Repeat("w", 32))
		if err != nil {
			t.Fatal(err)
		}
		c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/internal/v1/world/references" || r.Header.Get("X-User-Id") != p.SubjectID || r.Header.Get("X-Organization-Id") != p.OrganizationID {
				t.Fatal("wrong trusted scope")
			}
			var sent project.AssetRef
			if json.NewDecoder(r.Body).Decode(&sent) != nil || sent != ref {
				t.Fatal("changed immutable reference")
			}
			data, _ := json.Marshal(project.AssetResolution{Refs: []project.ContentRef{{StoreID: identifier(10), ArtifactID: identifier(11), VersionID: identifier(12)}}, Grants: grants})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
		})
		_, err = c.Resolve(context.Background(), p, ref)
		wantOK := len(grants) == 1 && grants[0].ResourceID == ref.AssetID && !grants[0].Write
		if (err == nil) != wantOK {
			t.Fatalf("closure validation: %v", err)
		}
	}
}
