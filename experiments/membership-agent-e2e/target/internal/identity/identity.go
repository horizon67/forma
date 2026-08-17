// Package identity holds the credential, verification evidence, and session
// records for the membership flow. No plaintext secret is stored: credentials
// keep a salted digest and verification evidence keeps a digest of the token
// that was mailed.
package identity

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"time"
	"unicode/utf8"
)

const (
	// MinCredentialLength and MaxCredentialLength are counted in Unicode scalar
	// values, matching the Forma credential policy.
	MinCredentialLength = 12
	MaxCredentialLength = 128

	// VerificationLifetime and MaxVerificationUses mirror the declared
	// verification policy.
	VerificationLifetime = 30 * time.Minute
	MaxVerificationUses  = 1

	// CredentialKDF names the derivation so a stored credential can be re-hashed
	// when the work factor is raised.
	CredentialKDF = "pbkdf2-hmac-sha256"

	// credentialIterations follows the current OWASP guidance for
	// PBKDF2-HMAC-SHA256. crypto/pbkdf2 has been in the standard library since
	// Go 1.24, so no key derivation is hand-rolled here. Whether this work
	// factor is adequate is a human review item.
	credentialIterations = 600000
	credentialKeyLength  = 32
	credentialSaltLength = 16
)

var (
	ErrCredentialTooShort       = errors.New("credential is shorter than the policy allows")
	ErrCredentialTooLong        = errors.New("credential is longer than the policy allows")
	ErrCredentialInvalidUnicode = errors.New("credential is not valid UTF-8")
)

// ValidateCredentialPolicy checks the declared length window. Whitespace is
// preserved, so the value is measured exactly as supplied.
func ValidateCredentialPolicy(value string) error {
	// RuneCountInString counts each invalid byte as one replacement rune, which
	// is not a Unicode scalar value. Reject the encoding before measuring.
	if !utf8.ValidString(value) {
		return ErrCredentialInvalidUnicode
	}
	length := utf8.RuneCountInString(value)
	switch {
	case length < MinCredentialLength:
		return ErrCredentialTooShort
	case length > MaxCredentialLength:
		return ErrCredentialTooLong
	}
	return nil
}

// Credential binds a stored digest to a user. The plaintext never leaves the
// request that supplied it.
type Credential struct {
	UserID     string
	KDF        string
	Iterations int
	Salt       []byte
	Digest     []byte
}

func NewCredential(userID, value string) (Credential, error) {
	if err := ValidateCredentialPolicy(value); err != nil {
		return Credential{}, err
	}
	salt := make([]byte, credentialSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return Credential{}, err
	}
	digest, err := deriveCredential(value, salt, credentialIterations)
	if err != nil {
		return Credential{}, err
	}
	return Credential{
		UserID: userID, KDF: CredentialKDF, Iterations: credentialIterations,
		Salt: salt, Digest: digest,
	}, nil
}

// NeedsRehash reports whether a stored credential was derived with an older
// algorithm or a lower work factor than the current policy.
func (c Credential) NeedsRehash() bool {
	return c.KDF != CredentialKDF || c.Iterations < credentialIterations
}

// Matches reports whether the supplied value reproduces the stored digest. The
// comparison is constant time so a failure does not leak how far it matched.
func (c Credential) Matches(value string) bool {
	if c.KDF != CredentialKDF || c.Iterations <= 0 {
		return false
	}
	digest, err := deriveCredential(value, c.Salt, c.Iterations)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(c.Digest, digest) == 1
}

func deriveCredential(value string, salt []byte, iterations int) ([]byte, error) {
	return pbkdf2.Key(sha256.New, value, salt, iterations, credentialKeyLength)
}

// Evidence is one verification attempt. Only the digest of the mailed token is
// kept, so the store cannot reveal a usable link.
type Evidence struct {
	ID         string
	UserID     string
	TokenSum   []byte
	IssuedAt   time.Time
	ConsumedAt time.Time
	Superseded bool
}

// NewEvidence returns the record to store and the token to mail. The caller
// must put the token in the message and then discard it.
func NewEvidence(id, userID string, issuedAt time.Time) (Evidence, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Evidence{}, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return Evidence{ID: id, UserID: userID, TokenSum: TokenDigest(token), IssuedAt: issuedAt.UTC()}, token, nil
}

func TokenDigest(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func (e Evidence) Consumed() bool { return !e.ConsumedAt.IsZero() }

// Expired reports the declared boundary: valid while now is strictly before
// issuedAt plus the lifetime.
func (e Evidence) Expired(now time.Time) bool {
	return !now.UTC().Before(e.IssuedAt.Add(VerificationLifetime))
}

// Usable is the single precondition the verify operation checks.
func (e Evidence) Usable(now time.Time) bool {
	return !e.Superseded && !e.Consumed() && !e.Expired(now)
}

// EqualiseDerivation performs the same key derivation as a real credential
// check so an unknown identifier costs what a wrong password costs. The result
// is discarded on purpose.
func EqualiseDerivation(value string) {
	salt := make([]byte, credentialSaltLength)
	_, _ = deriveCredential(value, salt, credentialIterations)
}

// Emission is the durable record that a verification notice was raised. It
// holds no token: the mailed link is handed to the caller in memory and the
// record only says who must be told, so the store can never reveal a usable
// link.
type Emission struct {
	ID         string
	UserID     string
	EvidenceID string
	To         string
	CreatedAt  time.Time
	Delivered  bool
	LastError  string
}

// Session is an authenticated principal. Terminating one must not terminate a
// different session for the same user.
type Session struct {
	ID        string
	UserID    string
	CreatedAt time.Time
}

func NewSessionID() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
