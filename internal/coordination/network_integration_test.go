//go:build integration

package coordination_test

import (
	"context"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/verdantflarehub/verdantflare-app-project/internal/coordination"
	"github.com/verdantflarehub/verdantflare-app-project/internal/testdb"
)

// These tests run ONLY in 5090 dev with PROJECT_TEST_DATABASE_URL. They use two
// disposable PostgreSQL databases and real TLS/TCP for every remote Peer call.
// Both domains use this package's production coordination implementation. The
// grant and business-CAS SQL tables below are explicit business substitutes;
// Artifact, full Project/World services, containers and NetworkPolicy are not
// exercised here. No fake Peer or RoundTripper is used.

type wireEndpoint struct {
	server     *httptest.Server
	caFile     string
	mu         sync.Mutex
	handler    http.Handler
	dropPath   string
	dropStatus int
	calls      map[string]int
}

func newWireEndpoint(t *testing.T, h http.Handler, address ...string) *wireEndpoint {
	t.Helper()
	e := &wireEndpoint{handler: h, calls: map[string]int{}}
	e.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		e.calls[r.URL.Path]++
		drop := e.dropPath == r.URL.Path
		if drop {
			e.dropPath = ""
		}
		h := e.handler
		e.mu.Unlock()
		if drop {
			// Execute and commit on the real participant before closing the actual
			// TLS connection without sending any response bytes to its caller.
			buffer := httptest.NewRecorder()
			h.ServeHTTP(buffer, r)
			e.mu.Lock()
			e.dropStatus = buffer.Code
			e.mu.Unlock()
			panic(http.ErrAbortHandler)
		}
		h.ServeHTTP(w, r)
	}))
	if len(address) != 0 {
		// Keep each domain's address across a restart. A fresh ephemeral port can
		// reuse the stopped peer's port and turn an outage into a different peer.
		_ = e.server.Listener.Close()
		listener, err := net.Listen("tcp", address[0])
		if err != nil {
			t.Fatal("cannot rebind isolated coordination listener")
		}
		e.server.Listener = listener
	}
	e.server.Config.ErrorLog = log.New(io.Discard, "", 0)
	e.server.StartTLS()
	t.Cleanup(e.server.Close)
	e.caFile = filepath.Join(t.TempDir(), "coordination-ca.pem")
	if err := os.WriteFile(e.caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: e.server.Certificate().Raw}), 0600); err != nil {
		t.Fatal("write temporary test trust anchor")
	}
	return e
}

func (e *wireEndpoint) dropNext(path string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dropPath = "/internal/v1/coordination/" + path
	e.dropStatus = 0
}

func (e *wireEndpoint) assertSuccessfulDrop(t *testing.T) {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dropPath != "" || e.dropStatus != http.StatusOK {
		t.Fatal("fault did not occur after successful participant persistence")
	}
}

func (e *wireEndpoint) count(path string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls["/internal/v1/coordination/"+path]
}

func (e *wireEndpoint) replace(h http.Handler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handler = h
}

type wireNode struct {
	domain   string
	resource string
	db       *pgxpool.Pool
	store    *coordination.Store
	engine   *coordination.Engine
	endpoint *wireEndpoint
}

type wirePair struct {
	t         *testing.T
	ctx       context.Context
	org       string
	subject   string
	project   *wireNode
	world     *wireNode
	toWorld   *coordination.Client
	toProject *coordination.Client
}

func wireID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal("allocate fixture identity")
	}
	return id.String()
}

func wireOK(t *testing.T, stage string, err error) {
	t.Helper()
	if err != nil {
		// Do not print database connection details or temporary filesystem paths.
		t.Fatalf("%s failed (%T)", stage, err)
	}
}

