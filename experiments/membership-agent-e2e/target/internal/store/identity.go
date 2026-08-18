package store

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/forma-admin-target/internal/domain"
	"example.com/forma-admin-target/internal/identity"
)

var (
	// ErrIdentifierTaken names no account: the caller turns it into the uniform
	// resend guidance so the response cannot be used to probe for members.
	ErrIdentifierTaken = errors.New("identifier is already registered")
	// ErrVerificationRejected covers unknown, expired, superseded, and already
	// consumed evidence. The caller reports the specific reason it asked for,
	// never the account behind it.
	ErrVerificationRejected = errors.New("verification evidence is not usable")
	ErrEvidenceExpired      = errors.New("verification evidence expired")
	ErrEvidenceConsumed     = errors.New("verification evidence was already used")
	ErrEvidenceSuperseded   = errors.New("verification evidence was replaced")
	ErrNotPending           = errors.New("account is not awaiting verification")
	// ErrNotActive is the eligibility failure for authentication. It is distinct
	// from ErrNotPending so a diagnostic cannot claim the opposite state.
	ErrNotActive = errors.New("account is not eligible to sign in")
	// ErrDuplicateIdentity guards against an account identity that is empty or
	// already in use.
	ErrDuplicateIdentity = errors.New("account identity is empty or already in use")
)

// CanonicalIdentifier folds the declared identifier canonicalization: surrounding
// Unicode whitespace is trimmed and ASCII case is folded. Every lookup,
// uniqueness check, registration, and admin edit must go through this function
// so the admin and membership surfaces cannot disagree about identity.
func CanonicalIdentifier(value string) string {
	trimmed := strings.TrimSpace(value)
	folded := []rune(trimmed)
	for index, character := range folded {
		if character >= 'A' && character <= 'Z' {
			folded[index] = character + ('a' - 'A')
		}
	}
	return string(folded)
}

// Registration is the committed result of one signup. Token is held only in
// memory for the caller to mail; it is never stored.
type Registration struct {
	Evidence identity.Evidence
	Emission identity.Emission
	Token    string
}

// IdentityRepository keeps every multi-record membership operation behind one
// atomic call, so a partial signup, verification, or resend cannot be observed.
type IdentityRepository interface {
	Register(context.Context, domain.User, string, time.Time) (Registration, error)
	FindUserByIdentifier(context.Context, string) (domain.User, bool, error)
	FindCredential(context.Context, string) (identity.Credential, bool, error)
	VerifyEvidence(context.Context, string, time.Time) (domain.User, error)
	ReissueEvidence(context.Context, string, time.Time) (Registration, error)
	MarkDelivered(context.Context, string, error) error
	CreateSession(context.Context, string, time.Time) (identity.Session, error)
	FindSession(context.Context, string) (identity.Session, bool, error)
	DeleteSession(context.Context, string) error
}

func (repository *Memory) nextID(prefix string) string {
	repository.sequence++
	return fmt.Sprintf("%s-%d", prefix, repository.sequence)
}

func evidenceKey(sum []byte) string { return base64.RawURLEncoding.EncodeToString(sum) }

// buildIssue derives the evidence and emission values. It advances the id
// sequence but changes no semantic record, so every failure happens before any
// account, evidence, or emission changes and no rollback is needed. Callers
// hold repository.mu.
func (repository *Memory) buildIssue(user domain.User, issuedAt time.Time) (Registration, error) {
	evidence, token, err := identity.NewEvidence(repository.nextID("evidence"), user.ID, issuedAt)
	if err != nil {
		return Registration{}, err
	}
	emission := identity.Emission{
		ID: repository.nextID("emission"), UserID: user.ID, EvidenceID: evidence.ID,
		To: user.Email, CreatedAt: issuedAt.UTC(),
	}
	return Registration{Evidence: evidence, Emission: emission, Token: token}, nil
}

