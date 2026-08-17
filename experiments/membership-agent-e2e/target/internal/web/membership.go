package web

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"example.com/forma-admin-target/internal/clock"
	"example.com/forma-admin-target/internal/domain"
	"example.com/forma-admin-target/internal/identity"
	"example.com/forma-admin-target/internal/mail"
	"example.com/forma-admin-target/internal/store"
)

const (
	sessionCookieName = "member_session"
	// genericAuthFailure is the only message an authentication failure produces.
	// Unknown identifier, wrong credential, and an account that is not eligible
	// all land here so the response cannot be used to probe for members.
	genericAuthFailure = "Sign in failed. Check your email address and password."
	// uniformResendNotice is returned whether or not the address belongs to an
	// account awaiting verification.
	uniformResendNotice = "If that address is awaiting verification, a new link is on its way."
)

// Membership holds the identity half of the server.
type Membership struct {
	repository store.Repository
	identity   store.IdentityRepository
	clock      clock.Clock
	outbox     *mail.Outbox
	guard      *anonymousSubmissionGuard
	// secureCookies is off for the plain-HTTP experiment and must be on wherever
	// the application is served over TLS.
	secureCookies bool
	sequence      func() string
}

type membershipForm struct {
	Token  string
	Name   string
	Email  string
	Errors map[string]string
	Notice string
}

func (server *Server) attachMembership(mux *http.ServeMux) {
	mux.HandleFunc("GET /members/signup", server.signUpForm)
	mux.HandleFunc("POST /members/signup", server.signUp)
	mux.HandleFunc("GET /members/check-email", server.checkEmail)
	mux.HandleFunc("POST /members/check-email", server.resend)
	mux.HandleFunc("GET /members/verify", server.verify)
	mux.HandleFunc("GET /members/registered", server.registrationComplete)
	mux.HandleFunc("GET /members/signin", server.signInForm)
	mux.HandleFunc("POST /members/signin", server.signIn)
	mux.HandleFunc("POST /members/signout", server.signOut)
	mux.HandleFunc("GET /members/users/{id}", server.profile)
	mux.HandleFunc("GET /members/users/{id}/edit", server.profileEditForm)
	mux.HandleFunc("POST /members/users/{id}/edit", server.profileEdit)
}

// currentSession resolves the authenticated principal from the session cookie.
// It reports only whether a session exists; loading the record it points at is a
// separate step so a missing or unreadable resource is not mistaken for an
// unauthenticated visitor.
func (server *Server) currentSession(request *http.Request) (identity.Session, bool) {
	cookie, err := request.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return identity.Session{}, false
	}
	session, ok, err := server.membership.identity.FindSession(request.Context(), cookie.Value)
	if err != nil || !ok {
		return identity.Session{}, false
	}
	return session, true
}

// resourceState distinguishes the observable outcomes of loading the record a
// surface presents.
type resourceState uint8

const (
	resourcePresent resourceState = iota
	resourceMissing
	resourceUnavailable
)

func (server *Server) setSessionCookie(writer http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(writer, &http.Cookie{
		Name: sessionCookieName, Value: value, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: server.membership.secureCookies, MaxAge: maxAge,
	})
}

func (server *Server) issueForm(writer http.ResponseWriter, form membershipForm, name string, status int) {
	token, err := server.membership.guard.issue()
	if err != nil {
		http.Error(writer, "try again shortly", http.StatusServiceUnavailable)
		return
	}
	form.Token = token
	// The credential is never written back into the form.
	server.render(writer, status, name, form)
}

func (server *Server) signUpForm(writer http.ResponseWriter, request *http.Request) {
	server.issueForm(writer, membershipForm{}, "signup", http.StatusOK)
}

