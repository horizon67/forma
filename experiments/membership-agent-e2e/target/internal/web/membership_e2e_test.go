package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"example.com/forma-admin-target/internal/clock"
	"example.com/forma-admin-target/internal/domain"
	"example.com/forma-admin-target/internal/identity"
	"example.com/forma-admin-target/internal/mail"
	"example.com/forma-admin-target/internal/store"
)

const (
	memberPassword = "correct horse battery"
	// A second policy-valid secret, so a duplicate registration can submit
	// something the store could plausibly have written.
	impostorPassword = "staple hinge lantern"
)

type harness struct {
	t          *testing.T
	handler    http.Handler
	repository *store.Memory
	outbox     *mail.Outbox
	clock      *clock.Fixed
	cookies    map[string]string
}

// newHarness builds the server the same way the executable artifact does, so
// these tests exercise the wiring that ships rather than a private assembly.
func newHarness(t *testing.T) *harness {
	t.Helper()
	repository := store.NewMemory(
		[]domain.Team{{ID: "team-platform", Name: "Platform"}},
		[]domain.User{{
			ID: "user-admin", Name: "Admin", Email: "admin@example.com",
			TeamID: "team-platform", Plan: domain.PlanPro, Status: domain.StatusActive,
		}},
	)
	fixed := clock.NewFixed(time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC))
	outbox := mail.NewOutbox()
	counter := 0
	server, err := NewWithMembership(repository, MembershipOptions{
		Identity: repository, Clock: fixed, Outbox: outbox,
		NextUserID: func() string { counter++; return "member-" + string(rune('a'+counter-1)) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return &harness{
		t: t, handler: server.Handler(), repository: repository,
		outbox: outbox, clock: fixed, cookies: map[string]string{},
	}
}

func (h *harness) do(method, target string, form url.Values) *httptest.ResponseRecorder {
	h.t.Helper()
	var request *http.Request
	if form == nil {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for name, value := range h.cookies {
		request.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.MaxAge < 0 || cookie.Value == "" {
			delete(h.cookies, cookie.Name)
			continue
		}
		h.cookies[cookie.Name] = cookie.Value
	}
	return recorder
}

var memberSubmissionPattern = regexp.MustCompile(`name="submission" value="([^"]+)"`)

func (h *harness) tokenFrom(recorder *httptest.ResponseRecorder) string {
	h.t.Helper()
	match := memberSubmissionPattern.FindStringSubmatch(recorder.Body.String())
	if len(match) != 2 {
		h.t.Fatalf("no submission token in response:\n%s", recorder.Body.String())
	}
	return match[1]
}

// verifyLink reads the confirmation link from the outbox. The store keeps only a
// digest, so this is the only place a test can obtain the token.
func (h *harness) verifyLink() string {
	h.t.Helper()
	message, ok := h.outbox.Latest()
	if !ok {
		h.t.Fatal("no verification notice was emitted")
	}
	return message.Link
}

func (h *harness) signUp(name, email string) {
	h.t.Helper()
	form := h.do(http.MethodGet, "/members/signup", nil)
	values := url.Values{
		"submission": {h.tokenFrom(form)}, "name": {name},
		"email": {email}, "password": {memberPassword},
	}
	if got := h.do(http.MethodPost, "/members/signup", values).Code; got != http.StatusSeeOther {
		h.t.Fatalf("signup status = %d, want 303", got)
	}
}

func (h *harness) activate() {
	h.t.Helper()
	if got := h.do(http.MethodGet, h.verifyLink(), nil).Code; got != http.StatusSeeOther {
		h.t.Fatalf("verify status = %d, want 303", got)
	}
}

func (h *harness) signIn(email string) string {
	h.t.Helper()
	form := h.do(http.MethodGet, "/members/signin", nil)
	values := url.Values{"submission": {h.tokenFrom(form)}, "email": {email}, "password": {memberPassword}}
	recorder := h.do(http.MethodPost, "/members/signin", values)
	if recorder.Code != http.StatusSeeOther {
		h.t.Fatalf("signin status = %d, want 303:\n%s", recorder.Code, recorder.Body.String())
	}
	return recorder.Header().Get("Location")
}

func (h *harness) member(email string) domain.User {
	h.t.Helper()
	user, found, err := h.repository.FindUserByIdentifier(context.Background(), email)
	if err != nil || !found {
		h.t.Fatalf("member %s not found: %v", email, err)
	}
	return user
}

var (
	formControlPattern = regexp.MustCompile(`<(?:input|select|textarea)[^>]*\bname="([^"]+)"`)
	detailFieldPattern = regexp.MustCompile(`data-field="([^"]+)"`)
)

// formControls lists the named controls a page offers, minus the submission
// token, which is transport rather than a declared input. Facts name an exact
// set, so the tests compare sets rather than checking that a control is present.
func formControls(body string) []string {
	var names []string
	for _, match := range formControlPattern.FindAllStringSubmatch(body, -1) {
		if match[1] == "submission" {
			continue
		}
		names = append(names, match[1])
	}
	return names
}

func detailFields(body string) []string {
	var names []string
	for _, match := range detailFieldPattern.FindAllStringSubmatch(body, -1) {
		names = append(names, match[1])
	}
	return names
}

func TestSignUpFormOffersExactlyTheDeclaredInputs(t *testing.T) {
	h := newHarness(t)
	body := h.do(http.MethodGet, "/members/signup", nil).Body.String()
	// The Fact declares one domain field, the identifier, and the credential in
	// that order. An extra control here would be an input Forma never asked for.
	if got := formControls(body); !reflect.DeepEqual(got, []string{"name", "email", "password"}) {
		t.Fatalf("signup controls = %v, want [name email password]", got)
	}
	if !strings.Contains(body, `name="password" type="password"`) {
		t.Error("the credential input must be a password control")
	}
}

func TestProfileShowsExactlyTheDeclaredFields(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	h.activate()
	h.signIn("mia@example.com")
	mia := h.member("mia@example.com")

	body := h.do(http.MethodGet, "/members/users/"+mia.ID, nil).Body.String()
	if got := detailFields(body); !reflect.DeepEqual(got, []string{"name", "nickname", "email", "status"}) {
		t.Fatalf("profile fields = %v, want [name nickname email status]", got)
	}
	if !strings.Contains(body, ">Mia<") || !strings.Contains(body, ">mia@example.com<") {
		t.Error("the declared fields must carry the member's own values")
	}
}

func TestProfileEditOffersExactlyTheDeclaredFields(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	h.activate()
	h.signIn("mia@example.com")
	mia := h.member("mia@example.com")

	body := h.do(http.MethodGet, "/members/users/"+mia.ID+"/edit", nil).Body.String()
	// The edit Fact names name and nickname only. The identifier and the state
	// are shown on the profile but are not editable here.
	if got := formControls(body); !reflect.DeepEqual(got, []string{"name", "nickname"}) {
		t.Fatalf("edit controls = %v, want [name nickname]", got)
	}
	// Submitting the omitted fields anyway must not change them.
	values := url.Values{
		"submission": {h.tokenFrom(h.do(http.MethodGet, "/members/users/"+mia.ID+"/edit", nil))},
		"name":       {"Mia"}, "nickname": {"mi"},
		"email": {"impostor@example.com"}, "status": {string(domain.StatusSuspended)},
	}
	if got := h.do(http.MethodPost, "/members/users/"+mia.ID+"/edit", values).Code; got != http.StatusSeeOther {
		t.Fatalf("edit submit = %d, want 303", got)
	}
	switch saved := h.member("mia@example.com"); {
	case saved.Email != "mia@example.com":
		t.Errorf("email = %q, want the identifier to be unchanged", saved.Email)
	case saved.Status != domain.StatusActive:
		t.Errorf("status = %s, want the state to be unchanged", saved.Status)
	case saved.Nickname != "mi":
		t.Errorf("nickname = %q, want the declared field to be saved", saved.Nickname)
	}
}

func TestMembershipRoutesAreReachableThroughTheShippedHandler(t *testing.T) {
	h := newHarness(t)
	// Every membership surface sits outside the admin role gate.
	for _, target := range []string{
		"/members/signup", "/members/check-email", "/members/signin", "/members/registered",
	} {
		if got := h.do(http.MethodGet, target, nil).Code; got != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 for an anonymous visitor", target, got)
		}
	}
	// The admin surface stays behind it.
	if got := h.do(http.MethodGet, "/admin/users", nil).Code; got != http.StatusForbidden {
		t.Errorf("anonymous GET /admin/users = %d, want 403", got)
	}
}

func TestSignUpCreatesAPendingMemberAndOneNotice(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	if h.repository.UserCount() != 2 || h.repository.CredentialCount() != 1 ||
		h.repository.EvidenceCount() != 1 || h.repository.EmissionCount() != 1 {
		t.Fatalf("after signup: users %d, credentials %d, evidence %d, emissions %d",
			h.repository.UserCount(), h.repository.CredentialCount(),
			h.repository.EvidenceCount(), h.repository.EmissionCount())
	}
	if user := h.member("mia@example.com"); user.Status != domain.StatusPending {
		t.Fatalf("status = %s, want Pending", user.Status)
	}
	if h.outbox.Count() != 1 {
		t.Fatalf("notices = %d, want exactly 1", h.outbox.Count())
	}
}

func TestSignUpValidationKeepsInputAndNeverEchoesTheCredential(t *testing.T) {
	// The Fact closes over the same three invalid cases as the rejection Fact,
	// and each one must preserve the domain inputs while dropping the
	// credential. Covering only the invalid address would leave the credential
	// unchecked on the two paths where it is the submitted secret.
	tests := []struct {
		name, email, password string
		wantPreserved         []string
	}{
		{
			name: "   ", email: "mia@example.com", password: memberPassword,
			// A blank name is preserved as blank; the address is what remains
			// observable on this path.
			wantPreserved: []string{`value="mia@example.com"`},
		},
		{
			name: "Mia", email: "not-an-address", password: memberPassword,
			wantPreserved: []string{`value="Mia"`, `value="not-an-address"`},
		},
		{
			name: "Mia", email: "mia@example.com", password: "short",
			wantPreserved: []string{`value="Mia"`, `value="mia@example.com"`},
		},
	}
	for _, test := range tests {
		t.Run(test.email+"/"+test.password, func(t *testing.T) {
			h := newHarness(t)
			form := h.do(http.MethodGet, "/members/signup", nil)
			token := h.tokenFrom(form)
			values := url.Values{
				"submission": {token}, "name": {test.name},
				"email": {test.email}, "password": {test.password},
			}
			recorder := h.do(http.MethodPost, "/members/signup", values)
			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", recorder.Code)
			}
			body := recorder.Body.String()
			for _, want := range test.wantPreserved {
				if !strings.Contains(body, want) {
					t.Errorf("the rejected form must keep %s", want)
				}
			}
			if strings.Contains(body, test.password) {
				t.Fatal("the credential must never be written back into the form")
			}
			if h.repository.UserCount() != 1 || h.outbox.Count() != 0 {
				t.Fatal("a rejected signup must not create an account or a notice")
			}
			if h.tokenFrom(recorder) == token {
				t.Error("a rejected submission must be handed a fresh token")
			}
		})
	}
}

func TestRepeatedSignUpDispatchAppliesOnce(t *testing.T) {
	h := newHarness(t)
	form := h.do(http.MethodGet, "/members/signup", nil)
	values := url.Values{
		"submission": {h.tokenFrom(form)}, "name": {"Mia"},
		"email": {"mia@example.com"}, "password": {memberPassword},
	}
	first := h.do(http.MethodPost, "/members/signup", values)
	second := h.do(http.MethodPost, "/members/signup", values)
	if first.Code != http.StatusSeeOther || second.Code != http.StatusSeeOther {
		t.Fatalf("statuses = %d and %d, want 303 twice", first.Code, second.Code)
	}
	if first.Header().Get("Location") != second.Header().Get("Location") {
		t.Error("a repeat must land on the first outcome")
	}
	// The navigation Fact names the destination, so agreement between the two
	// dispatches is not enough on its own.
	if location := first.Header().Get("Location"); location != "/members/check-email" {
		t.Errorf("signup landed on %q, want the check-email page", location)
	}
	if h.repository.UserCount() != 2 || h.repository.EmissionCount() != 1 || h.outbox.Count() != 1 {
		t.Fatalf("repeat created extra records: users %d, emissions %d, notices %d",
			h.repository.UserCount(), h.repository.EmissionCount(), h.outbox.Count())
	}
}

func TestExistingIdentifierGuidesToResendWithoutDisclosure(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	before := h.repository.UserCount()

	form := h.do(http.MethodGet, "/members/signup", nil)
	values := url.Values{
		"submission": {h.tokenFrom(form)}, "name": {"Impostor"},
		"email": {"  MIA@Example.com  "}, "password": {memberPassword},
	}
	recorder := h.do(http.MethodPost, "/members/signup", values)
	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/members/check-email?resend=1" {
		t.Fatalf("canonical-equivalent signup = %d %q", recorder.Code, recorder.Header().Get("Location"))
	}
	if h.repository.UserCount() != before {
		t.Fatal("a canonical-equivalent address must not create a second account")
	}
	page := h.do(http.MethodGet, "/members/check-email?resend=1", nil).Body.String()
	if !strings.Contains(page, uniformResendNotice) {
		t.Error("the response must be the uniform resend notice")
	}
}

