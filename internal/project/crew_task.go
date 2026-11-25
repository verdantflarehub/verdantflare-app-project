package project

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/verdantflarehub/verdantflare-app-project/internal/coordination"
)

type CrewFile struct {
	FileID    string `json:"file_id"`
	VersionID string `json:"version_id"`
	Purpose   string `json:"purpose,omitempty"`
}
type CrewAssignment struct {
	LeaseSHA  string     `json:"lease_sha256"`
	Worker    string     `json:"worker_id"`
	Collector string     `json:"collector_id"`
	Revision  string     `json:"input_revision_id"`
	Kind      string     `json:"kind"`
	Brief     string     `json:"brief"`
	Inputs    []CrewFile `json:"inputs"`
	Human     bool       `json:"human_required"`
	Budget    int64      `json:"execution_seconds"`
}
type CrewRequest struct {
	LeaseToken string          `json:"lease_token,omitempty"`
	ProjectID  string          `json:"project_id"`
	TaskID     string          `json:"task_id"`
	Action     string          `json:"action"`
	Event      string          `json:"event_id,omitempty"`
	Generation string          `json:"generation,omitempty"`
	Assignment *CrewAssignment `json:"assignment,omitempty"`
	Submission *CrewSubmission `json:"submission,omitempty"`
}
type CrewSubmission struct {
	Kind      string     `json:"kind"`
	Note      string     `json:"note,omitempty"`
	File      *CrewFile  `json:"file,omitempty"`
	Inputs    []CrewFile `json:"inputs,omitempty"`
	Candidate string     `json:"candidate_event_id,omitempty"`
	Run       string     `json:"run_id,omitempty"`
	Timeout   int64      `json:"timeout_seconds,omitempty"`
	Verdict   string     `json:"verdict,omitempty"`
}
type CrewState struct {
	Assignment  CrewAssignment `json:"assignment"`
	Generation  string         `json:"generation"`
	Phase       string         `json:"phase"`
	Started     time.Time      `json:"started_at,omitempty"`
	RemainingMS int64          `json:"remaining_ms"`
	Candidate   string         `json:"candidate_event_id,omitempty"`
	QC          string         `json:"qc_event_id,omitempty"`
	Selected    string         `json:"selected_event_id,omitempty"`
}
type crewRun struct {
	Collector string    `json:"collector_id"`
	Deadline  time.Time `json:"deadline"`
}
type crewReceipt struct {
	Generation string   `json:"generation"`
	Event      string   `json:"event_id,omitempty"`
	Phase      string   `json:"phase"`
	Run        *crewRun `json:"run,omitempty"`
}

