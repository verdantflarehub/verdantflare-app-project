package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/verdantflarehub/verdantflare-app-project/internal/project"
)

func dashboardRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/project/create", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("s", 32))
	r.Header.Set("X-Organization-Id", "0199b501-0000-7000-8000-000000000001")
	r.Header.Set("X-User-Id", "0199b501-0000-7000-8000-000000000002")
	r.Header.Set("X-Request-Id", "0199b501-0000-7000-8000-000000000003")
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestDashboardCreatePreservesRequestAndRecoveryResponse(t *testing.T) {
	// Exactly the SDK's byte ceiling, including multibyte text and JSON overhead.
	body := `{"commit_id":"0199b501-0000-7000-8000-000000000004","name":"项目<&>","category":"image","entry_path":"review.md","entry_text":"` + strings.Repeat("界", 349460) + `"}`
	body = strings.TrimSuffix(body, `"}`) + strings.Repeat("x", (1<<20)-len(body)) + `"}`
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable, http.StatusConflict} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			response := []byte(`{"marker":"original-result-or-error"}`)
			calls := 0
			mux := http.NewServeMux()
			registerDashboardAPI(mux, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				data, err := io.ReadAll(r.Body)
				if err != nil || string(data) != body || r.URL.Path != "/internal/v1/project/create" || r.Method != http.MethodPost {
					t.Fatal("create mapping changed original operation")
				}
				for key, values := range dashboardRequest(body).Header {
					if r.Header.Get(key) != values[0] {
						t.Fatal("mapping changed trusted context")
					}
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write(response)
			}))
			request := dashboardRequest(body)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, request)
			if calls != 1 || w.Code != status || !bytes.Equal(w.Body.Bytes(), response) || request.URL.Path != "/api/v1/project/create" {
				t.Fatal("mapping altered result, error or caller request")
			}
		})
	}
}

func TestDashboardCreateRejectsInvalidInputBeforeBusinessIO(t *testing.T) {
	service := &project.Service{}
	internal, err := service.HTTPHandler(strings.Repeat("s", 32), strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerDashboardAPI(mux, internal)
	for _, tc := range []struct {
		name string
		body string
		edit func(*http.Request)
		want int
	}{
		{"byte limit", `{"commit_id":"0199b501-0000-7000-8000-000000000004","name":"Project","category":"image","entry_path":"review.md","entry_text":"` + strings.Repeat("界", 349526) + `"}`, nil, 400},
		{"unknown field", `{"project_id":"client-chosen"}`, nil, 400},
		{"duplicate field", `{"commit_id":"first","commit_id":"second"}`, nil, 400},
		{"missing ID", `{}`, nil, 400},
		{"unauthenticated", `{}`, func(r *http.Request) { r.Header.Del("Authorization") }, 403},
		{"duplicate identity", `{}`, func(r *http.Request) { r.Header.Add("X-User-Id", r.Header.Get("X-User-Id")) }, 403},
		{"wrong method", `{}`, func(r *http.Request) { r.Method = http.MethodGet }, 400},
		{"query", `{}`, func(r *http.Request) { r.URL.RawQuery = "project_id=other" }, 400},
		{"encoding", `{}`, func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") }, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := dashboardRequest(tc.body)
			if tc.edit != nil {
				tc.edit(r)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			var body struct {
				Code string `json:"code"`
			}
			if w.Code != tc.want || json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Code == "" {
				t.Fatal("missing controlled API rejection", w.Code)
			}
		})
	}
	for _, path := range []string{"/api/v1/project/commit", "/api/v1/project/create_status", "/api/v1/project/member_change", "/api/v1/project/create/other"} {
		r := dashboardRequest(`{}`)
		r.URL.Path = path
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatal("unexpected Dashboard operation exposed", path)
		}
	}
}