func TestResendRotatesEvidenceAndRepeatsApplyOnce(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	firstLink := h.verifyLink()

	page := h.do(http.MethodGet, "/members/check-email", nil)
	values := url.Values{"submission": {h.tokenFrom(page)}, "email": {"mia@example.com"}}
	first := h.do(http.MethodPost, "/members/check-email", values)
	second := h.do(http.MethodPost, "/members/check-email", values)
	if first.Code != http.StatusSeeOther || second.Code != http.StatusSeeOther {
		t.Fatalf("resend statuses = %d and %d", first.Code, second.Code)
	}
	// The repeat is the same dispatch, so exactly one extra notice exists.
	if h.outbox.Count() != 2 || h.repository.EmissionCount() != 2 || h.repository.EvidenceCount() != 2 {
		t.Fatalf("after a repeated resend: notices %d, emissions %d, evidence %d, want 2 each",
			h.outbox.Count(), h.repository.EmissionCount(), h.repository.EvidenceCount())
	}
	if user := h.member("mia@example.com"); user.Status != domain.StatusPending {
		t.Fatalf("status = %s, want resend to leave the account Pending", user.Status)
	}
	if got := h.do(http.MethodGet, firstLink, nil).Code; got != http.StatusUnprocessableEntity {
		t.Fatalf("superseded link = %d, want 422", got)
	}
	if got := h.do(http.MethodGet, h.verifyLink(), nil).Code; got != http.StatusSeeOther {
		t.Fatal("the newest link must verify")
	}
}

