//go:build integration

package project

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/verdantflarehub/verdantflare-app-project/internal/coordination"
	"github.com/verdantflarehub/verdantflare-app-project/internal/testdb"
	"github.com/verdantflarehub/verdantflare-app-project/migrations"
)

// Only Artifact transport is substituted. The service, MCP/HTTP handlers,
// database commits, advisory locks, CAS and socket failures are real.
type entryArtifact struct {
	*integrationArtifact
	mu     sync.Mutex
	writes int
}

func (a *entryArtifact) Write(ctx context.Context, p Principal, project string, w TextWrite) (ContentVersion, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.writes++
	return a.integrationArtifact.Write(ctx, p, project, w)
}
func (a *entryArtifact) Metadata(ctx context.Context, p Principal, ref ContentRef, access Access) (ContentVersion, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.integrationArtifact.Metadata(ctx, p, ref, access)
}
func (a *entryArtifact) Read(ctx context.Context, p Principal, ref ContentRef, access Access, limit int64) ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.integrationArtifact.Read(ctx, p, ref, access, limit)
}
func (a *entryArtifact) writeCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.writes
}

type entryWire struct {
	server     *httptest.Server
	dropNext   atomic.Bool
	dropStatus atomic.Int32
	barrier    atomic.Pointer[sync.WaitGroup]
}

const entryToken = "synthetic-project-transport-token-0000"