// commitIssue writes the prepared records. It cannot fail. Callers hold
// repository.mu.
func (repository *Memory) commitIssue(registration Registration) {
	evidence := registration.Evidence
	emission := registration.Emission
	repository.evidence[evidence.ID] = evidence
	repository.evidenceOrder = append(repository.evidenceOrder, evidence.ID)
	repository.evidenceByToken[evidenceKey(evidence.TokenSum)] = evidence.ID
	repository.emissions[emission.ID] = emission
	repository.emissionOrder = append(repository.emissionOrder, emission.ID)
}

// Register commits the user, the credential, the first verification evidence,
// and the notice emission record as one outcome. Only the external delivery
// happens afterwards, because it cannot be rolled back.
func (repository *Memory) Register(
	_ context.Context, user domain.User, credentialValue string, issuedAt time.Time,
) (Registration, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.consumeFailure() {
		return Registration{}, ErrUnavailable
	}
	// The declared initial state is guaranteed at this boundary, not by the
	// caller that assembled the record.
	if user.Status != domain.StatusPending {
		return Registration{}, ErrNotPending
	}
	if user.ID == "" {
		return Registration{}, ErrDuplicateIdentity
	}
	if _, exists := repository.users[user.ID]; exists {
		// A colliding identity would overwrite an account and add the same id to
		// the order twice, so it is refused at the authoritative boundary.
		return Registration{}, ErrDuplicateIdentity
	}
	canonical := CanonicalIdentifier(user.Email)
	for _, id := range repository.order {
		if CanonicalIdentifier(repository.users[id].Email) == canonical {
			// Rejection must not rewrite the existing credential. A taken
			// identifier with counts unchanged would still steal the account
			// if the duplicate attempt's secret replaced the original binding.
			return Registration{}, ErrIdentifierTaken
		}
	}
	credential, err := identity.NewCredential(user.ID, credentialValue)
	if err != nil {
		return Registration{}, err
	}
	registration, err := repository.buildIssue(user, issuedAt)
	if err != nil {
		return Registration{}, err
	}
	repository.commitIssue(registration)
	repository.users[user.ID] = user
	repository.order = append(repository.order, user.ID)
	repository.credentials[user.ID] = credential
	return registration, nil
}

// VerifyEvidence resolves, checks, consumes, and activates under one lock. A
// second dispatch of the same token is rejected as consumed rather than treated
// as a repeat success.
func (repository *Memory) VerifyEvidence(_ context.Context, token string, now time.Time) (domain.User, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.consumeFailure() {
		return domain.User{}, ErrUnavailable
	}
	id, ok := repository.evidenceByToken[evidenceKey(identity.TokenDigest(token))]
	if !ok {
		return domain.User{}, ErrVerificationRejected
	}
	evidence := repository.evidence[id]
	// Defence in depth: the index is keyed by digest, and the digest itself is
	// compared without an early exit.
	if subtle.ConstantTimeCompare(evidence.TokenSum, identity.TokenDigest(token)) != 1 {
		return domain.User{}, ErrVerificationRejected
	}
	switch {
	case evidence.Consumed():
		return domain.User{}, ErrEvidenceConsumed
	case evidence.Superseded:
		return domain.User{}, ErrEvidenceSuperseded
	case evidence.Expired(now):
		return domain.User{}, ErrEvidenceExpired
	}
	user, ok := repository.users[evidence.UserID]
	if !ok {
		return domain.User{}, ErrVerificationRejected
	}
	if user.Status != domain.StatusPending {
		return domain.User{}, ErrNotPending
	}
	evidence.ConsumedAt = now.UTC()
	repository.evidence[id] = evidence
	user.Status = domain.StatusActive
	repository.users[user.ID] = user
	repository.mutationCount[user.ID]++
	return user, nil
}