func TestResendForAnUnknownAddressLooksIdentical(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	page := h.do(http.MethodGet, "/members/check-email", nil)
	values := url.Values{"submission": {h.tokenFrom(page)}, "email": {"nobody@example.com"}}
	recorder := h.do(http.MethodPost, "/members/check-email", values)
	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/members/check-email?resend=1" {
		t.Fatalf("unknown address resend = %d %q", recorder.Code, recorder.Header().Get("Location"))
	}
	if h.repository.EvidenceCount() != 1 || h.outbox.Count() != 1 {
		t.Fatal("an unknown address must not issue evidence or a notice")
	}
}

func TestVerificationExpiryIsCrossedOnlyByTheClock(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	link := h.verifyLink()

	// Move past the boundary without ever writing an expired record.
	h.clock.Advance(identity.VerificationLifetime)
	if got := h.do(http.MethodGet, link, nil).Code; got != http.StatusUnprocessableEntity {
		t.Fatalf("expired link = %d, want 422", got)
	}
	if user := h.member("mia@example.com"); user.Status != domain.StatusPending {
		t.Fatalf("status = %s, want the account to stay Pending", user.Status)
	}
}

func TestVerificationAppliesOnceAndDoesNotSignIn(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	link := h.verifyLink()

	first := h.do(http.MethodGet, link, nil)
	if first.Code != http.StatusSeeOther || first.Header().Get("Location") != "/members/registered" {
		t.Fatalf("verify = %d %q", first.Code, first.Header().Get("Location"))
	}
	if _, ok := h.cookies[sessionCookieName]; ok {
		t.Fatal("verification must not start a session")
	}
	// The Fact names both surfaces: the completion page and the sign-in page it
	// continues to. Landing on the first without the second would leave a
	// verified member with nowhere to go.
	complete := h.do(http.MethodGet, first.Header().Get("Location"), nil).Body.String()
	if !strings.Contains(complete, `href="/members/signin"`) {
		t.Error("the completion page must offer the declared continuation to sign in")
	}
	if got := h.do(http.MethodGet, link, nil).Code; got != http.StatusUnprocessableEntity {
		t.Fatal("a used link must be rejected")
	}
	user := h.member("mia@example.com")
	if user.Status != domain.StatusActive {
		t.Fatalf("status = %s, want Active", user.Status)
	}
	if got := h.repository.MutationCount(user.ID); got != 1 {
		t.Fatalf("activations = %d, want exactly 1", got)
	}
}

