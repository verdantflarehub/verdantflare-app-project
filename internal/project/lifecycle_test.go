package project

import (
	"context"
	"errors"
	"testing"
)

func TestLifecycleRejectsIncompleteCASBeforeIO(t *testing.T) {
	p := Principal{OrganizationID: id(1), SubjectID: id(2), RequestID: id(3)}
	zero := int64(0)
	negative := int64(-1)
	base := LifecycleRequest{ProjectID: id(4), ChangeID: id(5), ExpectedRevisionID: id(6), ExpectedLifecycleVersion: &zero}
	cases := []LifecycleRequest{base, base, base, base}
	cases[0].ExpectedLifecycleVersion = nil
	cases[1].ExpectedLifecycleVersion = &negative
	cases[2].ChangeID = "invalid"
	cases[3].ExpectedRevisionID = ""
	s := &Service{}
	for _, r := range cases {
		if _, err := s.ChangeLifecycle(context.Background(), p, r, true); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected invalid request: %v", err)
		}
	}
}
