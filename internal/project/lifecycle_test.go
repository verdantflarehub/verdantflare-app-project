package project

import (
	"context"
	"errors"
	"github.com/verdantflarehub/verdantflare-app-project/internal/strictjson"
	"strings"
	"testing"
)

func TestDeletedListFilterAcceptsExplicitFalse(t *testing.T) {
	for _, body := range []string{`{"category":"video","limit":100}`, `{"category":"video","limit":100,"deleted":false}`, `{"category":"video","limit":100,"deleted":true}`} {
		var request ListRequest
		if err := strictjson.Decode(strings.NewReader(body), 1024, &request); err != nil {
			t.Fatalf("valid list scope rejected: %v", err)
		}
		expected := strings.Contains(body, `"deleted":true`)
		if got := request.Deleted != nil && *request.Deleted; got != expected {
			t.Fatal("list scope changed")
		}
	}
}

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
