//go:build integration

package project

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/verdantflarehub/verdantflare-app-project/internal/coordination"
	"github.com/verdantflarehub/verdantflare-app-project/internal/testdb"
	"github.com/verdantflarehub/verdantflare-app-project/migrations"
	"strings"
	"testing"
	"time"
)

func TestCrewGenerationRecoveryAndSelection(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if err := migrations.ApplyFresh(ctx, db, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := migrations.ActivateFresh(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := migrations.CheckCrewTask(ctx, db); err != nil {
		t.Fatal(err)
	}
	store, err := coordination.NewStore(db, "project", 1, AuthorizeReservation)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(ctx, db, &integrationArtifact{versions: map[ContentRef]ContentVersion{}, bytes: map[ContentRef][]byte{}}, integrationAssets{}, &coordination.Engine{Store: store, Peers: map[string]coordination.Peer{"world": &integrationPeer{}}})
	if err != nil {
		t.Fatal(err)
	}
	owner := Principal{OrganizationID: id(30), SubjectID: id(31), RequestID: newID()}
	created, err := svc.Create(ctx, owner, CreateRequest{CommitID: newID(), Name: "Crew test", Category: "image", EntryPath: "brief.md", EntryText: "# Brief"})
	if err != nil {
		t.Fatal(err)
	}
	var projectID string
	if err = db.QueryRow(ctx, "SELECT project_id::text FROM studio.project_revisions WHERE revision_id=$1", created.RevisionID).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	worker := owner
	worker.SubjectID = newID()
	replacement := owner
	replacement.SubjectID = newID()
	collector := owner
	collector.SubjectID = newID()
	for _, p := range []Principal{worker, replacement, collector} {
		if _, err = svc.ChangeMember(ctx, owner, MemberChangeRequest{ProjectID: projectID, SubjectID: p.SubjectID, ChangeID: newID(), Role: "reader"}); err != nil {
			t.Fatal(err)
		}
	}
	m, _, err := revision(ctx, db, owner.OrganizationID, projectID, created.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	file := CrewFile{FileID: m.Files[0].ID, VersionID: m.Files[0].Content.VersionID, Purpose: "baseline"}
	token := strings.Repeat("1", 64)
	nextToken := strings.Repeat("2", 64)
	assignment := CrewAssignment{LeaseSHA: fmt.Sprintf("%x", sha256.Sum256([]byte(token))), Worker: worker.SubjectID, Collector: collector.SubjectID, Revision: created.RevisionID, Kind: "crew", Brief: "Keep character identity", Inputs: []CrewFile{file}, Human: true, Budget: 60}
	base := CrewRequest{ProjectID: projectID, TaskID: newID()}
	req := func(action string) CrewRequest { r := base; r.Action = action; r.Event = newID(); return r }
	call := func(p Principal, r CrewRequest, want error) crewReceipt {
		t.Helper()
		if r.Action == "claim" || r.Action == "submit" || r.Action == "pause" {
			if p.SubjectID == worker.SubjectID {
				r.LeaseToken = token
			} else {
				r.LeaseToken = nextToken
			}
		}
		res, e := svc.CrewTask(ctx, p, r)
		if !errors.Is(e, want) {
			t.Fatalf("%s got %v want %v", r.Action, e, want)
		}
		if want != nil {
			return crewReceipt{}
		}
		return res.(crewReceipt)
	}
	r := req("dispatch")
	r.Assignment = &assignment
	first := call(owner, r, nil)
	base.Generation = first.Generation
	call(worker, req("claim"), nil)
	run := req("submit")
	run.Submission = &CrewSubmission{Kind: "run", Run: newID(), Timeout: 60}
	call(worker, run, nil)
	assignment.Worker = replacement.SubjectID
	assignment.LeaseSHA = fmt.Sprintf("%x", sha256.Sum256([]byte(nextToken)))
	r = req("reassign")
	r.Assignment = &assignment
	next := call(owner, r, nil)
	stale := req("submit")
	stale.Submission = &CrewSubmission{Kind: "progress", Note: "old run done"}
	call(worker, stale, ErrForbidden)
	late := req("recover")
	late.Submission = &CrewSubmission{Kind: "result", Run: run.Submission.Run, File: &file}
	call(worker, late, ErrForbidden)
	call(collector, late, nil)
	call(collector, late, nil)
	base.Generation = next.Generation
	call(worker, req("claim"), ErrForbidden)
	claim := req("claim")
	claim.LeaseToken = nextToken
	concurrent := make(chan error, 2)
	for i := 0; i < 2; i++ {
		competing := claim
		competing.Event = newID()
		go func() { _, e := svc.CrewTask(ctx, replacement, competing); concurrent <- e }()
	}
	accepted, conflicted := 0, 0
	for i := 0; i < 2; i++ {
		e := <-concurrent
		if e == nil {
			accepted++
		} else if errors.Is(e, ErrConflict) {
			conflicted++
		} else {
			t.Fatal(e)
		}
	}
	if accepted != 1 || conflicted != 1 {
		t.Fatalf("concurrent claims accepted=%d conflicts=%d", accepted, conflicted)
	}
	wrongLease := req("status")
	wrongLease.LeaseToken = token
	if _, e := svc.CrewTask(ctx, replacement, wrongLease); !errors.Is(e, ErrForbidden) {
		t.Fatal("previous lease accepted under current subject", e)
	}

	call(replacement, req("pause"), nil)
	call(owner, req("resume"), nil)
	candidate := req("submit")
	candidate.Submission = &CrewSubmission{Kind: "candidate", File: &file, Inputs: assignment.Inputs}
	fastRun := req("submit")
	fastRun.Submission = &CrewSubmission{Kind: "run", Run: newID(), Timeout: 1}
	call(replacement, fastRun, nil)
	call(replacement, candidate, ErrInvalid)
	time.Sleep(1100 * time.Millisecond)
	candidate.Submission.Run = fastRun.Submission.Run
	call(replacement, candidate, ErrConflict)
	timedOut := req("recover")
	timedOut.Submission = &CrewSubmission{Kind: "result", Run: fastRun.Submission.Run, File: &file}
	call(collector, timedOut, nil)
	newRun := req("submit")
	newRun.Submission = &CrewSubmission{Kind: "run", Run: newID(), Timeout: 60}
	call(replacement, newRun, nil)
	candidate.Submission.Run = newRun.Submission.Run
	call(replacement, candidate, nil)
	selectReq := req("select")
	selectReq.Submission = &CrewSubmission{Kind: "selection", Candidate: candidate.Event}
	call(owner, selectReq, ErrConflict)
	qc := req("submit")
	qc.Submission = &CrewSubmission{Kind: "qc", Candidate: candidate.Event, Verdict: "pass", Note: "same identity observed"}
	call(replacement, qc, nil)
	call(owner, selectReq, ErrForbidden)
	owner.ReviewOrigin = "human_interactive"
	call(owner, selectReq, nil)
	var count int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM studio.project_crew_events WHERE task_id=$1 AND action='recover'", base.TaskID).Scan(&count); err != nil || count != 2 {
		t.Fatal("callback count", count, err)
	}
}