func (server *Server) signUp(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil {
		http.Error(writer, "invalid form", http.StatusBadRequest)
		return
	}
	token := request.FormValue("submission")
	state, outcome := server.membership.guard.begin(token)
	switch state {
	case anonymousCompleted:
		// A repeat of the same dispatch lands on the first outcome; it does not
		// create a second account or a second notice.
		http.Redirect(writer, request, outcome, http.StatusSeeOther)
		return
	case anonymousInProgress:
		http.Error(writer, "this signup is already being processed", http.StatusConflict)
		return
	case anonymousRejected:
		server.issueForm(writer, membershipForm{
			Name: request.FormValue("name"), Email: request.FormValue("email"),
			Errors: map[string]string{"form": "This form expired. Review the values and submit again."},
		}, "signup", http.StatusConflict)
		return
	}

	name := strings.TrimSpace(request.FormValue("name"))
	email := strings.TrimSpace(request.FormValue("email"))
	credential := request.FormValue("password")
	form := membershipForm{Name: name, Email: email, Errors: map[string]string{}}
	if name == "" {
		form.Errors["name"] = "Name is required."
	}
	if !validEmail(email) {
		form.Errors["email"] = "Enter a valid email address."
	}
	if err := identity.ValidateCredentialPolicy(credential); err != nil {
		form.Errors["password"] = credentialMessage(err)
	}
	if len(form.Errors) != 0 {
		// Validation failed before any commit: drop the token and hand back a
		// fresh one with the non-secret values preserved.
		server.membership.guard.release(token)
		server.issueForm(writer, form, "signup", http.StatusUnprocessableEntity)
		return
	}

	user := domain.User{
		ID: server.membership.sequence(), Name: name, Email: email,
		Plan: domain.PlanFree, Status: domain.StatusPending,
	}
	registration, err := server.membership.identity.Register(
		request.Context(), user, credential, server.membership.clock.Now())
	switch {
	case errors.Is(err, store.ErrIdentifierTaken):
		// Do not disclose that the address is registered: guide everyone to the
		// same resend surface.
		server.membership.guard.complete(token, "/members/check-email?resend=1")
		http.Redirect(writer, request, "/members/check-email?resend=1", http.StatusSeeOther)
		return
	case err != nil:
		server.membership.guard.release(token)
		form.Errors["form"] = "Registration could not be completed. Try again."
		server.issueForm(writer, form, "signup", http.StatusServiceUnavailable)
		return
	}

	// The records are committed. Delivery happens outside that boundary and a
	// failure must not release the token, or a repeat would raise a second
	// notice for the same signup.
	server.deliver(request, registration)
	server.membership.guard.complete(token, "/members/check-email")
	http.Redirect(writer, request, "/members/check-email", http.StatusSeeOther)
}

// deliver sends the committed notice and records the delivery outcome. A
// failure leaves the account awaiting verification and the link usable.
func (server *Server) deliver(request *http.Request, registration store.Registration) {
	link := "/members/verify?token=" + url.QueryEscape(registration.Token)
	err := server.membership.outbox.Send(mail.Message{
		To: registration.Emission.To, Subject: "Confirm your email address", Link: link,
	})
	_ = server.membership.identity.MarkDelivered(request.Context(), registration.Emission.ID, err)
}

func (server *Server) checkEmail(writer http.ResponseWriter, request *http.Request) {
	notice := "Check your email for the confirmation link."
	if request.URL.Query().Get("resend") == "1" {
		notice = uniformResendNotice
	}
	server.issueForm(writer, membershipForm{Notice: notice}, "check-email", http.StatusOK)
}

func (server *Server) resend(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil {
		http.Error(writer, "invalid form", http.StatusBadRequest)
		return
	}
	token := request.FormValue("submission")
	state, outcome := server.membership.guard.begin(token)
	switch state {
	case anonymousCompleted:
		// Fact 20: repeating the same resend dispatch lands on the first result
		// and raises no additional notice.
		http.Redirect(writer, request, outcome, http.StatusSeeOther)
		return
	case anonymousInProgress:
		http.Error(writer, "this request is already being processed", http.StatusConflict)
		return
	case anonymousRejected:
		server.issueForm(writer, membershipForm{
			Email:  request.FormValue("email"),
			Errors: map[string]string{"form": "This form expired. Submit the address again."},
		}, "check-email", http.StatusConflict)
		return
	}

	email := strings.TrimSpace(request.FormValue("email"))
	// The response is the same whether or not the address is awaiting
	// verification, so the outcome is decided before the lookup.
	const destination = "/members/check-email?resend=1"
	user, found, err := server.membership.identity.FindUserByIdentifier(request.Context(), email)
	if err == nil && found {
		registration, reissueErr := server.membership.identity.ReissueEvidence(
			request.Context(), user.ID, server.membership.clock.Now())
		if reissueErr == nil {
			server.deliver(request, registration)
		}
	}
	server.membership.guard.complete(token, destination)
	http.Redirect(writer, request, destination, http.StatusSeeOther)
}

