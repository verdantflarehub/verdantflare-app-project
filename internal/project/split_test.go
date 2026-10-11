package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/verdantflarehub/verdantflare-app-project/internal/coordination"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndependentProjectRejectsForeignAuthorityAndRoutes(t *testing.T) {
	s := &Service{}
	p := Principal{OrganizationID: id(101), SubjectID: id(102), RequestID: id(103)}
	for _, permission := range []Permission{
		{Action: "write", Source: &Source{Kind: "asset_manifest", AssetID: id(104), AssetVersionID: id(105)}},
		{Action: "read", Version: &ContentVersion{ContentRef: ContentRef{id(110), id(111), id(112)}, OrganizationID: p.OrganizationID}, Access: &Access{AssetID: id(104), AssetVersionID: id(105)}},
		{Action: "retain", Owner: &RetentionOwner{Kind: "asset_version", ID: id(105), CommitID: id(106)}},
	} {
		if err := s.AuthorizeArtifact(context.Background(), p, permission); !errors.Is(err, ErrForbidden) {
			t.Fatalf("foreign authority accepted: %v", err)
		}
	}
	token := strings.Repeat("s", 32)
	h, err := s.HTTPHandler(token, strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/internal/v1/world/register", "/internal/v1/world/get", "/internal/v1/world/references"} {
		req := httptest.NewRequest("POST", path, bytes.NewBufferString("{}"))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Organization-Id", p.OrganizationID)
		req.Header.Set("X-User-Id", p.SubjectID)
		req.Header.Set("X-Request-Id", p.RequestID)
		out := httptest.NewRecorder()
		h.ServeHTTP(out, req)
		if out.Code != 404 {
			t.Fatalf("foreign route status: %d", out.Code)
		}
	}
	for _, tool := range MCPTools() {
		if !strings.HasPrefix(tool.Name, "project.") {
			t.Fatalf("foreign MCP tool %s", tool.Name)
		}
	}
}
func TestProjectPreparationPersistsExactAuthorizationFence(t *testing.T) {
	p := Principal{OrganizationID: id(101), SubjectID: id(102), RequestID: id(103)}
	c := commitRecord{ProjectID: id(1), CommitID: id(2), Actor: p.SubjectID, Hash: strings.Repeat("a", 64)}
	// A stored operation survives process reconstruction and is never replaced by
	// an in-memory newer fence. A fenced recovery must reconcile explicitly.
	data := []byte(`{"operation":{"coordinator":"project","organization_id":"` + p.OrganizationID + `","subject_id":"` + p.SubjectID + `","resource_id":"` + c.ProjectID + `","commit_id":"` + c.CommitID + `","request_sha256":"` + c.Hash + `","fence":4},"authorization":{"project":[{"resource_id":"` + c.ProjectID + `","write":true}]}}`)
	if err := json.Unmarshal(data, &c.Plan); err != nil {
		t.Fatal(err)
	}
	s := &Service{}
	if o := s.operation(p, c); o.Fence != 4 || !o.Valid() {
		t.Fatal("lost fixed recovery identity")
	}
	s.coord = &coordination.Engine{Store: &coordination.Store{Fence: 5}}
	if s.validOperation(p, c) {
		t.Fatal("old operation adopted a new fence")
	}
	c.Plan.Operation = nil
	if s.operation(p, c).Valid() || s.validOperation(p, c) {
		t.Fatal("missing operation reconstructed from current fence")
	}
}
