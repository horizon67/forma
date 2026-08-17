package web

import (
	"errors"
	"testing"
	"time"

	"example.com/forma-admin-target/internal/clock"
)

func testGuard() (*anonymousSubmissionGuard, *clock.Fixed) {
	fixed := clock.NewFixed(time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC))
	return newAnonymousSubmissionGuard(fixed), fixed
}

func TestGuardHoldsTheCapAfterInsert(t *testing.T) {
	guard, _ := testGuard()
	for index := 0; index < maxOutstandingAnonymousSubmissions+1; index++ {
		if _, err := guard.issue(); err != nil {
			t.Fatalf("issue %d: %v", index, err)
		}
		if got := guard.outstanding(); got > maxOutstandingAnonymousSubmissions {
			t.Fatalf("after issue %d outstanding = %d, want at most %d",
				index, got, maxOutstandingAnonymousSubmissions)
		}
	}
}

func TestGuardKeepsRunningTokensAndRefusesWhenFull(t *testing.T) {
	guard, _ := testGuard()
	var running []string
	for index := 0; index < maxOutstandingAnonymousSubmissions; index++ {
		token, err := guard.issue()
		if err != nil {
			t.Fatal(err)
		}
		if state, _ := guard.begin(token); state != anonymousAccepted {
			t.Fatalf("token %d was not accepted", index)
		}
		running = append(running, token)
	}
	if _, err := guard.issue(); !errors.Is(err, errSubmissionCapacity) {
		t.Fatalf("issue with every token running = %v, want a capacity refusal", err)
	}
	// Every running dispatch must still be able to record its outcome.
	for _, token := range running {
		guard.complete(token, "/outcome")
	}
	for _, token := range running {
		state, outcome := guard.begin(token)
		if state != anonymousCompleted || outcome != "/outcome" {
			t.Fatalf("running token lost its outcome: state %v outcome %q", state, outcome)
		}
	}
}

func TestGuardDoesNotRecordUnknownTokens(t *testing.T) {
	guard, _ := testGuard()
	before := guard.outstanding()
	if state, _ := guard.begin("not-a-token"); state != anonymousRejected {
		t.Fatalf("unknown token state = %v, want rejected", state)
	}
	if guard.outstanding() != before {
		t.Fatal("an unknown token must not create a record")
	}
}

func TestGuardAcceptsOneDispatchPerToken(t *testing.T) {
	guard, _ := testGuard()
	token, err := guard.issue()
	if err != nil {
		t.Fatal(err)
	}
	if state, _ := guard.begin(token); state != anonymousAccepted {
		t.Fatal("the first dispatch must be accepted")
	}
	if state, _ := guard.begin(token); state != anonymousInProgress {
		t.Fatal("a concurrent dispatch must be reported as in progress")
	}
}

func TestGuardReplaysTheCompletedOutcome(t *testing.T) {
	guard, _ := testGuard()
	token, err := guard.issue()
	if err != nil {
		t.Fatal(err)
	}
	guard.begin(token)
	guard.complete(token, "/members/check-email")
	for attempt := 0; attempt < 3; attempt++ {
		state, outcome := guard.begin(token)
		if state != anonymousCompleted || outcome != "/members/check-email" {
			t.Fatalf("attempt %d: state %v outcome %q, want the first outcome", attempt, state, outcome)
		}
	}
}

func TestGuardExpiresTokensByTheClock(t *testing.T) {
	guard, fixed := testGuard()
	token, err := guard.issue()
	if err != nil {
		t.Fatal(err)
	}
	fixed.Advance(anonymousSubmissionLifetime - time.Second)
	if state, _ := guard.begin(token); state != anonymousAccepted {
		t.Fatal("a token inside its lifetime must be accepted")
	}
	guard.complete(token, "/outcome")

	next, err := guard.issue()
	if err != nil {
		t.Fatal(err)
	}
	fixed.Advance(anonymousSubmissionLifetime)
	if state, _ := guard.begin(next); state != anonymousRejected {
		t.Fatal("a token past its lifetime must be rejected")
	}
	if guard.outstanding() != 0 {
		t.Fatalf("outstanding = %d, want expired tokens to be dropped", guard.outstanding())
	}
}

func TestGuardReleaseMakesATokenUnusable(t *testing.T) {
	guard, _ := testGuard()
	token, err := guard.issue()
	if err != nil {
		t.Fatal(err)
	}
	guard.begin(token)
	guard.release(token)
	if state, _ := guard.begin(token); state != anonymousRejected {
		t.Fatal("a released token must not be reusable")
	}
	if guard.outstanding() != 0 {
		t.Fatalf("outstanding = %d, want the released token dropped", guard.outstanding())
	}
}
