package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/forma-admin-target/internal/domain"
)

const testCredential = "correct horse battery"

func membershipRepository() *Memory {
	return NewMemory(
		[]domain.Team{{ID: "team-platform", Name: "Platform"}},
		[]domain.User{{
			ID: "user-alice", Name: "Alice", Email: "alice@example.com",
			TeamID: "team-platform", Plan: domain.PlanFree, Status: domain.StatusActive,
		}},
	)
}

func pendingUser(id, email string) domain.User {
	return domain.User{ID: id, Name: "New", Email: email, Plan: domain.PlanFree, Status: domain.StatusPending}
}

func TestRegisterCommitsFourRecordsTogether(t *testing.T) {
	repository := membershipRepository()
	issued := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	registration, err := repository.Register(context.Background(), pendingUser("user-bob", "bob@example.com"), testCredential, issued)
	if err != nil {
		t.Fatal(err)
	}
	if repository.UserCount() != 2 || repository.CredentialCount() != 1 ||
		repository.EvidenceCount() != 1 || repository.EmissionCount() != 1 {
		t.Fatalf("committed records = users %d, credentials %d, evidence %d, emissions %d",
			repository.UserCount(), repository.CredentialCount(), repository.EvidenceCount(), repository.EmissionCount())
	}
	if registration.Token == "" {
		t.Fatal("the mailed token must be returned in memory")
	}
	emission, ok := repository.EmissionByID(registration.Emission.ID)
	if !ok || emission.To != "bob@example.com" || emission.Delivered {
		t.Fatalf("emission record = %#v", emission)
	}
	if strings.Contains(emission.To+emission.LastError+emission.EvidenceID, registration.Token) {
		t.Fatal("the emission record must not carry the token")
	}
}

func TestRegisterFailureLeavesNoPartialAccount(t *testing.T) {
	repository := membershipRepository()
	repository.FailNext()
	_, err := repository.Register(context.Background(), pendingUser("user-bob", "bob@example.com"), testCredential, time.Now())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("register error = %v", err)
	}
	if repository.UserCount() != 1 || repository.CredentialCount() != 0 ||
		repository.EvidenceCount() != 0 || repository.EmissionCount() != 0 {
		t.Fatal("a failed registration must leave no user, credential, evidence, or emission")
	}
}

func TestRegisterRejectsCanonicalEquivalentIdentifier(t *testing.T) {
	repository := membershipRepository()
	for _, identifier := range []string{"ALICE@example.com", "  alice@example.com  ", "Alice@Example.com"} {
		_, err := repository.Register(context.Background(), pendingUser("user-clone", identifier), testCredential, time.Now())
		if !errors.Is(err, ErrIdentifierTaken) {
			t.Fatalf("register %q = %v, want the identifier to be taken", identifier, err)
		}
	}
	if repository.UserCount() != 1 {
		t.Fatal("a rejected registration must not create a second account")
	}
}

func TestAdminEditRejectsCanonicalEquivalentIdentifier(t *testing.T) {
	repository := membershipRepository()
	registration, err := repository.Register(context.Background(), pendingUser("user-bob", "bob@example.com"), testCredential, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_ = registration
	bob, _, err := repository.FindUser(context.Background(), "user-bob")
	if err != nil {
		t.Fatal(err)
	}
	bob.Email = "  ALICE@example.com  "
	if err := repository.UpdateUser(context.Background(), bob); !errors.Is(err, ErrDuplicateEmail) {
		t.Fatalf("admin edit = %v, want a duplicate identifier", err)
	}
	exists, err := repository.EmailExists(context.Background(), " Alice@Example.com ", "user-bob")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("EmailExists must fold the same way registration does")
	}
}

func TestVerifyActivatesExactlyOnce(t *testing.T) {
	repository := membershipRepository()
	issued := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	registration, err := repository.Register(context.Background(), pendingUser("user-bob", "bob@example.com"), testCredential, issued)
	if err != nil {
		t.Fatal(err)
	}
	within := issued.Add(time.Minute)
	user, err := repository.VerifyEvidence(context.Background(), registration.Token, within)
	if err != nil {
		t.Fatal(err)
	}
	if user.Status != domain.StatusActive {
		t.Fatalf("status = %s, want Active", user.Status)
	}
	if _, err := repository.VerifyEvidence(context.Background(), registration.Token, within); !errors.Is(err, ErrEvidenceConsumed) {
		t.Fatalf("second verify = %v, want a consumed rejection", err)
	}
	if got := repository.MutationCount("user-bob"); got != 1 {
		t.Fatalf("activations = %d, want exactly 1", got)
	}
}

func TestVerifyFailureLeavesUserAndEvidenceUnchanged(t *testing.T) {
	issued := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		token func(string) string
		now   time.Time
		want  error
	}{
		{name: "unknown", token: func(string) string { return "not-a-token" }, now: issued.Add(time.Minute), want: ErrVerificationRejected},
		// The expiry case advances the clock instead of writing an expired record.
		{name: "expired", token: func(token string) string { return token }, now: issued.Add(30 * time.Minute), want: ErrEvidenceExpired},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := membershipRepository()
			registration, err := repository.Register(context.Background(), pendingUser("user-bob", "bob@example.com"), testCredential, issued)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := repository.VerifyEvidence(context.Background(), test.token(registration.Token), test.now); !errors.Is(err, test.want) {
				t.Fatalf("verify = %v, want %v", err, test.want)
			}
			user, _, err := repository.FindUser(context.Background(), "user-bob")
			if err != nil {
				t.Fatal(err)
			}
			if user.Status != domain.StatusPending {
				t.Fatalf("status = %s, want the account to stay Pending", user.Status)
			}
			evidence, ok := repository.EvidenceByID(registration.Evidence.ID)
			if !ok || evidence.Consumed() || evidence.Superseded {
				t.Fatalf("evidence = %#v, want it untouched", evidence)
			}
			if repository.MutationCount("user-bob") != 0 {
				t.Fatal("a rejected verification must not mutate the account")
			}
		})
	}
}