func newWirePair(t *testing.T) *wirePair {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	t.Cleanup(cancel)
	f := &wirePair{t: t, ctx: ctx, org: wireID(t), subject: wireID(t)}
	f.project = &wireNode{domain: "project", resource: wireID(t), db: testdb.New(t)}
	f.world = &wireNode{domain: "world", resource: wireID(t), db: testdb.New(t)}
	if f.project.db.Config().ConnConfig.Database == f.world.db.Config().ConnConfig.Database {
		t.Fatal("participants must use independent databases")
	}
	for _, n := range []*wireNode{f.project, f.world} {
		schema, err := coordination.Schema(n.domain)
		wireOK(t, "coordination schema", err)
		_, err = n.db.Exec(ctx, "CREATE SCHEMA studio;"+schema+`
CREATE TABLE studio.fixture_grants (organization_id uuid NOT NULL, resource_id uuid NOT NULL, subject_id uuid NOT NULL, writable boolean NOT NULL, PRIMARY KEY(organization_id,resource_id,subject_id));
CREATE TABLE studio.fixture_heads (resource_id uuid PRIMARY KEY, head uuid);
CREATE TABLE studio.fixture_publications (commit_id uuid PRIMARY KEY, resource_id uuid NOT NULL, request_hash text NOT NULL);`)
		wireOK(t, "initialize isolated coordination fixture", err)
		_, err = n.db.Exec(ctx, "INSERT INTO studio."+n.domain+"_coord_fences(coordinator,fence) VALUES('project',1),('world',1)")
		wireOK(t, "provision external fixture fences", err)
		// Consume the actual migration's fixed-domain SECURITY DEFINER helper;
		// Schema() intentionally preserves the original migration checksum.
		_, err = n.db.Exec(ctx, lockedFenceFixtureSQL(t, n.domain))
		wireOK(t, "install actual migration fence reader", err)
		_, err = n.db.Exec(ctx, "INSERT INTO studio.fixture_grants VALUES($1,$2,$3,true)", f.org, n.resource, f.subject)
		wireOK(t, "grant fixture permission", err)
		_, err = n.db.Exec(ctx, "INSERT INTO studio.fixture_heads(resource_id) VALUES($1)", n.resource)
		wireOK(t, "initialize fixture head", err)
		f.makeEngine(n, 1)
		n.endpoint = newWireEndpoint(t, f.handler(n))
	}
	f.connect()
	return f
}

func lockedFenceFixtureSQL(t *testing.T, domain string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "verdantflare-app-"+domain, "migrations", "0003_locked_fence.sql")
	data, err := os.ReadFile(path)
	wireOK(t, "read sibling application's actual handoff migration", err)
	source := string(data)
	marker := "CREATE FUNCTION studio." + domain + "_locked_fence("
	if strings.Count(source, marker) != 1 {
		t.Fatal("actual migration must contain exactly one fixed-domain fence reader")
	}
	start := strings.Index(source, marker)
	end := strings.Index(source[start:], "$$;")
	if end < 0 {
		t.Fatal("actual fence reader function terminator missing")
	}
	return source[start : start+end+3]
}

func fixtureAuthorize(ctx context.Context, tx pgx.Tx, o coordination.Operation, g coordination.Grant) error {
	var writable bool
	err := tx.QueryRow(ctx, "SELECT writable FROM studio.fixture_grants WHERE organization_id=$1 AND resource_id=$2 AND subject_id=$3 FOR SHARE", o.OrganizationID, g.ResourceID, o.SubjectID).Scan(&writable)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && g.Write && !writable {
		return coordination.ErrDenied
	}
	return err
}

func (f *wirePair) makeEngine(n *wireNode, fence int64) {
	f.t.Helper()
	store, err := coordination.NewStore(n.db, n.domain, fence, fixtureAuthorize)
	wireOK(f.t, "rebuild durable store", err)
	n.store = store
	n.engine = &coordination.Engine{Store: store, Peers: map[string]coordination.Peer{}}
}

func (f *wirePair) handler(n *wireNode) http.Handler {
	f.t.Helper()
	caller := "project"
	if n.domain == "project" {
		caller = "world"
	}
	h, err := n.engine.Handler(map[string]string{caller: strings.Repeat(caller, 8)})
	wireOK(f.t, "authenticated coordination handler", err)
	return h
}

func (f *wirePair) connect() {
	f.t.Helper()
	var err error
	f.toWorld, err = coordination.NewClientWithCA(f.world.endpoint.server.URL, strings.Repeat("project", 8), f.world.endpoint.caFile)
	wireOK(f.t, "TLS Project to World client", err)
	f.toProject, err = coordination.NewClientWithCA(f.project.endpoint.server.URL, strings.Repeat("world", 8), f.project.endpoint.caFile)
	wireOK(f.t, "TLS World to Project client", err)
	f.project.engine.Peers = map[string]coordination.Peer{"world": f.toWorld}
	f.world.engine.Peers = map[string]coordination.Peer{"project": f.toProject}
}

func (f *wirePair) operation() coordination.Operation {
	return coordination.Operation{Coordinator: "project", OrganizationID: f.org, SubjectID: f.subject, ResourceID: f.project.resource, CommitID: wireID(f.t), RequestHash: strings.Repeat("a", 64), Fence: 1}
}

func (f *wirePair) plan() coordination.Plan {
	return coordination.Plan{"project": {{ResourceID: f.project.resource, Write: true}}, "world": {{ResourceID: f.world.resource}}}
}