func TestPendingMemberCannotSignIn(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	form := h.do(http.MethodGet, "/members/signin", nil)
	values := url.Values{"submission": {h.tokenFrom(form)}, "email": {"mia@example.com"}, "password": {memberPassword}}
	recorder := h.do(http.MethodPost, "/members/signin", values)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("pending signin = %d, want 422", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), genericAuthFailure) {
		t.Error("a pending account must produce the generic failure")
	}
	if h.repository.SessionCount() != 0 {
		t.Fatal("no session may exist")
	}
}

func TestSignInFailuresShareOneMessage(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	h.activate()
	tests := []struct{ name, email, password string }{
		{name: "unknown address", email: "nobody@example.com", password: memberPassword},
		{name: "wrong credential", email: "mia@example.com", password: "wrong password here"},
	}
	var rendered []string
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			form := h.do(http.MethodGet, "/members/signin", nil)
			values := url.Values{"submission": {h.tokenFrom(form)}, "email": {test.email}, "password": {test.password}}
			recorder := h.do(http.MethodPost, "/members/signin", values)
			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", recorder.Code)
			}
			if strings.Contains(recorder.Body.String(), test.password) {
				t.Error("the credential must not be echoed")
			}
			// The submitted address legitimately differs between the cases, so
			// it is normalised out along with the random token before the two
			// responses are compared in full.
			body := strings.ReplaceAll(normaliseTokens(recorder.Body.String()), test.email, "ADDRESS")
			rendered = append(rendered, body)
		})
	}
	if len(rendered) == 2 && rendered[0] != rendered[1] {
		t.Fatalf("failure responses differ:\n%s\n---\n%s", rendered[0], rendered[1])
	}
	if h.repository.SessionCount() != 0 {
		t.Fatal("failed sign in must not create a session")
	}
}

