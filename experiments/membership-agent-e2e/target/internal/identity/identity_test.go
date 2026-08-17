package identity

import (
	"strings"
	"testing"
	"time"
)

func TestCredentialUsesStandardLibraryPBKDF2(t *testing.T) {
	credential, err := NewCredential("user-1", strings.Repeat("a", MinCredentialLength))
	if err != nil {
		t.Fatal(err)
	}
	if credential.KDF != CredentialKDF {
		t.Fatalf("kdf = %q, want %q", credential.KDF, CredentialKDF)
	}
	if credential.Iterations != credentialIterations {
		t.Fatalf("iterations = %d, want %d", credential.Iterations, credentialIterations)
	}
	if len(credential.Salt) != credentialSaltLength || len(credential.Digest) != credentialKeyLength {
		t.Fatalf("salt = %d bytes, digest = %d bytes", len(credential.Salt), len(credential.Digest))
	}
	if credential.NeedsRehash() {
		t.Error("a freshly derived credential must not need a rehash")
	}
}

func TestCredentialSaltMakesDigestsUnique(t *testing.T) {
	value := strings.Repeat("b", MinCredentialLength)
	first, err := NewCredential("user-1", value)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewCredential("user-2", value)
	if err != nil {
		t.Fatal(err)
	}
	if string(first.Digest) == string(second.Digest) {
		t.Fatal("the same value must not produce the same digest for two accounts")
	}
	if !first.Matches(value) || !second.Matches(value) {
		t.Fatal("both credentials must verify their own value")
	}
	if first.Matches(value + "x") {
		t.Fatal("a different value must not verify")
	}
}

func TestCredentialRejectsWeakerStoredParameters(t *testing.T) {
	credential, err := NewCredential("user-1", strings.Repeat("c", MinCredentialLength))
	if err != nil {
		t.Fatal(err)
	}
	legacy := credential
	legacy.KDF = "sha256-loop"
	if legacy.Matches(strings.Repeat("c", MinCredentialLength)) {
		t.Error("an unknown derivation must not verify")
	}
	if !legacy.NeedsRehash() {
		t.Error("an unknown derivation must be flagged for rehash")
	}
	lower := credential
	lower.Iterations = credentialIterations - 1
	if !lower.NeedsRehash() {
		t.Error("a lower work factor must be flagged for rehash")
	}
}

func TestCredentialPolicyCountsUnicodeScalarValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  error
	}{
		{name: "too short", value: strings.Repeat("a", MinCredentialLength-1), want: ErrCredentialTooShort},
		{name: "minimum", value: strings.Repeat("a", MinCredentialLength)},
		{name: "maximum", value: strings.Repeat("a", MaxCredentialLength)},
		{name: "too long", value: strings.Repeat("a", MaxCredentialLength+1), want: ErrCredentialTooLong},
		// Twelve scalar values that occupy more than twelve bytes.
		{name: "multi-byte scalars", value: strings.Repeat("あ", MinCredentialLength)},
		// Whitespace is preserved, so it counts toward the window.
		{name: "surrounding whitespace", value: "  " + strings.Repeat("a", MinCredentialLength-2)},
		{name: "invalid utf-8", value: strings.Repeat("a", MinCredentialLength-1) + "\xff", want: ErrCredentialInvalidUnicode},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ValidateCredentialPolicy(test.value); got != test.want {
				t.Fatalf("policy = %v, want %v", got, test.want)
			}
		})
	}
}

func TestEvidenceKeepsOnlyATokenDigest(t *testing.T) {
	issued := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	evidence, token, err := NewEvidence("evidence-1", "user-1", issued)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("the mailed token must be returned to the caller")
	}
	if strings.Contains(string(evidence.TokenSum), token) {
		t.Fatal("the stored record must not contain the mailed token")
	}
	if string(evidence.TokenSum) != string(TokenDigest(token)) {
		t.Fatal("the stored digest must match the mailed token")
	}
}

func TestEvidenceExpiryIsAClockRelation(t *testing.T) {
	issued := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	evidence, _, err := NewEvidence("evidence-1", "user-1", issued)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		now     time.Time
		expired bool
	}{
		{name: "before expiry", now: issued.Add(VerificationLifetime - time.Second)},
		{name: "at expiry", now: issued.Add(VerificationLifetime), expired: true},
		{name: "after expiry", now: issued.Add(VerificationLifetime + time.Second), expired: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := evidence.Expired(test.now); got != test.expired {
				t.Fatalf("expired = %v, want %v", got, test.expired)
			}
			if got := evidence.Usable(test.now); got != !test.expired {
				t.Fatalf("usable = %v, want %v", got, !test.expired)
			}
		})
	}
}

func TestEvidenceIsUnusableOnceConsumedOrSuperseded(t *testing.T) {
	issued := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	within := issued.Add(time.Minute)

	consumed, _, err := NewEvidence("evidence-1", "user-1", issued)
	if err != nil {
		t.Fatal(err)
	}
	consumed.ConsumedAt = within
	if !consumed.Consumed() || consumed.Usable(within) {
		t.Error("consumed evidence must not be usable again")
	}

	superseded, _, err := NewEvidence("evidence-2", "user-1", issued)
	if err != nil {
		t.Fatal(err)
	}
	superseded.Superseded = true
	if superseded.Usable(within) {
		t.Error("superseded evidence must not be usable")
	}
}

func TestSessionIDsAreDistinct(t *testing.T) {
	first, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first == second {
		t.Fatalf("session ids must be unique and non-empty: %q %q", first, second)
	}
}