func newEntryWire(t *testing.T, s *Service) *entryWire {
	t.Helper()
	h, err := s.HTTPHandler(entryToken)
	if err != nil {
		t.Fatal(err)
	}
	mcp, err := s.MCPHandler(entryToken)
	if err != nil {
		t.Fatal(err)
	}
	wire := &entryWire{}
	wire.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if barrier := wire.barrier.Load(); barrier != nil {
			barrier.Done()
			barrier.Wait()
		}
		handler := h
		if r.URL.Path == "/mcp" {
			handler = mcp
		}
		if wire.dropNext.Swap(false) {
			out := httptest.NewRecorder()
			handler.ServeHTTP(out, r)
			wire.dropStatus.Store(int32(out.Code))
			// Drop the actual TCP response only after the handler committed.
			panic(http.ErrAbortHandler)
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(wire.server.Close)
	return wire
}

func (w *entryWire) call(p Principal, tool string, args any, mcp bool, out any) (int, error) {
	path := "/internal/v1/" + strings.Replace(tool, ".", "/", 1)
	var payload any = args
	if mcp {
		path = "/mcp"
		payload = map[string]any{"jsonrpc": "2.0", "id": p.RequestID, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	r, err := http.NewRequest("POST", w.server.URL+path, bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+entryToken)
	r.Header.Set("X-Organization-Id", p.OrganizationID)
	r.Header.Set("X-User-Id", p.SubjectID)
	r.Header.Set("X-Request-Id", p.RequestID)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(r)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, err = io.ReadAll(resp.Body)
	if err != nil || out == nil {
		return resp.StatusCode, err
	}
	if mcp {
		var envelope struct {
			Result struct {
				Data json.RawMessage `json:"structuredContent"`
			} `json:"result"`
		}
		if err = json.Unmarshal(b, &envelope); err != nil {
			return resp.StatusCode, err
		}
		b = envelope.Result.Data
	}
	return resp.StatusCode, json.Unmarshal(b, out)
}

func TestProjectEntryLostResponseReplayAndConcurrentCAS(t *testing.T) {
	t.Setenv("PROJECT_CONTENT_CACHE_DIR", t.TempDir())
	ctx := context.Background()
	db := testdb.New(t)
	if err := migrations.ApplyFresh(ctx, db, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := migrations.ActivateFresh(ctx, db); err != nil {
		t.Fatal(err)
	}
	content := &entryArtifact{integrationArtifact: &integrationArtifact{versions: map[ContentRef]ContentVersion{}, bytes: map[ContentRef][]byte{}}}
	newService := func() *Service {
		t.Helper()
		store, err := coordination.NewStore(db, "project", 1, AuthorizeReservation)
		if err != nil {
			t.Fatal(err)
		}
		s, err := NewService(ctx, db, content, integrationAssets{}, &coordination.Engine{Store: store, Peers: map[string]coordination.Peer{"world": &integrationPeer{}}})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	a, b := newEntryWire(t, newService()), newEntryWire(t, newService())
	p := Principal{OrganizationID: newID(), SubjectID: newID(), RequestID: newID()}
	var created Result
	if status, err := a.call(p, "project.create", CreateRequest{CommitID: newID(), Name: "Transport fixture", Category: "image", EntryPath: "review.md", EntryText: "# Synthetic"}, false, &created); err != nil || status != 200 {
		t.Fatalf("create: status=%d error=%T", status, err)
	}
	runs := []RunRef{{ServiceID: "video", RunID: "synthetic-run-a"}, {ServiceID: "video", RunID: "synthetic-run-b"}}
	selections := []Selection{{Purpose: "review", FileIDs: []string{created.Manifest.EntryDocumentID}}}
	request := CommitRequest{ProjectID: created.ProjectID, ExpectedRevisionID: created.RevisionID, CommitID: newID(), Changes: &Changes{RunRefs: &runs, Selections: &selections}}
	if _, err := changed(created.Manifest, request); err != nil {
		t.Fatal("invalid association fixture before transport fault")
	}
	a.dropNext.Store(true)
	if _, err := a.call(p, "project.commit", request, true, nil); err == nil || a.dropStatus.Load() != 200 {
		t.Fatalf("response fault did not follow commit: handler_status=%d transport_error=%T", a.dropStatus.Load(), err)
	}
	writes := content.writeCount()
	a.server.Close()
	a = newEntryWire(t, newService())
	var saved CommitStatus
	if status, err := a.call(p, "project.commit_status", StatusRequest{ProjectID: created.ProjectID, CommitID: request.CommitID}, false, &saved); err != nil || status != 200 || saved.State != "committed" || saved.Result == nil {
		t.Fatal("reconstructed HTTP entry could not recover original MCP commit")
	}
	for _, useMCP := range []bool{false, true} {
		var result Result
		if status, err := b.call(p, "project.commit", request, useMCP, &result); err != nil || status != 200 || result.RevisionID != saved.Result.RevisionID || content.writeCount() != writes {
			t.Fatal("replay changed revision or wrote content again")
		}
	}
	assertRevisions := func(want int) {
		t.Helper()
		var count int
		if err := db.QueryRow(ctx, "SELECT count(*) FROM studio.project_revisions WHERE project_id=$1", created.ProjectID).Scan(&count); err != nil || count != want {
			t.Fatalf("revision count: got %d want %d", count, want)
		}
	}
	assertRevisions(2)
	// Two service objects share only PostgreSQL, not a process-local task lock.
	parallel := func(left, right CommitRequest) ([2]int, [2]Result) {
		t.Helper()
		var barrier sync.WaitGroup
		barrier.Add(2)
		a.barrier.Store(&barrier)
		b.barrier.Store(&barrier)
		var wg sync.WaitGroup
		var statuses [2]int
		var results [2]Result
		var errs [2]error
		for i, wire := range []*entryWire{a, b} {
			wg.Add(1)
			go func(i int, wire *entryWire) {
				defer wg.Done()
				r := left
				if i == 1 {
					r = right
				}
				statuses[i], errs[i] = wire.call(p, "project.commit", r, i == 1, &results[i])
			}(i, wire)
		}
		wg.Wait()
		a.barrier.Store(nil)
		b.barrier.Store(nil)
		for _, err := range errs {
			if err != nil {
				t.Fatalf("parallel transport: %T", err)
			}
		}
		return statuses, results
	}
	name := "Concurrent same operation"
	next := CommitRequest{ProjectID: created.ProjectID, ExpectedRevisionID: saved.Result.RevisionID, CommitID: newID(), Changes: &Changes{Metadata: &MetadataChange{Name: &name}}}
	statuses, results := parallel(next, next)
	if statuses != [2]int{200, 200} || results[0].RevisionID != results[1].RevisionID || content.writeCount() != writes+1 {
		t.Fatal("same operation was not serialized across entries")
	}
	assertRevisions(3)
	leftName, rightName := "Left choice", "Right choice"
	left := CommitRequest{ProjectID: created.ProjectID, ExpectedRevisionID: results[0].RevisionID, CommitID: newID(), Changes: &Changes{Metadata: &MetadataChange{Name: &leftName}}}
	right := left
	right.CommitID, right.Changes = newID(), &Changes{Metadata: &MetadataChange{Name: &rightName}}
	statuses, results = parallel(left, right)
	if statuses != [2]int{200, 409} && statuses != [2]int{409, 200} {
		t.Fatalf("CAS did not choose exactly one winner: %v", statuses)
	}
	winner := 0
	if statuses[0] != 200 {
		winner = 1
	}
	var opened OpenResult
	if status, err := a.call(p, "project.open", OpenRequest{ProjectID: created.ProjectID}, false, &opened); err != nil || status != 200 || opened.RevisionID != results[winner].RevisionID || !contentEqual(opened.Manifest.Selections, selections) || !contentEqual(opened.Manifest.RunRefs, runs) {
		t.Fatal("CAS winner or prior selections/run provenance lost")
	}
	assertRevisions(4)
	// A changed request and a different actor cannot recover an earlier receipt.
	altered := request
	altered.Changes = &Changes{Metadata: &MetadataChange{Name: &leftName}}
	if status, err := a.call(p, "project.commit", altered, true, nil); err != nil || status != 409 {
		t.Fatal("same ID accepted changed intent")
	}
	stranger := p
	stranger.SubjectID = newID()
	if status, err := a.call(stranger, "project.commit_status", StatusRequest{ProjectID: created.ProjectID, CommitID: request.CommitID}, true, nil); err != nil || status != 403 {
		t.Fatal("foreign actor recovered receipt")
	}
	assertRevisions(4)
}