func (server *Server) verify(writer http.ResponseWriter, request *http.Request) {
	token := request.URL.Query().Get("token")
	_, err := server.membership.identity.VerifyEvidence(request.Context(), token, server.membership.clock.Now())
	switch {
	case err == nil:
		http.Redirect(writer, request, "/members/registered", http.StatusSeeOther)
	case errors.Is(err, store.ErrEvidenceExpired):
		server.issueForm(writer, membershipForm{
			Errors: map[string]string{"form": "That link has expired. Request a new one."},
		}, "check-email", http.StatusUnprocessableEntity)
	case errors.Is(err, store.ErrEvidenceConsumed), errors.Is(err, store.ErrEvidenceSuperseded):
		server.issueForm(writer, membershipForm{
			Errors: map[string]string{"form": "That link is no longer valid. Request a new one."},
		}, "check-email", http.StatusUnprocessableEntity)
	default:
		server.issueForm(writer, membershipForm{
			Errors: map[string]string{"form": "That link is not valid. Request a new one."},
		}, "check-email", http.StatusUnprocessableEntity)
	}
}

func (server *Server) registrationComplete(writer http.ResponseWriter, request *http.Request) {
	// Verification does not sign the member in; they continue to the sign-in
	// surface deliberately.
	server.render(writer, http.StatusOK, "registered", membershipForm{
		Notice: "Your email address is confirmed. You can sign in now.",
	})
}

func (server *Server) signInForm(writer http.ResponseWriter, request *http.Request) {
	server.issueForm(writer, membershipForm{}, "signin", http.StatusOK)
}

func (server *Server) signIn(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil {
		http.Error(writer, "invalid form", http.StatusBadRequest)
		return
	}
	token := request.FormValue("submission")
	state, outcome := server.membership.guard.begin(token)
	switch state {
	case anonymousCompleted:
		http.Redirect(writer, request, outcome, http.StatusSeeOther)
		return
	case anonymousInProgress:
		http.Error(writer, "this sign in is already being processed", http.StatusConflict)
		return
	case anonymousRejected:
		server.issueForm(writer, membershipForm{
			Email:  request.FormValue("email"),
			Errors: map[string]string{"form": "This form expired. Sign in again."},
		}, "signin", http.StatusConflict)
		return
	}

	email := strings.TrimSpace(request.FormValue("email"))
	credential := request.FormValue("password")
	user, found, err := server.membership.identity.FindUserByIdentifier(request.Context(), email)
	authenticated := false
	if err == nil && found {
		stored, ok, credentialErr := server.membership.identity.FindCredential(request.Context(), user.ID)
		if credentialErr == nil && ok {
			authenticated = stored.Matches(credential)
		} else {
			identity.EqualiseDerivation(credential)
		}
	} else {
		// Derive against a throwaway credential so an unknown address costs the
		// same as a wrong password.
		identity.EqualiseDerivation(credential)
	}
	if !authenticated || user.Status != domain.StatusActive {
		server.membership.guard.release(token)
		server.issueForm(writer, membershipForm{
			Email: email, Errors: map[string]string{"form": genericAuthFailure},
		}, "signin", http.StatusUnprocessableEntity)
		return
	}

	session, err := server.membership.identity.CreateSession(request.Context(), user.ID, server.membership.clock.Now())
	if err != nil {
		server.membership.guard.release(token)
		server.issueForm(writer, membershipForm{
			Email: email, Errors: map[string]string{"form": genericAuthFailure},
		}, "signin", http.StatusUnprocessableEntity)
		return
	}
	server.setSessionCookie(writer, session.ID, 0)
	destination := "/members/users/" + url.PathEscape(user.ID)
	server.membership.guard.complete(token, destination)
	http.Redirect(writer, request, destination, http.StatusSeeOther)
}

func (server *Server) signOut(writer http.ResponseWriter, request *http.Request) {
	session, ok := server.currentSession(request)
	if !ok {
		http.Redirect(writer, request, "/members/signin", http.StatusSeeOther)
		return
	}
	_ = server.membership.identity.DeleteSession(request.Context(), session.ID)
	server.setSessionCookie(writer, "", -1)
	http.Redirect(writer, request, "/members/signin", http.StatusSeeOther)
}