// ReissueEvidence supersedes every unconsumed evidence for the user and commits
// the replacement together with its emission record. The user's own state is
// unchanged.
func (repository *Memory) ReissueEvidence(
	_ context.Context, userID string, issuedAt time.Time,
) (Registration, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.consumeFailure() {
		return Registration{}, ErrUnavailable
	}
	user, ok := repository.users[userID]
	if !ok {
		return Registration{}, ErrVerificationRejected
	}
	// Only an account still awaiting verification can be resent to. Checking
	// here rather than in the handler removes the window where the state
	// changes between the check and the write.
	if user.Status != domain.StatusPending {
		return Registration{}, ErrNotPending
	}
	// Prepare first: once the replacement exists, superseding the previous
	// evidence and committing cannot fail, so there is no rollback path.
	registration, err := repository.buildIssue(user, issuedAt)
	if err != nil {
		return Registration{}, err
	}
	for _, id := range repository.evidenceOrder {
		evidence := repository.evidence[id]
		if evidence.UserID == userID && !evidence.Consumed() && !evidence.Superseded {
			evidence.Superseded = true
			repository.evidence[id] = evidence
		}
	}
	repository.commitIssue(registration)
	return registration, nil
}

// MarkDelivered records the outcome of the external delivery. A failure keeps
// the emission and leaves the account awaiting verification.
func (repository *Memory) MarkDelivered(_ context.Context, emissionID string, deliveryErr error) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	emission, ok := repository.emissions[emissionID]
	if !ok {
		return errors.New("unknown emission")
	}
	if deliveryErr != nil {
		emission.LastError = deliveryErr.Error()
	} else {
		emission.Delivered = true
		emission.LastError = ""
	}
	repository.emissions[emissionID] = emission
	return nil
}

func (repository *Memory) FindUserByIdentifier(_ context.Context, identifier string) (domain.User, bool, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	canonical := CanonicalIdentifier(identifier)
	for _, id := range repository.order {
		user := repository.users[id]
		if CanonicalIdentifier(user.Email) == canonical {
			return user, true, nil
		}
	}
	return domain.User{}, false, nil
}

func (repository *Memory) FindCredential(_ context.Context, userID string) (identity.Credential, bool, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	credential, ok := repository.credentials[userID]
	return credential, ok, nil
}

func (repository *Memory) CreateSession(_ context.Context, userID string, at time.Time) (identity.Session, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.consumeFailure() {
		return identity.Session{}, ErrUnavailable
	}
	// A session may only start for an account that exists and is eligible to
	// sign in, re-checked here so the handler cannot race the state.
	user, ok := repository.users[userID]
	if !ok {
		return identity.Session{}, ErrVerificationRejected
	}
	if user.Status != domain.StatusActive {
		return identity.Session{}, ErrNotActive
	}
	id, err := identity.NewSessionID()
	if err != nil {
		return identity.Session{}, err
	}
	session := identity.Session{ID: id, UserID: userID, CreatedAt: at.UTC()}
	repository.sessions[id] = session
	return session, nil
}

func (repository *Memory) FindSession(_ context.Context, id string) (identity.Session, bool, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	session, ok := repository.sessions[id]
	return session, ok, nil
}

func (repository *Memory) DeleteSession(_ context.Context, id string) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	delete(repository.sessions, id)
	return nil
}

// Observation helpers for assertions that must read state without going through
// a mutation path.

func (repository *Memory) EvidenceByID(id string) (identity.Evidence, bool) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	evidence, ok := repository.evidence[id]
	return evidence, ok
}

// EvidenceForSubject returns the subject's evidence in issue order, so a test
// can read the condition of a record whose token it never held.
func (repository *Memory) EvidenceForSubject(userID string) []identity.Evidence {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	var records []identity.Evidence
	for _, id := range repository.evidenceOrder {
		if evidence := repository.evidence[id]; evidence.UserID == userID {
			records = append(records, evidence)
		}
	}
	return records
}

func (repository *Memory) EvidenceCount() int {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	return len(repository.evidenceOrder)
}

func (repository *Memory) EmissionCount() int {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	return len(repository.emissionOrder)
}

func (repository *Memory) EmissionByID(id string) (identity.Emission, bool) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	emission, ok := repository.emissions[id]
	return emission, ok
}

func (repository *Memory) CredentialCount() int {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	return len(repository.credentials)
}

func (repository *Memory) SessionCount() int {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	return len(repository.sessions)
}

func (repository *Memory) UserCount() int {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	return len(repository.order)
}
