package artifactclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/verdantflarehub/verdantflare-app-project/internal/project"
)

type readTransport func(*http.Request) (*http.Response, error)

func (f readTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestReadAcceptsPosterBudgetAndStillChecksBytes(t *testing.T) {
	id := "01900000-0000-7000-8000-000000000001"
	p := project.Principal{OrganizationID: id, SubjectID: id, RequestID: id}
	ref := project.ContentRef{StoreID: id, ArtifactID: id, VersionID: id}
	body := []byte("small video source with a poster-sized read budget")
	hash := sha256.Sum256(body)
	v := project.ContentVersion{SchemaVersion: 2, ContentRef: ref, OrganizationID: id, VersionNo: 1, CreatedBy: id, CreatedAt: time.Now(), Size: int64(len(body)), SHA256: hex.EncodeToString(hash[:]), MIME: "video/mp4"}
	corrupt := false
	requests := 0
	c := &Client{base: "https://artifact.test", token: strings.Repeat("t", 32), http: &http.Client{Transport: readTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		data, _ := json.Marshal(v)
		if strings.HasSuffix(r.URL.Path, "/content") {
			data = append([]byte(nil), body...)
			if corrupt {
				data[0] ^= 1
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}}
	got, err := c.Read(context.Background(), p, ref, project.Access{ProjectID: id, RevisionID: id}, 128<<20)
	if err != nil || !bytes.Equal(got, body) || requests != 2 {
		t.Fatalf("poster read: bytes=%d requests=%d error=%v", len(got), requests, err)
	}
	corrupt = true
	if _, err := c.Read(context.Background(), p, ref, project.Access{}, 128<<20); !errors.Is(err, project.ErrNotReady) {
		t.Fatalf("corrupt source accepted: %v", err)
	}
	before := requests
	if _, err := c.Read(context.Background(), p, ref, project.Access{}, 128<<20+1); !errors.Is(err, project.ErrInvalid) || requests != before {
		t.Fatalf("unbounded read accepted: %v", err)
	}
}