func TestSessionOwnershipIsEnforcedOnTheServer(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	h.activate()
	h.signUp("Noel", "noel@example.com")
	if got := h.do(http.MethodGet, h.verifyLink(), nil).Code; got != http.StatusSeeOther {
		t.Fatal("second member must activate")
	}
	mia := h.member("mia@example.com")
	noel := h.member("noel@example.com")

	if got := h.do(http.MethodGet, "/members/users/"+mia.ID, nil).Code; got != http.StatusSeeOther {
		t.Fatalf("anonymous profile = %d, want a redirect to sign in", got)
	}
	if location := h.signIn("mia@example.com"); location != "/members/users/"+mia.ID {
		t.Fatalf("sign in landed on %q", location)
	}
	if got := h.do(http.MethodGet, "/members/users/"+mia.ID, nil).Code; got != http.StatusOK {
		t.Fatalf("own profile = %d, want 200", got)
	}
	if got := h.do(http.MethodGet, "/members/users/"+noel.ID, nil).Code; got != http.StatusForbidden {
		t.Fatalf("another member's profile = %d, want 403", got)
	}
	if got := h.do(http.MethodGet, "/members/users/"+noel.ID+"/edit", nil).Code; got != http.StatusForbidden {
		t.Fatalf("another member's edit form = %d, want 403", got)
	}
}

func TestProfileEditAppliesOneMutationForARepeatedDispatch(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	h.activate()
	h.signIn("mia@example.com")
	mia := h.member("mia@example.com")
	before := h.repository.MutationCount(mia.ID)

	form := h.do(http.MethodGet, "/members/users/"+mia.ID+"/edit", nil)
	values := url.Values{
		"submission": {h.tokenFrom(form)}, "name": {"Mia Renamed"}, "nickname": {"mi"},
	}
	first := h.do(http.MethodPost, "/members/users/"+mia.ID+"/edit", values)
	second := h.do(http.MethodPost, "/members/users/"+mia.ID+"/edit", values)
	if first.Code != http.StatusSeeOther || second.Code != http.StatusSeeOther {
		t.Fatalf("statuses = %d and %d, want 303 twice", first.Code, second.Code)
	}
	if first.Header().Get("Location") != second.Header().Get("Location") {
		t.Error("the repeat must land on the first outcome")
	}
	if location := first.Header().Get("Location"); location != "/members/users/"+mia.ID {
		t.Errorf("edit landed on %q, want the profile page", location)
	}
	if got := h.repository.MutationCount(mia.ID) - before; got != 1 {
		t.Fatalf("applied mutations = %d, want exactly 1", got)
	}
	page := h.do(http.MethodGet, "/members/users/"+mia.ID, nil).Body.String()
	if !strings.Contains(page, "Mia Renamed") {
		t.Error("the saved name must appear on the profile")
	}
}

func TestProfileEditValidationKeepsInputAndIssuesAFreshToken(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	h.activate()
	h.signIn("mia@example.com")
	mia := h.member("mia@example.com")
	before := h.repository.MutationCount(mia.ID)

	form := h.do(http.MethodGet, "/members/users/"+mia.ID+"/edit", nil)
	token := h.tokenFrom(form)
	values := url.Values{"submission": {token}, "name": {"  "}, "nickname": {"kept"}}
	recorder := h.do(http.MethodPost, "/members/users/"+mia.ID+"/edit", values)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `value="kept"`) {
		t.Error("the rejected form must keep the other input")
	}
	if h.tokenFrom(recorder) == token {
		t.Error("a rejected save must be handed a fresh token")
	}
	if got := h.repository.MutationCount(mia.ID); got != before {
		t.Fatalf("mutations = %d, want the rejected save to change nothing", got)
	}
}

func TestSignOutEndsTheSession(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	h.activate()
	h.signIn("mia@example.com")
	mia := h.member("mia@example.com")

	if got := h.do(http.MethodPost, "/members/signout", nil).Code; got != http.StatusSeeOther {
		t.Fatal("sign out must redirect")
	}
	if got := h.do(http.MethodGet, "/members/users/"+mia.ID, nil).Code; got != http.StatusSeeOther {
		t.Fatal("the ended session must no longer open the profile")
	}
	if h.repository.SessionCount() != 0 {
		t.Fatalf("sessions = %d, want the signed out session removed", h.repository.SessionCount())
	}
}

func TestDeliveryFailureKeepsTheAccountRecoverable(t *testing.T) {
	h := newHarness(t)
	h.outbox.FailNextDelivery()
	h.signUp("Mia", "mia@example.com")
	if user := h.member("mia@example.com"); user.Status != domain.StatusPending {
		t.Fatalf("status = %s, want Pending after a delivery failure", user.Status)
	}
	if h.repository.EmissionCount() != 1 {
		t.Fatal("the emission record must survive the delivery failure")
	}
	if got := h.do(http.MethodGet, h.verifyLink(), nil).Code; got != http.StatusSeeOther {
		t.Fatal("the raised link must still verify")
	}
}