func (f *wirePair) count(n *wireNode, table string, o coordination.Operation) int {
	f.t.Helper()
	var count int
	err := n.db.QueryRow(f.ctx, "SELECT count(*) FROM studio."+n.domain+"_coord_"+table+" WHERE organization_id=$1 AND resource_id=$2 AND commit_id=$3", o.OrganizationID, o.ResourceID, o.CommitID).Scan(&count)
	wireOK(f.t, "read durable coordination count", err)
	return count
}

func (f *wirePair) state(n *wireNode, table string, o coordination.Operation) string {
	f.t.Helper()
	var state string
	err := n.db.QueryRow(f.ctx, "SELECT state FROM studio."+n.domain+"_coord_"+table+" WHERE organization_id=$1 AND resource_id=$2 AND commit_id=$3", o.OrganizationID, o.ResourceID, o.CommitID).Scan(&state)
	wireOK(f.t, "read durable coordination state", err)
	return state
}

func (f *wirePair) outbox() int {
	f.t.Helper()
	items, err := f.project.store.PendingOutbox(f.ctx)
	wireOK(f.t, "read pending outbox", err)
	return len(items)
}

// publish is deliberately a minimal SQL business substitute. Its unique result,
// CAS head, coordination terminal and outbox share exactly one real transaction.
func (f *wirePair) publish(o coordination.Operation, expected string) error {
	tx, err := f.project.db.Begin(f.ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(f.ctx, "INSERT INTO studio.fixture_publications VALUES($1,$2,$3)", o.CommitID, o.ResourceID, o.RequestHash); err != nil {
		return err
	}
	tag, err := tx.Exec(f.ctx, "UPDATE studio.fixture_heads SET head=$1 WHERE resource_id=$2 AND COALESCE(head::text,'')=$3", o.CommitID, o.ResourceID, expected)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return coordination.ErrConflict
	}
	if err = f.project.store.FinishTx(f.ctx, tx, o, "committed"); err != nil {
		return err
	}
	return tx.Commit(f.ctx)
}

func (f *wirePair) cancel(o coordination.Operation) {
	f.t.Helper()
	tx, err := f.project.db.Begin(f.ctx)
	wireOK(f.t, "begin cancellation", err)
	defer tx.Rollback(context.Background())
	wireOK(f.t, "durable cancellation", f.project.store.FinishTx(f.ctx, tx, o, "cancelled"))
	wireOK(f.t, "commit cancellation", tx.Commit(f.ctx))
}

func (f *wirePair) revokeWorld() bool {
	f.t.Helper()
	tx, err := f.world.db.Begin(f.ctx)
	wireOK(f.t, "begin fixture revoke", err)
	defer tx.Rollback(context.Background())
	pending, err := f.world.store.BlockTx(f.ctx, tx, f.org, f.world.resource, f.subject)
	wireOK(f.t, "install revocation barrier", err)
	if !pending {
		_, err = tx.Exec(f.ctx, "DELETE FROM studio.fixture_grants WHERE organization_id=$1 AND resource_id=$2 AND subject_id=$3", f.org, f.world.resource, f.subject)
		wireOK(f.t, "change fixture grant", err)
		wireOK(f.t, "complete fixture grant change", f.world.store.ChangedTx(f.ctx, tx, f.org, f.world.resource, f.subject))
	}
	wireOK(f.t, "commit fixture revoke state", tx.Commit(f.ctx))
	return pending
}

func (f *wirePair) assertGrantAndReservation(o coordination.Operation, granted bool, reservations int) {
	f.t.Helper()
	var exists bool
	err := f.world.db.QueryRow(f.ctx, "SELECT EXISTS(SELECT 1 FROM studio.fixture_grants WHERE organization_id=$1 AND resource_id=$2 AND subject_id=$3)", f.org, f.world.resource, f.subject).Scan(&exists)
	wireOK(f.t, "read fixture grant", err)
	if exists != granted || f.count(f.world, "reservations", o) != reservations {
		f.t.Fatal("permission or durable reservation changed without verified terminal")
	}
}

func (f *wirePair) assertPublication(head string, count int) {
	f.t.Helper()
	var actual string
	var publications int
	err := f.project.db.QueryRow(f.ctx, "SELECT COALESCE(head::text,'') FROM studio.fixture_heads WHERE resource_id=$1", f.project.resource).Scan(&actual)
	wireOK(f.t, "read business fixture head", err)
	err = f.project.db.QueryRow(f.ctx, "SELECT count(*) FROM studio.fixture_publications").Scan(&publications)
	wireOK(f.t, "read business fixture results", err)
	if actual != head || publications != count {
		f.t.Fatal("business publication was repeated or escaped its terminal transaction")
	}
}

