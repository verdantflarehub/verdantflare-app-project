package project

import (
	"testing"
	"time"
)

func TestCrewClockPausesAndExhausts(t *testing.T) {
	start := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	st := CrewState{Phase: "working", Started: start, RemainingMS: 4500}
	if st.remaining(start.Add(time.Second)) != 3500 || st.remaining(start.Add(time.Minute)) != 0 {
		t.Fatal("execution clock")
	}
	st.RemainingMS = st.remaining(start.Add(time.Second))
	st.Phase = "waiting_human"
	if st.remaining(start.Add(24*time.Hour)) != 3500 {
		t.Fatal("human wait consumed execution time")
	}
	st.Phase = "working"
	st.Started = start.Add(24 * time.Hour)
	if st.remaining(st.Started.Add(2*time.Second)) != 1500 {
		t.Fatal("restart reset budget")
	}
}
func TestCrewSubmissionAndVersionLimits(t *testing.T) {
	file := CrewFile{FileID: id(2), VersionID: id(3)}
	if (CrewSubmission{Kind: "opinion", File: &file}).valid("submit") {
		t.Fatal("opinion carried candidate file")
	}
	if !(CrewSubmission{Kind: "result", Run: id(4), File: &file}).valid("recover") {
		t.Fatal("late callback fields rejected")
	}
	m := Manifest{Files: []File{{ID: file.FileID, Content: ContentRef{VersionID: id(4)}}}}
	if crewRefs([]CrewFile{file}, m) {
		t.Fatal("replaced input matched")
	}
	m.Files[0].Content.VersionID = file.VersionID
	if !crewRefs([]CrewFile{file}, m) {
		t.Fatal("pinned input missing")
	}
}