func TestSessionCookieIsOpaqueAndProtected(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	h.activate()
	form := h.do(http.MethodGet, "/members/signin", nil)
	values := url.Values{"submission": {h.tokenFrom(form)}, "email": {"mia@example.com"}, "password": {memberPassword}}
	recorder := h.do(http.MethodPost, "/members/signin", values)

	var session *http.Cookie
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			session = cookie
		}
	}
	if session == nil {
		t.Fatal("sign in must set a session cookie")
	}
	if !session.HttpOnly || session.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie flags = HttpOnly %v SameSite %v", session.HttpOnly, session.SameSite)
	}
	mia := h.member("mia@example.com")
	if strings.Contains(session.Value, mia.ID) || strings.Contains(session.Value, "mia@example.com") {
		t.Error("the cookie must carry an opaque id, not the account it belongs to")
	}
}

// snapshot records every count a rejected registration must leave untouched.
type snapshot struct {
	users, credentials, evidence, emissions, delivered int
}

func (h *harness) snapshot() snapshot {
	return snapshot{
		users: h.repository.UserCount(), credentials: h.repository.CredentialCount(),
		evidence: h.repository.EvidenceCount(), emissions: h.repository.EmissionCount(),
		delivered: h.outbox.Count(),
	}
}

func TestRegistrationRejectsEveryDeclaredInvalidCase(t *testing.T) {
	// The Acceptance Fact closes over three cases; each is exercised in
	// isolation and must leave all five counts unchanged.
	tests := []struct {
		name, field, value string
	}{
		{name: "invalid name", field: "name", value: "   "},
		{name: "invalid identifier", field: "email", value: "not-an-address"},
		{name: "invalid credential", field: "password", value: "short"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t)
			before := h.snapshot()
			form := h.do(http.MethodGet, "/members/signup", nil)
			values := url.Values{
				"submission": {h.tokenFrom(form)}, "name": {"Mia"},
				"email": {"mia@example.com"}, "password": {memberPassword},
			}
			values.Set(test.field, test.value)
			recorder := h.do(http.MethodPost, "/members/signup", values)
			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", recorder.Code)
			}
			if got := h.snapshot(); got != before {
				t.Fatalf("counts = %+v, want them unchanged at %+v", got, before)
			}
		})
	}
}

func TestDuplicateIdentifierCoversExactAndCanonicalForms(t *testing.T) {
	for _, identifier := range []string{"mia@example.com", "  MIA@Example.com  "} {
		t.Run(identifier, func(t *testing.T) {
			h := newHarness(t)
			h.signUp("Mia", "mia@example.com")
			before := h.snapshot()

			// The duplicate attempt carries a different, policy-valid secret.
			// Reusing the original password here would hide an implementation
			// that overwrites the credential with whatever was submitted.
			form := h.do(http.MethodGet, "/members/signup", nil)
			values := url.Values{
				"submission": {h.tokenFrom(form)}, "name": {"Impostor"},
				"email": {identifier}, "password": {impostorPassword},
			}
			recorder := h.do(http.MethodPost, "/members/signup", values)
			if recorder.Code != http.StatusSeeOther ||
				recorder.Header().Get("Location") != "/members/check-email?resend=1" {
				t.Fatalf("duplicate signup = %d %q", recorder.Code, recorder.Header().Get("Location"))
			}
			if got := h.snapshot(); got != before {
				t.Fatalf("counts = %+v, want them unchanged at %+v", got, before)
			}
			// Equal counts would still hold if the duplicate attempt had
			// overwritten the existing credential, so the binding itself is
			// checked against the two secrets that were actually submitted:
			// the original still verifies, the one sent by the duplicate
			// attempt does not.
			h.activate()
			form = h.do(http.MethodGet, "/members/signin", nil)
			attempt := url.Values{
				"submission": {h.tokenFrom(form)}, "email": {"mia@example.com"},
				"password": {impostorPassword},
			}
			if got := h.do(http.MethodPost, "/members/signin", attempt).Code; got != http.StatusUnprocessableEntity {
				t.Fatalf("the duplicate attempt's secret signed in: %d", got)
			}
			mia := h.member("mia@example.com")
			if location := h.signIn("mia@example.com"); location != "/members/users/"+mia.ID {
				t.Fatalf("original credential no longer signs in: %q", location)
			}
		})
	}
}