// requireOwner resolves the session and confirms it owns the requested record.
// Both checks run on the server for every protected surface.
// requireOwnedSession resolves the session, authorises it against the requested
// record, and only then loads the record. Each step has its own outcome:
// no session redirects, a mismatch is refused, a load failure and a missing
// record are reported as such rather than folded into "not signed in".
func (server *Server) requireOwnedSession(
	writer http.ResponseWriter, request *http.Request,
) (identity.Session, domain.User, resourceState, bool) {
	session, ok := server.currentSession(request)
	if !ok {
		http.Redirect(writer, request, "/members/signin", http.StatusSeeOther)
		return identity.Session{}, domain.User{}, resourceUnavailable, false
	}
	if session.UserID != request.PathValue("id") {
		http.Error(writer, "forbidden", http.StatusForbidden)
		return identity.Session{}, domain.User{}, resourceUnavailable, false
	}
	user, found, err := server.repository.FindUser(request.Context(), session.UserID)
	switch {
	case err != nil:
		return session, domain.User{}, resourceUnavailable, true
	case !found:
		return session, domain.User{}, resourceMissing, true
	}
	return session, user, resourcePresent, true
}

func (server *Server) profile(writer http.ResponseWriter, request *http.Request) {
	session, user, state, ok := server.requireOwnedSession(writer, request)
	if !ok {
		return
	}
	switch state {
	case resourceMissing:
		server.render(writer, http.StatusNotFound, "profile", profileData{
			ID: session.UserID, Empty: true,
		})
	case resourceUnavailable:
		server.render(writer, http.StatusServiceUnavailable, "profile", profileData{
			ID: session.UserID, Errors: map[string]string{"form": "Your profile is unavailable right now."},
		})
	default:
		server.render(writer, http.StatusOK, "profile", presentProfile(user))
	}
}

func (server *Server) profileEditForm(writer http.ResponseWriter, request *http.Request) {
	session, user, state, ok := server.requireOwnedSession(writer, request)
	if !ok {
		return
	}
	if state != resourcePresent {
		server.renderProfileEdit(writer, session, profileData{
			ID: session.UserID, Errors: map[string]string{"form": "Your profile is unavailable right now."},
		}, http.StatusServiceUnavailable)
		return
	}
	server.renderProfileEdit(writer, session, presentProfile(user), http.StatusOK)
}

func (server *Server) profileEdit(writer http.ResponseWriter, request *http.Request) {
	session, user, state, ok := server.requireOwnedSession(writer, request)
	if !ok {
		return
	}
	if state != resourcePresent {
		server.renderProfileEdit(writer, session, profileData{
			ID: session.UserID, Errors: map[string]string{"form": "Your profile is unavailable right now."},
		}, http.StatusServiceUnavailable)
		return
	}
	if err := request.ParseForm(); err != nil {
		http.Error(writer, "invalid form", http.StatusBadRequest)
		return
	}
	scope := profileScope(session.ID, user.ID)
	token := request.FormValue("submission")
	destination := "/members/users/" + url.PathEscape(user.ID)
	switch claim, completed := server.submissions.begin(token, scope); claim {
	case submissionCompleted:
		// The same logical save lands on its first result instead of applying a
		// second mutation.
		http.Redirect(writer, request, completed, http.StatusSeeOther)
		return
	case submissionProcessing:
		http.Error(writer, "this save is already being processed", http.StatusConflict)
		return
	case submissionRejected:
		data := presentProfile(user)
		data.Errors = map[string]string{"form": "This form expired. Review the values and save again."}
		data.Name = strings.TrimSpace(request.FormValue("name"))
		data.Nickname = strings.TrimSpace(request.FormValue("nickname"))
		server.renderProfileEdit(writer, session, data, http.StatusConflict)
		return
	}

	updated := user
	updated.Name = strings.TrimSpace(request.FormValue("name"))
	updated.Nickname = strings.TrimSpace(request.FormValue("nickname"))
	if updated.Name == "" {
		server.submissions.fail(token, scope)
		data := presentProfile(user)
		data.Errors = map[string]string{"name": "Name is required."}
		data.Name = updated.Name
		data.Nickname = updated.Nickname
		server.renderProfileEdit(writer, session, data, http.StatusUnprocessableEntity)
		return
	}
	if err := server.repository.UpdateUser(request.Context(), updated); err != nil {
		server.submissions.fail(token, scope)
		data := presentProfile(user)
		data.Errors = map[string]string{"form": "Your profile could not be saved."}
		server.renderProfileEdit(writer, session, data, http.StatusServiceUnavailable)
		return
	}
	server.submissions.complete(token, scope, destination)
	http.Redirect(writer, request, destination, http.StatusSeeOther)
}

