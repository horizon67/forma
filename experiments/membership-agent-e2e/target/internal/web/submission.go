package web

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"example.com/forma-admin-target/internal/clock"
)

// Anonymous submissions cannot be scoped by a client-supplied value such as an
// email address or a cookie, because the caller could then create unbounded
// scopes. The server-generated token is itself the scope, and the guard keeps a
// global cap and a lifetime so the set stays bounded.
const (
	maxOutstandingAnonymousSubmissions = 512
	anonymousSubmissionLifetime        = 30 * time.Minute
)

// The admin surface has its own guard bound to an authenticated principal.
// Signup and resend are anonymous, so this guard is scoped by the
// server-generated token alone and bounded globally instead.
// errSubmissionCapacity is returned when every outstanding token is mid-dispatch
// and none may be discarded. Refusing a new form is safer than dropping a
// running one, whose completion would then be unable to record its outcome.
var errSubmissionCapacity = errors.New("no submission capacity available")

type anonymousSubmissionState uint8

const (
	anonymousRejected anonymousSubmissionState = iota
	anonymousAccepted
	anonymousInProgress
	anonymousCompleted
)

type anonymousSubmissionRecord struct {
	issuedAt  time.Time
	running   bool
	completed bool
	// outcome is the redirect the first completed dispatch produced. A repeat of
	// the same token lands on it again instead of starting a new operation.
	outcome string
}

type anonymousSubmissionGuard struct {
	mu      sync.Mutex
	clock   clock.Clock
	records map[string]*anonymousSubmissionRecord
	order   []string
}

func newAnonymousSubmissionGuard(source clock.Clock) *anonymousSubmissionGuard {
	return &anonymousSubmissionGuard{clock: source, records: map[string]*anonymousSubmissionRecord{}}
}

func (g *anonymousSubmissionGuard) issue() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.expireLocked()
	// Make room before inserting so the cap holds after the insert, not only
	// until the next call.
	if !g.makeRoomLocked() {
		return "", errSubmissionCapacity
	}
	g.records[token] = &anonymousSubmissionRecord{issuedAt: g.clock.Now()}
	g.order = append(g.order, token)
	return token, nil
}

// begin claims a token for one dispatch. A completed token returns its original
// outcome so a repeat lands on the same result rather than applying again.
func (g *anonymousSubmissionGuard) begin(token string) (anonymousSubmissionState, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.expireLocked()
	record, ok := g.records[token]
	if !ok {
		return anonymousRejected, ""
	}
	switch {
	case record.completed:
		return anonymousCompleted, record.outcome
	case record.running:
		return anonymousInProgress, ""
	}
	record.running = true
	return anonymousAccepted, ""
}

// complete records the outcome of the first successful dispatch.
func (g *anonymousSubmissionGuard) complete(token, outcome string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	record, ok := g.records[token]
	if !ok {
		return
	}
	record.running = false
	record.completed = true
	record.outcome = outcome
}

// release drops a token whose dispatch failed validation. The form is
// re-rendered with a fresh token so the member can correct and resubmit.
func (g *anonymousSubmissionGuard) release(token string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.records, token)
	g.removeLocked(token)
}

// expireLocked drops tokens past their lifetime, including running ones: a
// dispatch that outlives the window has no claim on the outcome either.
func (g *anonymousSubmissionGuard) expireLocked() {
	now := g.clock.Now()
	kept := g.order[:0]
	for _, token := range g.order {
		record, ok := g.records[token]
		if !ok {
			continue
		}
		if now.Sub(record.issuedAt) >= anonymousSubmissionLifetime {
			delete(g.records, token)
			continue
		}
		kept = append(kept, token)
	}
	g.order = kept
}

// makeRoomLocked frees one slot if the cap is reached. Running dispatches are
// never discarded, because completing one must still be able to record its
// outcome for a repeat of the same token.
func (g *anonymousSubmissionGuard) makeRoomLocked() bool {
	if len(g.order) < maxOutstandingAnonymousSubmissions {
		return true
	}
	for index, token := range g.order {
		record, ok := g.records[token]
		if !ok || !record.running {
			delete(g.records, token)
			g.order = append(g.order[:index], g.order[index+1:]...)
			return true
		}
	}
	return false
}

func (g *anonymousSubmissionGuard) removeLocked(token string) {
	for index, candidate := range g.order {
		if candidate == token {
			g.order = append(g.order[:index], g.order[index+1:]...)
			return
		}
	}
}

func (g *anonymousSubmissionGuard) outstanding() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.records)
}
