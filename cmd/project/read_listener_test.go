package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/verdantflarehub/verdantflare-app-project/internal/project"
)

func TestReadListenerSurface(t *testing.T) {
	called := 0
	body := `{"project_id":"fixed-project","revision_id":"fixed-revision","require_write":true}`
	h := projectReadHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		data, _ := io.ReadAll(r.Body)
		if string(data) != body || r.Header.Get("X-User-Id") != "original-subject" {
			t.Fatal("request changed before existing permission checks")
		}
		w.WriteHeader(204)
	}), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"POST", "/internal/v1/project/open", 204}, {"GET", "/health", 204},
		{"POST", "/internal/v1/project/commit", 404}, {"POST", "/internal/v1/project/member_change", 404},
		{"POST", "/internal/v1/artifact/authorize", 404}, {"POST", "/internal/v1/coordination/prepare", 404},
		{"POST", "/mcp", 404}, {"GET", "/mcp", 404}, {"GET", "/internal/dashboard/", 404},
		{"GET", "/internal/v1/project/open", 404}, {"POST", "/health", 404},
		{"POST", "/internal/v1/project/open?", 404}, {"POST", "/internal/v1/project/open?x=1", 404},
		{"POST", "/internal/v1/project/op%65n", 404}, {"POST", "/internal/v1/project/open/", 404},
		{"POST", "/internal/v1/project/../project/open", 404}, {"GET", "/health?", 404},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(body))
		r.Header.Set("X-User-Id", "original-subject")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s %s: got %d want %d", tc.method, tc.path, w.Code, tc.want)
		}
	}
	if called != 1 {
		t.Fatalf("open reached %d times", called)
	}
}

func TestReadListenerConfiguration(t *testing.T) {
	readToken, businessToken := strings.Repeat("r", 32), strings.Repeat("b", 32)
	for _, tc := range []struct {
		read, token   string
		secure, valid bool
	}{
		{"", "", false, true}, {"0.0.0.0:8096", readToken, true, true},
		{"127.0.0.1:8096", readToken, false, true}, {"[::1]:8096", readToken, false, true},
		{"0.0.0.0:8096", readToken, false, false}, {"localhost:8096", readToken, false, false},
		{"0.0.0.0:8443", readToken, true, false}, {"0.0.0.0:8444", readToken, true, false},
		{"0.0.0.0:08443", readToken, true, false}, {"bad", readToken, true, false},
		{"0.0.0.0:0", readToken, true, false}, {"0.0.0.0:65536", readToken, true, false},
		{"0.0.0.0:8096", "", true, false}, {"", readToken, true, false},
		{"0.0.0.0:8096", "short", true, false}, {"0.0.0.0:8096", readToken + " ", true, false},
		{"0.0.0.0:8096", businessToken, true, false},
	} {
		err := validateReadListener("0.0.0.0:8443", "0.0.0.0:8444", tc.read, tc.secure, tc.token, []string{businessToken})
		if (err == nil) != tc.valid {
			t.Errorf("read=%q secure=%v: valid=%v, error=%v", tc.read, tc.secure, tc.valid, err)
		}
	}
}

func TestReadListenerUsesIndependentCredentialAndPrincipal(t *testing.T) {
	readToken := strings.Repeat("r", 32)
	var svc *project.Service // Rejected requests must never reach storage.
	internal, err := svc.HTTPHandler(readToken, strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	h := projectReadHandler(internal, http.NotFoundHandler())
	for _, tc := range []struct {
		token, subject string
		duplicate      bool
	}{
		{strings.Repeat("b", 32), "01a12f5b-6448-7c00-bac2-629b797b19a5", false},
		{"", "01a12f5b-6448-7c00-bac2-629b797b19a5", false},
		{readToken, "invalid", false},
		{readToken, "01a12f5b-6448-7c00-bac2-629b797b19a5", true},
	} {
		r := httptest.NewRequest("POST", "/internal/v1/project/open", strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+tc.token)
		r.Header.Set("X-Organization-Id", "01a12f5b-6448-7c00-bac2-629b797b19a5")
		r.Header.Set("X-User-Id", tc.subject)
		r.Header.Set("X-Request-Id", "01a12f5b-6448-7c00-bac2-629b797b19a5")
		r.Header.Set("Content-Type", "application/json")
		if tc.duplicate {
			r.Header.Add("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("unauthorized request: %d", w.Code)
		}
	}
}