func (f *wirePair) rebuild(nodes ...*wireNode) {
	f.t.Helper()
	for _, n := range nodes {
		address := n.endpoint.server.Listener.Addr().String()
		n.endpoint.server.Close()
		config := n.db.Config().Copy()
		n.db.Close()
		db, err := pgxpool.NewWithConfig(f.ctx, config)
		wireOK(f.t, "reconnect persisted participant database", err)
		f.t.Cleanup(db.Close)
		n.db = db
		f.makeEngine(n, 1)
		n.endpoint = newWireEndpoint(f.t, f.handler(n), address)
	}
	f.connect()
}

func TestCoordinationNetworkPrepareLostResponseRetry(t *testing.T) {
	f := newWirePair(t)
	o, plan := f.operation(), f.plan()
	f.world.endpoint.dropNext("prepare")
	if err := f.project.engine.Prepare(f.ctx, o, plan); !errors.Is(err, coordination.ErrUnknown) {
		t.Fatal("lost prepare response did not remain unknown")
	}
	f.world.endpoint.assertSuccessfulDrop(t)
	if f.count(f.world, "participants", o) != 1 || f.count(f.world, "reservations", o) != 1 || f.state(f.project, "operations", o) != "preparing" {
		t.Fatal("successful remote prepare was not durable after lost response")
	}
	var recorded bool
	err := f.project.db.QueryRow(f.ctx, "SELECT prepared ? 'world' FROM studio.project_coord_operations WHERE commit_id=$1", o.CommitID).Scan(&recorded)
	wireOK(t, "read unacknowledged participant", err)
	if recorded {
		t.Fatal("lost response was treated as preparation acknowledgement")
	}
	wireOK(t, "retry exact operation over TLS", f.project.engine.Prepare(f.ctx, o, plan))
	if f.world.endpoint.count("prepare") != 2 || f.count(f.world, "participants", o) != 1 || f.count(f.world, "reservations", o) != 1 {
		t.Fatal("same-operation retry duplicated participant state")
	}
	changed := o
	changed.RequestHash = strings.Repeat("b", 64)
	if err = f.toWorld.Prepare(f.ctx, changed, plan["world"]); !errors.Is(err, coordination.ErrConflict) {
		t.Fatal("same ID with different hash crossed the network boundary")
	}
	wireOK(t, "publish original operation", f.publish(o, ""))
	wireOK(t, "deliver original terminal", f.project.engine.Deliver(f.ctx))
	f.assertGrantAndReservation(o, true, 0)
	f.assertPublication(o.CommitID, 1)
}

func TestCoordinationNetworkOutageKeepsRevocationPending(t *testing.T) {
	f := newWirePair(t)
	o, plan := f.operation(), f.plan()
	wireOK(t, "prepare before outage", f.project.engine.Prepare(f.ctx, o, plan))
	f.project.endpoint.server.Close()
	if !f.revokeWorld() {
		t.Fatal("revocation completed while durable reservation was active")
	}
	if err := f.toWorld.Finalize(f.ctx, o); !errors.Is(err, coordination.ErrUnknown) {
		t.Fatal("unreachable coordinator was treated as terminal")
	}
	f.assertGrantAndReservation(o, true, 1)
	newOperation := f.operation()
	if err := f.toWorld.Prepare(f.ctx, newOperation, plan["world"]); !errors.Is(err, coordination.ErrDenied) {
		t.Fatal("pending revoke admitted a new operation")
	}
	// Rebuild the participant from its own database while the coordinator stays
	// down. Its persisted barrier and reservation must survive without any cache.
	f.rebuild(f.world)
	if !f.revokeWorld() {
		t.Fatal("participant reconstruction forgot pending revoke")
	}
	if err := f.toWorld.Finalize(f.ctx, o); !errors.Is(err, coordination.ErrUnknown) {
		t.Fatal("participant reconstruction invented coordinator terminal")
	}
	f.assertGrantAndReservation(o, true, 1)
	f.rebuild(f.project)
	f.cancel(o)
	wireOK(t, "deliver restored coordinator cancellation", f.project.engine.Deliver(f.ctx))
	if f.revokeWorld() {
		t.Fatal("verified terminal did not allow explicit revoke retry")
	}
	f.assertGrantAndReservation(o, false, 0)
	if err := f.toWorld.Prepare(f.ctx, o, plan["world"]); !errors.Is(err, coordination.ErrDenied) {
		t.Fatal("cancelled operation resurrected after recovery")
	}
	f.assertPublication("", 0)
}