func (st CrewState) remaining(now time.Time) int64 {
	if st.Phase == "working" {
		return max(0, st.RemainingMS-max(0, now.Sub(st.Started).Milliseconds()))
	}
	return st.RemainingMS
}
func crewRefs(files []CrewFile, m Manifest) bool {
	versions := make(map[string]string, len(m.Files))
	for _, f := range m.Files {
		versions[f.ID] = f.Content.VersionID
	}
	for _, f := range files {
		if !ValidID(f.FileID) || !ValidID(f.VersionID) || versions[f.FileID] != f.VersionID {
			return false
		}
	}
	return true
}
func crewMember(ctx context.Context, tx pgx.Tx, p Principal, project, subject string) (string, error) {
	var value string
	err := tx.QueryRow(ctx, "SELECT role FROM studio.project_members WHERE organization_id=$1 AND project_id=$2 AND subject_id=$3 FOR SHARE", p.OrganizationID, project, subject).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	return value, err
}
func crewDirector(role string) bool { return role == "editor" || role == "owner" }
func (s *Service) CrewTask(ctx context.Context, p Principal, r CrewRequest) (any, error) {
	if (r.Action == "dispatch" || r.Action == "reassign") != (r.Assignment != nil) || (r.Action == "submit" || r.Action == "select" || r.Action == "recover") != (r.Submission != nil) {
		return nil, ErrInvalid
	}
	if r.Submission != nil && !r.Submission.valid(r.Action) {
		return nil, ErrInvalid
	}
	if !p.Valid() || !ValidID(r.ProjectID) || !ValidID(r.TaskID) || r.Action != "status" && !ValidID(r.Event) {
		return nil, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	var head string
	err = tx.QueryRow(ctx, "SELECT head_revision_id::text FROM studio.projects WHERE organization_id=$1 AND project_id=$2 AND NOT deleted FOR SHARE", p.OrganizationID, r.ProjectID).Scan(&head)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrForbidden
	}
	if err != nil {
		return nil, err
	}
	if err = advisory(ctx, tx, "crew:"+p.OrganizationID+":"+r.ProjectID+":"+r.TaskID); err != nil {
		return nil, err
	}
	role, err := crewMember(ctx, tx, p, r.ProjectID, p.SubjectID)
	if err != nil {
		return nil, err
	}
	key := []any{p.OrganizationID, r.ProjectID, r.TaskID}
	const scope = "organization_id=$1 AND project_id=$2 AND task_id=$3"
	var st CrewState
	err = tx.QueryRow(ctx, "SELECT state FROM studio.project_crew_tasks WHERE "+scope, key...).Scan(&st)
	fresh := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !fresh {
		return nil, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return nil, err
	}
	workerOperation := r.Action == "claim" || r.Action == "submit" || r.Action == "pause" || r.Action == "status" && !crewDirector(role)
	if workerOperation {
		if fresh || role != "reader" || p.SubjectID != st.Assignment.Worker || !crewLeaseValid(r.LeaseToken, st.Assignment.LeaseSHA) {
			return nil, ErrForbidden
		}
	} else if r.LeaseToken != "" {
		return nil, ErrInvalid
	}
	r.LeaseToken = "" // Never persist credentials or echo them in receipts.
	if r.Action == "status" {
		if fresh || !crewDirector(role) && (role != "reader" || p.SubjectID != st.Assignment.Worker) {
			return nil, ErrForbidden
		}
		events := []json.RawMessage{}
		rows, e := tx.Query(ctx, "SELECT jsonb_build_object('event_id',event_id,'generation',generation,'action',action,'submission',payload->'submission','receipt',receipt,'created_at',created_at) FROM studio.project_crew_events WHERE "+scope+" AND ($4 OR generation=$5) ORDER BY created_at,event_id", append(key, crewDirector(role), st.Generation)...)
		if e != nil {
			return nil, e
		}
		defer rows.Close()
		for rows.Next() {
			var b json.RawMessage
			if e = rows.Scan(&b); e != nil {
				return nil, e
			}
			events = append(events, b)
		}
		if e = rows.Err(); e != nil {
			return nil, e
		}
		return map[string]any{"state": st, "remaining_ms": st.remaining(now), "timed_out": st.Phase == "working" && st.remaining(now) == 0, "events": events}, nil
	}
	hash := coordination.Fingerprint(r)
	var prev crewReceipt
	var actor, digest string
	err = tx.QueryRow(ctx, "SELECT actor::text,request_hash,receipt FROM studio.project_crew_events WHERE "+scope+" AND event_id=$4", append(key, r.Event)...).Scan(&actor, &digest, &prev)
	if err == nil {
		if actor != p.SubjectID || digest != hash {
			return nil, ErrIdempotency
		}
		return prev, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if fresh && r.Action != "dispatch" {
		return nil, ErrForbidden
	}
	if r.Action != "recover" && r.Generation != st.Generation {
		return nil, ErrConflict
	}
	result := crewReceipt{Event: r.Event}
	switch r.Action {
	case "dispatch", "reassign":
		if !crewDirector(role) {
			return nil, ErrForbidden
		}
		if fresh != (r.Action == "dispatch") {
			return nil, ErrConflict
		}
		a := r.Assignment
		if a == nil || !validCrewDigest(a.LeaseSHA) || r.Submission != nil || !ValidID(a.Worker) || !ValidID(a.Collector) || a.Collector == a.Worker || a.Revision != head || a.Kind != "crew" && a.Kind != "craft" || a.Brief == "" || len(a.Brief) > 16000 || a.Budget < 1 || a.Budget > 604800 || len(a.Inputs) > 128 {
			return nil, ErrInvalid
		}
		if !fresh {
			var reused bool
			if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM studio.project_crew_events WHERE "+scope+" AND action IN ('dispatch','reassign') AND payload->'assignment'->>'lease_sha256'=$4)", append(key, a.LeaseSHA)...).Scan(&reused); err != nil {
				return nil, err
			}
			if reused || st.Assignment.Human && !a.Human {
				return nil, ErrConflict
			}
		}
		for _, subject := range []string{a.Worker, a.Collector} {
			rr, e := crewMember(ctx, tx, p, r.ProjectID, subject)
			if e != nil {
				return nil, e
			}
			if rr != "reader" {
				return nil, ErrForbidden
			}
		}
		seen := map[string]bool{}
		for _, f := range a.Inputs {
			if f.Purpose == "" || seen[f.FileID] {
				return nil, ErrInvalid
			}
			seen[f.FileID] = true
		}
		st = CrewState{Assignment: *a, Generation: newID(), Phase: "assigned", RemainingMS: a.Budget * 1000}
	case "cancel", "resume", "select":
		if !crewDirector(role) {
			return nil, ErrForbidden
		}
		switch r.Action {
		case "cancel":
			st.Generation = newID()
			st.Phase = "cancelled"
			st.Candidate = ""
			st.QC = ""
			st.Selected = ""
		case "resume":
			if st.Phase != "waiting_human" {
				return nil, ErrConflict
			}
			st.Started = now
			st.Phase = "working"
		case "select":
			if (st.Phase != "working" && st.Phase != "waiting_human") || st.QC == "" || r.Submission == nil || r.Submission.Candidate != st.Candidate {
				return nil, ErrConflict
			}
			if st.Assignment.Human && p.ReviewOrigin != "human_interactive" {
				return nil, ErrForbidden
			}
			var source CrewRequest
			if err = tx.QueryRow(ctx, "SELECT payload FROM studio.project_crew_events WHERE "+scope+" AND generation=$4 AND event_id=$5", append(key, st.Generation, st.Candidate)...).Scan(&source); err != nil {
				return nil, err
			}
			m, _, e := revision(ctx, tx, p.OrganizationID, r.ProjectID, head)
			if e != nil {
				return nil, e
			}
			if source.Submission == nil || source.Submission.File == nil || !crewRefs([]CrewFile{*source.Submission.File}, m) {
				return nil, ErrConflict
			}
			st.Selected = st.Candidate
			st.RemainingMS = st.remaining(now)
			st.Phase = "selected"
		}
	case "claim", "submit", "pause":
		if role != "reader" || p.SubjectID != st.Assignment.Worker {
			return nil, ErrForbidden
		}
		switch r.Action {
		case "claim":
			if st.Phase != "assigned" {
				return nil, ErrConflict
			}
			st.Started = now
			st.Phase = "working"
		default:
			if st.Phase != "working" || st.remaining(now) == 0 {
				return nil, ErrConflict
			}
			if r.Action == "pause" {
				st.RemainingMS = st.remaining(now)
				st.Phase = "waiting_human"
				break
			}
			x := r.Submission
			if x == nil || len(x.Note) > 16000 {
				return nil, ErrInvalid
			}
			switch x.Kind {
			case "progress":
				if x.Note == "" {
					return nil, ErrInvalid
				}
			case "opinion":
				if st.Assignment.Kind != "craft" || x.Note == "" {
					return nil, ErrForbidden
				}
			case "candidate":
				if x.Run == "" {
					var hasRun bool
					if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM studio.project_crew_events WHERE "+scope+" AND generation=$4 AND receipt ? 'run')", append(key, st.Generation)...).Scan(&hasRun); err != nil {
						return nil, err
					}
					if hasRun {
						return nil, ErrInvalid
					}
				}
				if x.Run != "" {
					if !ValidID(x.Run) {
						return nil, ErrInvalid
					}
					var run crewRun
					err = tx.QueryRow(ctx, "SELECT receipt->'run' FROM studio.project_crew_events WHERE "+scope+" AND generation=$4 AND receipt ? 'run' AND payload->'submission'->>'run_id'=$5", append(key, st.Generation, x.Run)...).Scan(&run)
					if errors.Is(err, pgx.ErrNoRows) {
						return nil, ErrForbidden
					}
					if err != nil {
						return nil, err
					}
					if !now.Before(run.Deadline) {
						return nil, ErrConflict
					}
				}
				if st.Assignment.Kind != "crew" || x.File == nil {
					return nil, ErrForbidden
				}
				if coordination.Fingerprint(x.Inputs) != coordination.Fingerprint(st.Assignment.Inputs) {
					return nil, ErrConflict
				}
				m, _, e := revision(ctx, tx, p.OrganizationID, r.ProjectID, head)
				if e != nil {
					return nil, e
				}
				if !crewRefs([]CrewFile{*x.File}, m) {
					return nil, ErrConflict
				}
				st.Candidate = r.Event
				st.QC = ""
			case "qc":
				if st.Candidate == "" || x.Candidate != st.Candidate || x.Note == "" || x.Verdict != "pass" && x.Verdict != "revise" {
					return nil, ErrInvalid
				}
				st.QC = ""
				if x.Verdict == "pass" {
					st.QC = r.Event
				}
			case "run":
				if !ValidID(x.Run) || x.Timeout < 1 || x.Timeout > 604800 {
					return nil, ErrInvalid
				}
				var exists bool
				if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM studio.project_crew_events WHERE "+scope+" AND generation=$4 AND receipt ? 'run' AND payload->'submission'->>'run_id'=$5)", append(key, st.Generation, x.Run)...).Scan(&exists); err != nil {
					return nil, err
				}
				if exists {
					return nil, ErrConflict
				}
				result.Run = &crewRun{Collector: st.Assignment.Collector, Deadline: now.Add(time.Duration(x.Timeout) * time.Second)}
			default:
				return nil, ErrInvalid
			}
		}
	case "recover":
		x := r.Submission
		if role != "reader" || !ValidID(r.Generation) || x == nil || !ValidID(x.Run) || x.File == nil {
			return nil, ErrForbidden
		}
		var run crewRun
		err = tx.QueryRow(ctx, "SELECT receipt->'run' FROM studio.project_crew_events WHERE "+scope+" AND generation=$4 AND receipt ? 'run' AND payload->'submission'->>'run_id'=$5", append(key, r.Generation, x.Run)...).Scan(&run)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrForbidden
		}
		if err != nil {
			return nil, err
		}
		if run.Collector != p.SubjectID {
			return nil, ErrForbidden
		}
		if r.Generation == st.Generation && st.Phase != "cancelled" && st.remaining(now) > 0 && now.Before(run.Deadline) {
			return nil, ErrConflict
		}
		m, _, e := revision(ctx, tx, p.OrganizationID, r.ProjectID, head)
		if e != nil {
			return nil, e
		}
		if !crewRefs([]CrewFile{*x.File}, m) {
			return nil, ErrInvalid
		}
		result.Generation = r.Generation
		result.Phase = "recovered"
	default:
		return nil, ErrInvalid
	}
	if r.Action != "recover" {
		if r.Action != "cancel" {
			m, _, e := revision(ctx, tx, p.OrganizationID, r.ProjectID, head)
			if e != nil {
				return nil, e
			}
			if !crewRefs(st.Assignment.Inputs, m) {
				return nil, ErrConflict
			}
		}
		result.Generation = st.Generation
		result.Phase = st.Phase
		_, err = tx.Exec(ctx, "INSERT INTO studio.project_crew_tasks VALUES($1,$2,$3,$4) ON CONFLICT(organization_id,project_id,task_id) DO UPDATE SET state=excluded.state", append(key, st)...)
		if err != nil {
			return nil, err
		}
	}
	_, err = tx.Exec(ctx, "INSERT INTO studio.project_crew_events(organization_id,project_id,task_id,event_id,generation,actor,action,request_hash,payload,receipt) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)", append(key, r.Event, result.Generation, p.SubjectID, r.Action, hash, r, result)...)
	if err != nil {
		return nil, err
	}
	return result, tx.Commit(ctx)
}