// profileScope binds a profile submission to the session that opened the form
// and the record it edits, so a token cannot be replayed against another
// account or after signing out.
func profileScope(sessionID, userID string) string {
	return "profile:" + sessionID + ":" + userID
}

type profileData struct {
	Token    string
	Empty    bool
	ID       string
	Name     string
	Nickname string
	Email    string
	Status   string
	Errors   map[string]string
}

func presentProfile(user domain.User) profileData {
	return profileData{
		ID: user.ID, Name: user.Name, Nickname: user.Nickname,
		Email: user.Email, Status: string(user.Status),
	}
}

// renderProfileEdit always hands out a fresh submission token so a rejected
// save can be corrected and resubmitted.
func (server *Server) renderProfileEdit(
	writer http.ResponseWriter, session identity.Session, data profileData, status int,
) {
	token, err := server.submissions.issue(profileScope(session.ID, data.ID))
	if err != nil {
		http.Error(writer, "try again shortly", http.StatusServiceUnavailable)
		return
	}
	data.Token = token
	server.render(writer, status, "profile-edit", data)
}

func credentialMessage(err error) string {
	switch {
	case errors.Is(err, identity.ErrCredentialTooShort):
		return fmt.Sprintf("Use at least %d characters.", identity.MinCredentialLength)
	case errors.Is(err, identity.ErrCredentialTooLong):
		return fmt.Sprintf("Use at most %d characters.", identity.MaxCredentialLength)
	default:
		return "Use a password made of valid text."
	}
}

// identifierPattern is the declared constraint, applied with RE2 search
// semantics exactly as written. The target must not add rules Forma did not
// state, so no extra whitespace, length, or domain checks appear here.
//
// The spec normalises to NFC before matching. This pattern is unaffected by
// normalisation, because no composition produces or removes "@" or a non-empty
// run on either side of it, so applying the expression directly is semantically
// equivalent here. Patterns whose result does depend on normalisation are a
// separate front-end and agent concern, not a gap in this application.
var identifierPattern = regexp.MustCompile(`.+@.+`)

func validEmail(value string) bool {
	return identifierPattern.MatchString(value)
}

// MembershipOptions configures the identity half of the server.
type MembershipOptions struct {
	Identity      store.IdentityRepository
	Clock         clock.Clock
	Outbox        *mail.Outbox
	SecureCookies bool
	NextUserID    func() string
}

// NewWithMembership builds a server that serves both the admin surfaces and the
// membership flow.
func NewWithMembership(repository store.Repository, options MembershipOptions) (*Server, error) {
	if options.Identity == nil {
		return nil, errors.New("membership requires an identity repository")
	}
	server, err := New(repository)
	if err != nil {
		return nil, err
	}
	source := options.Clock
	if source == nil {
		source = clock.System()
	}
	outbox := options.Outbox
	if outbox == nil {
		outbox = mail.NewOutbox()
	}
	next := options.NextUserID
	if next == nil {
		// HTTP handlers run concurrently, so the default identity must not come
		// from an unsynchronised counter. NextUserID stays available for
		// deterministic fixtures.
		next = func() string {
			raw := make([]byte, 12)
			if _, err := rand.Read(raw); err != nil {
				return ""
			}
			return "member-" + base64.RawURLEncoding.EncodeToString(raw)
		}
	}
	server.membership = &Membership{
		repository: repository, identity: options.Identity, clock: source, outbox: outbox,
		guard: newAnonymousSubmissionGuard(source), secureCookies: options.SecureCookies,
		sequence: next,
	}
	return server, nil
}