func TestResendRotatesEvidenceAndEmissionTogether(t *testing.T) {
	repository := membershipRepository()
	issued := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	first, err := repository.Register(context.Background(), pendingUser("user-bob", "bob@example.com"), testCredential, issued)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.ReissueEvidence(context.Background(), "user-bob", issued.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if repository.EvidenceCount() != 2 || repository.EmissionCount() != 2 {
		t.Fatalf("after resend: evidence %d, emissions %d, want 2 and 2",
			repository.EvidenceCount(), repository.EmissionCount())
	}
	user, _, err := repository.FindUser(context.Background(), "user-bob")
	if err != nil {
		t.Fatal(err)
	}
	if user.Status != domain.StatusPending {
		t.Fatalf("status = %s, want resend to leave the account Pending", user.Status)
	}
	if _, err := repository.VerifyEvidence(context.Background(), first.Token, issued.Add(2*time.Minute)); !errors.Is(err, ErrEvidenceSuperseded) {
		t.Fatalf("old token = %v, want a superseded rejection", err)
	}
	if _, err := repository.VerifyEvidence(context.Background(), second.Token, issued.Add(2*time.Minute)); err != nil {
		t.Fatalf("new token must verify: %v", err)
	}
}

func TestResendRejectsAccountsThatAreNotPending(t *testing.T) {
	repository := membershipRepository()
	// Alice is already Active in the fixture.
	if _, err := repository.ReissueEvidence(context.Background(), "user-alice", time.Now()); !errors.Is(err, ErrNotPending) {
		t.Fatalf("resend to an Active account = %v, want ErrNotPending", err)
	}
	if repository.EvidenceCount() != 0 || repository.EmissionCount() != 0 {
		t.Fatal("a rejected resend must not issue evidence or an emission")
	}
}

func TestRegisterRequiresTheDeclaredInitialState(t *testing.T) {
	repository := membershipRepository()
	active := pendingUser("user-bob", "bob@example.com")
	active.Status = domain.StatusActive
	if _, err := repository.Register(context.Background(), active, testCredential, time.Now()); !errors.Is(err, ErrNotPending) {
		t.Fatalf("register with a non-Pending state = %v, want ErrNotPending", err)
	}
	if repository.UserCount() != 1 {
		t.Fatal("a rejected registration must not create an account")
	}
}

func TestSessionRequiresAnActiveAccount(t *testing.T) {
	repository := membershipRepository()
	if _, err := repository.CreateSession(context.Background(), "user-missing", time.Now()); err == nil {
		t.Fatal("an unknown account must not start a session")
	}
	if _, err := repository.Register(context.Background(), pendingUser("user-bob", "bob@example.com"), testCredential, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateSession(context.Background(), "user-bob", time.Now()); !errors.Is(err, ErrNotActive) {
		t.Fatalf("a Pending account must not start a session: %v", err)
	}
	if repository.SessionCount() != 0 {
		t.Fatal("no session may exist after both rejections")
	}
}

func TestResendFailureLeavesThePriorLinkUsable(t *testing.T) {
	repository := membershipRepository()
	issued := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	first, err := repository.Register(context.Background(), pendingUser("user-bob", "bob@example.com"), testCredential, issued)
	if err != nil {
		t.Fatal(err)
	}
	// The only failure point is before any record changes: evidence is prepared
	// first, so superseding and committing cannot fail afterwards.
	repository.FailNext()
	if _, err := repository.ReissueEvidence(context.Background(), "user-bob", issued.Add(time.Minute)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("resend = %v, want the store failure", err)
	}
	if repository.EvidenceCount() != 1 || repository.EmissionCount() != 1 {
		t.Fatal("a failed resend must not add evidence or an emission")
	}
	if _, err := repository.VerifyEvidence(context.Background(), first.Token, issued.Add(2*time.Minute)); err != nil {
		t.Fatalf("the prior link must stay usable after a failed resend: %v", err)
	}
}

func TestDeliveryFailureKeepsTheEmissionAndThePendingAccount(t *testing.T) {
	repository := membershipRepository()
	issued := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	registration, err := repository.Register(context.Background(), pendingUser("user-bob", "bob@example.com"), testCredential, issued)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkDelivered(context.Background(), registration.Emission.ID, errors.New("smtp refused")); err != nil {
		t.Fatal(err)
	}
	emission, ok := repository.EmissionByID(registration.Emission.ID)
	if !ok || emission.Delivered || emission.LastError == "" {
		t.Fatalf("emission = %#v, want a recorded failure", emission)
	}
	user, _, err := repository.FindUser(context.Background(), "user-bob")
	if err != nil {
		t.Fatal(err)
	}
	if user.Status != domain.StatusPending {
		t.Fatal("a delivery failure must not activate or remove the account")
	}
	if _, err := repository.VerifyEvidence(context.Background(), registration.Token, issued.Add(time.Minute)); err != nil {
		t.Fatalf("the issued link must still verify after a delivery failure: %v", err)
	}
}