func TestResendDisclosureIsUniformForEveryAccountState(t *testing.T) {
	// The three declared cases: unknown, an Active member, and one still
	// awaiting verification. Only the pending case may raise a notice, and the
	// visible response must not differ.
	tests := []struct {
		name          string
		address       string
		expectsNotice bool
	}{
		{name: "unknown", address: "nobody@example.com"},
		{name: "active", address: "active@example.com"},
		{name: "pending", address: "pending@example.com", expectsNotice: true},
	}
	var bodies []string
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t)
			h.signUp("Active", "active@example.com")
			h.activate()
			h.signUp("Pending", "pending@example.com")
			before := h.outbox.Count()

			page := h.do(http.MethodGet, "/members/check-email", nil)
			values := url.Values{"submission": {h.tokenFrom(page)}, "email": {test.address}}
			recorder := h.do(http.MethodPost, "/members/check-email", values)
			if recorder.Code != http.StatusSeeOther ||
				recorder.Header().Get("Location") != "/members/check-email?resend=1" {
				t.Fatalf("resend = %d %q", recorder.Code, recorder.Header().Get("Location"))
			}
			added := h.outbox.Count() - before
			want := 0
			if test.expectsNotice {
				want = 1
			}
			if added != want {
				t.Fatalf("notices added = %d, want %d", added, want)
			}
			bodies = append(bodies, normaliseTokens(recorder.Body.String()+
				h.do(http.MethodGet, "/members/check-email?resend=1", nil).Body.String()))
		})
	}
	// Comparing whole responses, not just the presence of one phrase, so a
	// state-specific addition cannot slip in unnoticed.
	for index := 1; index < len(bodies); index++ {
		if bodies[index] != bodies[0] {
			t.Fatalf("state %s renders differently:\n%s\n---\n%s", tests[index].name, bodies[0], bodies[index])
		}
	}
}

// normaliseTokens replaces the per-response random submission token so two
// otherwise identical pages compare equal.
func normaliseTokens(body string) string {
	return memberSubmissionPattern.ReplaceAllString(body, `name="submission" value="TOKEN"`)
}

func TestVerificationRejectionCoversEveryDeclaredCase(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		h := newHarness(t)
		h.signUp("Mia", "mia@example.com")
		if got := h.do(http.MethodGet, "/members/verify?token=not-a-token", nil).Code; got != http.StatusUnprocessableEntity {
			t.Fatalf("invalid token = %d, want 422", got)
		}
		if user := h.member("mia@example.com"); user.Status != domain.StatusPending {
			t.Fatalf("status = %s, want Pending", user.Status)
		}
	})
	t.Run("expired", func(t *testing.T) {
		h := newHarness(t)
		h.signUp("Mia", "mia@example.com")
		link := h.verifyLink()
		h.clock.Advance(identity.VerificationLifetime)
		if got := h.do(http.MethodGet, link, nil).Code; got != http.StatusUnprocessableEntity {
			t.Fatalf("expired token = %d, want 422", got)
		}
		if user := h.member("mia@example.com"); user.Status != domain.StatusPending {
			t.Fatalf("status = %s, want Pending", user.Status)
		}
	})
	t.Run("consumed", func(t *testing.T) {
		h := newHarness(t)
		h.signUp("Mia", "mia@example.com")
		link := h.verifyLink()
		h.activate()
		// Consumed evidence only exists once the subject is Active, which is the
		// state the corrected Acceptance Fact requires.
		mia := h.member("mia@example.com")
		mutations := h.repository.MutationCount(mia.ID)
		if got := h.do(http.MethodGet, link, nil).Code; got != http.StatusUnprocessableEntity {
			t.Fatalf("consumed token = %d, want 422", got)
		}
		if user := h.member("mia@example.com"); user.Status != domain.StatusActive {
			t.Fatalf("status = %s, want the subject to stay Active", user.Status)
		}
		if got := h.repository.MutationCount(mia.ID); got != mutations {
			t.Fatalf("mutations = %d, want no further change from %d", got, mutations)
		}
	})
}

func TestSuccessfulVerificationLeavesTheEvidenceConsumed(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	mia := h.member("mia@example.com")
	issued := h.repository.EvidenceForSubject(mia.ID)
	if len(issued) != 1 || issued[0].Consumed() {
		t.Fatalf("before verification: %d evidence records, consumed = %v", len(issued), issued[0].Consumed())
	}

	h.activate()

	// The Fact observes the condition of the evidence itself, not only that a
	// second use is refused: the one record must be marked consumed rather than
	// deleted, replaced, or left usable.
	after := h.repository.EvidenceForSubject(mia.ID)
	if len(after) != 1 {
		t.Fatalf("evidence records = %d, want the issued record to remain", len(after))
	}
	switch {
	case after[0].ID != issued[0].ID:
		t.Errorf("evidence id = %q, want the issued record %q", after[0].ID, issued[0].ID)
	case !after[0].Consumed():
		t.Error("the evidence must be marked consumed after a successful verification")
	case after[0].Superseded:
		t.Error("a consumed evidence must not also be recorded as superseded")
	}
}

