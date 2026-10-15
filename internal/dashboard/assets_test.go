package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	html := []byte("<!doctype html><title>Project</title>")
	index, _ := json.Marshal(map[string]any{"schemaVersion": 1, "files": map[string]string{"index.html": hash(html)}})
	declaration, _ := json.Marshal(map[string]any{"schemaVersion": 1, "appId": "project", "appVersion": "0.1.0", "dashboard": map[string]any{"entryRef": "dashboard/index.html", "artifactDigest": "sha256:" + hash(index), "adapter": "wujie", "sdkMajor": 1, "minimumSdkVersion": "1.1.0", "requiredCapabilities": []string{"app.request", "navigation.request"}, "optionalCapabilities": []string{"artifact.read"}, "appApiPolicyRef": "project-dashboard-v1", "keepAlive": true}})
	navigation, _ := json.Marshal(map[string]any{"schemaVersion": 1, "appId": "project", "appVersion": "0.1.0", "artifactDigest": "sha256:" + hash(index), "entry": map[string]any{"entryId": "main", "title": "项目", "iconRef": "folder", "route": "/", "accessRuleRef": "project-read"}})
	for name, data := range map[string][]byte{"index.html": html, "asset-index.json": index, "dashboard-declaration.json": declaration, "navigation-declaration.json": navigation} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func TestPackageBindsReleaseAndServesOriginalBytesBehindTrustedIdentity(t *testing.T) {
	root := fixture(t)
	a, err := Load(root, "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	h, err := a.Handler(strings.Repeat("s", 32))
	if err != nil {
		t.Fatal(err)
	}
	for _, authenticated := range []bool{false, true} {
		req := httptest.NewRequest("GET", "/internal/dashboard/navigation", nil)
		if authenticated {
			req.Header.Set("Authorization", "Bearer "+strings.Repeat("s", 32))
			for _, header := range []string{"X-User-Id", "X-Organization-Id", "X-Request-Id"} {
				req.Header.Set(header, "01900000-0000-7000-8000-000000000001")
			}
		}
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if !authenticated && res.Code != 403 {
			t.Fatal("public package access")
		}
		if authenticated && (res.Code != 200 || !bytes.Equal(res.Body.Bytes(), a.Navigation())) {
			t.Fatal("navigation bytes were changed")
		}
	}
	if _, _, ok := a.File("navigation-declaration.json"); ok {
		t.Fatal("navigation in frontend index")
	}
	if _, _, ok := a.File("../index.html"); ok {
		t.Fatal("path escaped")
	}
	if _, err = Load(root, "0.2.0"); err == nil {
		t.Fatal("wrong release accepted")
	}
}
func TestPackageRejectsAssetAndNavigationTampering(t *testing.T) {
	for _, name := range []string{"index.html", "navigation-declaration.json"} {
		root := fixture(t)
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("tampered"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(root, "0.1.0"); err == nil {
			t.Fatalf("tampered %s accepted", name)
		}
	}
}

func TestPackageAcceptsCurrentNavigationTitleAndRejectsUnknownTitle(t *testing.T) {
	for _, title := range []string{"项目管理", "其他应用"} {
		root := fixture(t)
		path := filepath.Join(root, "navigation-declaration.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var nav map[string]any
		if err = json.Unmarshal(raw, &nav); err != nil {
			t.Fatal(err)
		}
		nav["entry"].(map[string]any)["title"] = title
		raw, _ = json.Marshal(nav)
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		_, err = Load(root, "0.1.0")
		if (title == "项目管理") != (err == nil) {
			t.Fatal(title, err)
		}
	}
}

func TestPackageAcceptsFourBusinessNavigationAndRejectsTamperedChildren(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(root, "navigation-declaration.json")
	raw, _ := os.ReadFile(path)
	var nav map[string]any
	if err := json.Unmarshal(raw, &nav); err != nil {
		t.Fatal(err)
	}
	nav["schemaVersion"] = 2
	entry := nav["entry"].(map[string]any)
	entry["title"], entry["route"] = "项目管理", ""
	children := []navigationChild{
		{"character", "角色设计", "/projects?category=character", "project-read", 0},
		{"music", "音乐制作", "/projects?category=music", "project-read", 1},
		{"video", "创意视频", "/projects?category=video", "project-read", 2},
		{"ai_short_drama", "短剧制作", "/projects?category=ai_short_drama", "project-read", 3},
	}
	entry["children"] = children
	raw, _ = json.Marshal(nav)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, "0.1.0"); err != nil {
		t.Fatal("generated navigation rejected", err)
	}
	children[2].Route = "/projects"
	raw, _ = json.Marshal(nav)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, "0.1.0"); err == nil {
		t.Fatal("aggregate child route accepted")
	}
}