func (x CrewSubmission) valid(action string) bool {
	if len(x.Note) > 16000 {
		return false
	}
	// Consume only fields belonging to this operation. Reject unconsumed data
	// rather than retaining apparent candidates or approvals as event metadata.
	y := x
	switch action {
	case "recover":
		if x.Kind != "result" {
			return false
		}
		y.Run = ""
		y.File = nil
		y.Note = ""
	case "select":
		if x.Kind != "selection" {
			return false
		}
		y.Candidate = ""
		y.Note = ""
	case "submit":
		switch x.Kind {
		case "progress", "opinion":
			y.Note = ""
		case "run":
			y.Run = ""
			y.Timeout = 0
		case "candidate":
			y.File = nil
			y.Inputs = nil
			y.Run = ""
		case "qc":
			y.Note = ""
			y.Candidate = ""
			y.Verdict = ""
		default:
			return false
		}
	default:
		return false
	}
	return y.Note == "" && y.File == nil && len(y.Inputs) == 0 && y.Candidate == "" && y.Run == "" && y.Timeout == 0 && y.Verdict == ""
}

func validCrewDigest(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size && hex.EncodeToString(b) == s
}
func crewLeaseValid(token, digest string) bool {
	if !validCrewDigest(token) {
		return false
	}
	sum := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(digest)) == 1
}