func TestExpiryBoundaryIsCheckedOnBothSides(t *testing.T) {
	tests := []struct {
		name     string
		advance  time.Duration
		accepted bool
	}{
		{name: "before expiry", advance: identity.VerificationLifetime - time.Nanosecond, accepted: true},
		{name: "at expiry", advance: identity.VerificationLifetime},
		{name: "after expiry", advance: identity.VerificationLifetime + time.Nanosecond},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t)
			h.signUp("Mia", "mia@example.com")
			link := h.verifyLink()
			h.clock.Advance(test.advance)
			recorder := h.do(http.MethodGet, link, nil)
			want := http.StatusUnprocessableEntity
			wantStatus := domain.StatusPending
			if test.accepted {
				want = http.StatusSeeOther
				wantStatus = domain.StatusActive
			}
			if recorder.Code != want {
				t.Fatalf("verify = %d, want %d", recorder.Code, want)
			}
			if user := h.member("mia@example.com"); user.Status != wantStatus {
				t.Fatalf("status = %s, want %s", user.Status, wantStatus)
			}
		})
	}
}

func TestSignOutClosesBothProtectedSurfaces(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	h.activate()
	h.signIn("mia@example.com")
	mia := h.member("mia@example.com")

	if got := h.do(http.MethodPost, "/members/signout", nil).Code; got != http.StatusSeeOther {
		t.Fatal("sign out must redirect")
	}
	for _, target := range []string{
		"/members/users/" + mia.ID,
		"/members/users/" + mia.ID + "/edit",
	} {
		if got := h.do(http.MethodGet, target, nil).Code; got != http.StatusSeeOther {
			t.Errorf("GET %s after sign out = %d, want a redirect to sign in", target, got)
		}
	}
}

func TestProfileReportsFailureWhenTheRecordCannotBeRead(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	h.activate()
	h.signIn("mia@example.com")
	mia := h.member("mia@example.com")

	h.repository.FailNext()
	recorder := h.do(http.MethodGet, "/members/users/"+mia.ID, nil)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unreadable profile = %d, want 503 rather than a sign-in redirect", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `data-state="failure"`) {
		t.Error("the failure state must be observable on the page")
	}
}

func TestProfileEditReportsFailureWhenTheSaveCannotCommit(t *testing.T) {
	h := newHarness(t)
	h.signUp("Mia", "mia@example.com")
	h.activate()
	h.signIn("mia@example.com")
	mia := h.member("mia@example.com")
	before := h.repository.MutationCount(mia.ID)

	form := h.do(http.MethodGet, "/members/users/"+mia.ID+"/edit", nil)
	values := url.Values{"submission": {h.tokenFrom(form)}, "name": {"Mia Renamed"}, "nickname": {"mi"}}
	h.repository.FailNext()
	recorder := h.do(http.MethodPost, "/members/users/"+mia.ID+"/edit", values)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("failed save = %d, want 503", recorder.Code)
	}
	if got := h.repository.MutationCount(mia.ID); got != before {
		t.Fatalf("mutations = %d, want the failed save to change nothing", got)
	}
	// A fresh token lets the member retry after the failure.
	if h.tokenFrom(recorder) == values.Get("submission") {
		t.Error("a failed save must be handed a fresh token")
	}
}

// TestProfileReportsEmptyWhenTheRecordIsGone builds the one arrangement the
// empty state describes: the session store still holds a valid session while
// the domain store no longer has the record it names. The two are separate
// repositories in the interface, so this is a real divergence rather than an
// injected outcome.
func TestProfileReportsEmptyWhenTheRecordIsGone(t *testing.T) {
	identityStore := store.NewMemory(nil, nil)
	registration, err := identityStore.Register(
		context.Background(),
		domain.User{ID: "member-gone", Name: "Gone", Email: "gone@example.com", Plan: domain.PlanFree, Status: domain.StatusPending},
		memberPassword, time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identityStore.VerifyEvidence(context.Background(), registration.Token, time.Now()); err != nil {
		t.Fatal(err)
	}
	session, sessionErr := identityStore.CreateSession(context.Background(), "member-gone", time.Now())
	if sessionErr != nil {
		t.Fatal(sessionErr)
	}

	// The domain store never had this member.
	domainStore := store.NewMemory(nil, nil)
	server, serverErr := NewWithMembership(domainStore, MembershipOptions{Identity: identityStore})
	if serverErr != nil {
		t.Fatal(serverErr)
	}
	request := httptest.NewRequest(http.MethodGet, "/members/users/member-gone", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing record = %d, want 404 rather than a sign-in redirect", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `data-state="empty"`) {
		t.Fatalf("the empty state must be observable:\n%s", recorder.Body.String())
	}
}