func TestCoordinationNetworkRestartRecoversTerminalAndOutbox(t *testing.T) {
	f := newWirePair(t)
	o := f.operation()
	wireOK(t, "prepare publication", f.project.engine.Prepare(f.ctx, o, f.plan()))
	wireOK(t, "publish terminal and outbox atomically", f.publish(o, ""))
	f.world.endpoint.server.Close()
	if err := f.project.engine.Deliver(f.ctx); !errors.Is(err, coordination.ErrUnknown) || f.outbox() != 1 {
		t.Fatal("unavailable participant lost its pending notification")
	}
	f.assertGrantAndReservation(o, true, 1)
	f.rebuild(f.project, f.world)
	proof, err := f.toProject.State(f.ctx, o)
	wireOK(t, "query reconstructed coordinator over TLS", err)
	if proof.Operation != o || proof.State != "committed" || f.outbox() != 1 {
		t.Fatal("reconstruction changed original terminal identity or outbox")
	}
	f.world.endpoint.dropNext("finalize")
	if err = f.project.engine.Deliver(f.ctx); !errors.Is(err, coordination.ErrUnknown) {
		t.Fatal("lost finalize response acknowledged outbox")
	}
	f.world.endpoint.assertSuccessfulDrop(t)
	if f.outbox() != 1 || f.state(f.world, "participants", o) != "committed" {
		t.Fatal("lost response changed durable terminal or removed undelivered outbox")
	}
	f.assertGrantAndReservation(o, true, 0)
	wireOK(t, "redeliver exact original terminal", f.project.engine.Deliver(f.ctx))
	wireOK(t, "repeat empty recovery pass", f.project.engine.Deliver(f.ctx))
	if f.outbox() != 0 || f.world.endpoint.count("finalize") != 2 || f.project.endpoint.count("state") < 3 {
		t.Fatal("recovery did not verify terminal over the restored network")
	}
	f.assertPublication(o.CommitID, 1)
}

func TestCoordinationNetworkFenceAdvanceRejectsOldTerminal(t *testing.T) {
	f := newWirePair(t)
	committed, preparing := f.operation(), f.operation()
	wireOK(t, "prepare old committed operation", f.project.engine.Prepare(f.ctx, committed, f.plan()))
	wireOK(t, "commit before external epoch change", f.publish(committed, ""))
	wireOK(t, "prepare second old operation", f.project.engine.Prepare(f.ctx, preparing, f.plan()))
	for _, n := range []*wireNode{f.project, f.world} {
		// Explicit operator-side fixture action, never a request-driven epoch bump.
		_, err := n.db.Exec(f.ctx, "UPDATE studio."+n.domain+"_coord_fences SET fence=2 WHERE coordinator='project' AND fence=1")
		wireOK(t, "advance external fixture fence", err)
	}
	if err := f.publish(preparing, committed.CommitID); !errors.Is(err, coordination.ErrFenced) {
		t.Fatal("old worker published after external epoch advance")
	}
	f.assertPublication(committed.CommitID, 1)
	if err := f.toWorld.Finalize(f.ctx, committed); !errors.Is(err, coordination.ErrFenced) {
		t.Fatal("old coordinator terminal released reservation after epoch advance")
	}
	f.assertGrantAndReservation(committed, true, 1)
	f.assertGrantAndReservation(preparing, true, 1)
	if f.state(f.project, "operations", preparing) != "preparing" {
		t.Fatal("fenced transaction rewrote old durable state")
	}
	f.makeEngine(f.project, 2)
	f.project.endpoint.replace(f.handler(f.project))
	f.connect()
	if _, err := f.toProject.State(f.ctx, committed); !errors.Is(err, coordination.ErrFenced) {
		t.Fatal("new coordinator instance emitted old-epoch terminal proof")
	}
	if err := f.toWorld.Prepare(f.ctx, preparing, f.plan()["world"]); !errors.Is(err, coordination.ErrFenced) {
		t.Fatal("participant accepted old-epoch prepare after recovery")
	}
	retagged := preparing
	retagged.Fence = 2
	if err := f.project.engine.Prepare(f.ctx, retagged, f.plan()); !errors.Is(err, coordination.ErrConflict) {
		t.Fatal("new worker adopted old unknown operation by rewriting its epoch")
	}
	if !f.revokeWorld() {
		t.Fatal("fenced unknown reservations disappeared")
	}
}
