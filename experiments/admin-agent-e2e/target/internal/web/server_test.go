package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"example.com/forma-admin-target/internal/domain"
	"example.com/forma-admin-target/internal/store"
)

func TestAccessDenied(t *testing.T) {
	handler, _ := fixtureHandler(t)
	for _, test := range []struct {
		method string
		path   string
		form   url.Values
	}{
		{method: http.MethodGet, path: "/admin/users"},
		{method: http.MethodGet, path: "/admin/users/user-alice"},
		{method: http.MethodGet, path: "/admin/users/user-alice/edit"},
		{method: http.MethodPost, path: "/admin/users/user-alice/edit", form: validEditValues("unused")},
	} {
		response := perform(handler, test.method, test.path, test.form, false)
		if response.Code != http.StatusForbidden {
			t.Errorf("%s %s status = %d, want 403", test.method, test.path, response.Code)
		}
	}
}

func TestAdminList(t *testing.T) {
	handler, _ := fixtureHandler(t)
	response := perform(handler, http.MethodGet, "/admin/users", nil, true)
	assertStatus(t, response, http.StatusOK)
	body := response.Body.String()
	if count := strings.Count(body, "data-user-row="); count != 4 {
		t.Fatalf("visible rows = %d, want 4", count)
	}
	for _, value := range []string{
		"<th>Name</th>", "<th>Email</th>", "<th>Team</th>", "<th>Plan</th>", "<th>Status</th>",
		"Alice", "alice@example.com", "Bob", "bob@example.com", "Platform", "Pro", "Active",
		`href="/admin/users/user-alice"`, `href="/admin/users/user-alice/edit"`,
	} {
		assertContains(t, body, value)
	}
}

func TestListQueryCapabilities(t *testing.T) {
	handler, _ := fixtureHandler(t)
	tests := []struct {
		name    string
		query   string
		present []string
		absent  []string
	}{
		{name: "search name", query: "q=alice", present: []string{"alice@example.com"}, absent: []string{"bob@example.com"}},
		{name: "search email", query: "q=bob%40example.com", present: []string{"bob@example.com"}, absent: []string{"alice@example.com"}},
		{name: "filter team", query: "team=team-support", present: []string{"bob@example.com"}, absent: []string{"alice@example.com"}},
		{name: "filter plan", query: "plan=Enterprise", present: []string{"sam.first@example.com"}, absent: []string{"alice@example.com"}},
		{name: "filter status", query: "status=Suspended", present: []string{"sam.second@example.com"}, absent: []string{"bob@example.com"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := perform(handler, http.MethodGet, "/admin/users?"+test.query, nil, true)
			assertStatus(t, response, http.StatusOK)
			body := response.Body.String()
			for _, value := range test.present {
				assertContains(t, body, value)
			}
			for _, value := range test.absent {
				assertNotContains(t, body, value)
			}
		})
	}
	response := perform(handler, http.MethodGet, "/admin/users", nil, true)
	body := response.Body.String()
	ordered := []string{"alice@example.com", "bob@example.com", "sam.first@example.com", "sam.second@example.com"}
	last := -1
	for _, value := range ordered {
		index := strings.Index(body, value)
		if index <= last {
			t.Fatalf("stable name order not preserved for %q in %s", value, body)
		}
		last = index
	}
}

func TestListPagination(t *testing.T) {
	teams := []domain.Team{{ID: "team-platform", Name: "Platform"}}
	users := make([]domain.User, 0, 21)
	for index := 1; index <= 21; index++ {
		users = append(users, domain.User{
			ID: fmt.Sprintf("user-%02d", index), Name: fmt.Sprintf("User %02d", index),
			Email: fmt.Sprintf("user%02d@example.com", index), TeamID: "team-platform",
			Plan: domain.PlanFree, Status: domain.StatusPending,
		})
	}
	handler := newHandler(t, store.NewMemory(teams, users))
	first := perform(handler, http.MethodGet, "/admin/users", nil, true)
	assertStatus(t, first, http.StatusOK)
	if count := strings.Count(first.Body.String(), "data-user-row="); count != 20 {
		t.Fatalf("first page rows = %d, want 20", count)
	}
	assertContains(t, first.Body.String(), `rel="next"`)
	second := perform(handler, http.MethodGet, "/admin/users?page=2", nil, true)
	assertStatus(t, second, http.StatusOK)
	if count := strings.Count(second.Body.String(), "data-user-row="); count != 1 {
		t.Fatalf("second page rows = %d, want 1", count)
	}
	assertContains(t, second.Body.String(), `rel="prev"`)
}

func TestListEmptyAndFailure(t *testing.T) {
	emptyRepository := store.NewMemory(nil, nil)
	empty := perform(newHandler(t, emptyRepository), http.MethodGet, "/admin/users", nil, true)
	assertStatus(t, empty, http.StatusOK)
	assertContains(t, empty.Body.String(), `data-state="empty"`)

	handler, repository := fixtureHandler(t)
	repository.FailNext()
	failure := perform(handler, http.MethodGet, "/admin/users", nil, true)
	assertStatus(t, failure, http.StatusInternalServerError)
	assertContains(t, failure.Body.String(), `data-state="failure"`)
}

func TestUserDetail(t *testing.T) {
	handler, _ := fixtureHandler(t)
	response := perform(handler, http.MethodGet, "/admin/users/user-alice", nil, true)
	assertStatus(t, response, http.StatusOK)
	body := response.Body.String()
	for _, value := range []string{
		"<dt>Name</dt><dd>Alice</dd>", "<dt>Email</dt><dd>alice@example.com</dd>",
		"<dt>Team</dt><dd>Platform</dd>", "<dt>Plan</dt><dd>Pro</dd>",
		"<dt>Status</dt><dd>Active</dd>", `href="/admin/users/user-alice/edit"`,
	} {
		assertContains(t, body, value)
	}
}

func TestDetailEmptyAndFailure(t *testing.T) {
	handler, repository := fixtureHandler(t)
	empty := perform(handler, http.MethodGet, "/admin/users/missing", nil, true)
	assertStatus(t, empty, http.StatusNotFound)
	assertContains(t, empty.Body.String(), `data-state="empty"`)
	repository.FailNext()
	failure := perform(handler, http.MethodGet, "/admin/users/user-alice", nil, true)
	assertStatus(t, failure, http.StatusInternalServerError)
	assertContains(t, failure.Body.String(), `data-state="failure"`)
}

func TestRelationUsesTeamLabel(t *testing.T) {
	handler, _ := fixtureHandler(t)
	for _, path := range []string{"/admin/users", "/admin/users/user-alice", "/admin/users/user-alice/edit"} {
		response := perform(handler, http.MethodGet, path, nil, true)
		assertStatus(t, response, http.StatusOK)
		assertContains(t, response.Body.String(), "Platform")
	}
}

func TestEditUser(t *testing.T) {
	handler, repository := fixtureHandler(t)
	formPage := perform(handler, http.MethodGet, "/admin/users/user-alice/edit", nil, true)
	assertStatus(t, formPage, http.StatusOK)
	body := formPage.Body.String()
	for _, value := range []string{`name="name"`, `name="email"`, `name="team"`, `name="plan"`} {
		assertContains(t, body, value)
	}
	token := submissionToken(t, body)
	values := url.Values{
		"submission": {token}, "name": {"Alice Updated"}, "email": {"alice.updated@example.com"},
		"team": {"team-support"}, "plan": {"Enterprise"},
	}
	response := perform(handler, http.MethodPost, "/admin/users/user-alice/edit", values, true)
	assertStatus(t, response, http.StatusSeeOther)
	if location := response.Header().Get("Location"); location != "/admin/users/user-alice" {
		t.Fatalf("location = %q", location)
	}
	updated, ok, err := repository.FindUser(t.Context(), "user-alice")
	if err != nil || !ok {
		t.Fatalf("updated user = %#v, %v, %v", updated, ok, err)
	}
	if updated.Name != "Alice Updated" || updated.Email != "alice.updated@example.com" || updated.TeamID != "team-support" || updated.Plan != domain.PlanEnterprise || updated.Status != domain.StatusActive {
		t.Fatalf("updated user = %#v", updated)
	}
	detail := perform(handler, http.MethodGet, response.Header().Get("Location"), nil, true)
	assertContains(t, detail.Body.String(), "Alice Updated")
}

func TestEditValidationAndFailure(t *testing.T) {
	tests := []struct {
		name       string
		change     func(url.Values)
		errorField string
		preserved  []string
	}{
		{name: "name required", change: func(values url.Values) { values.Set("name", "") }, errorField: "name", preserved: []string{"preserved@example.com", `value="team-support" selected`, `value="Enterprise" selected`}},
		{name: "email required", change: func(values url.Values) { values.Set("email", "") }, errorField: "email", preserved: []string{`value="Preserved Name"`, `value="team-support" selected`, `value="Enterprise" selected`}},
		{name: "email matches", change: func(values url.Values) { values.Set("email", "invalid") }, errorField: "email", preserved: []string{`value="Preserved Name"`, `value="team-support" selected`, `value="Enterprise" selected`}},
		{name: "plan required", change: func(values url.Values) { values.Set("plan", "") }, errorField: "plan", preserved: []string{`value="Preserved Name"`, "preserved@example.com", `value="team-support" selected`}},
		{name: "plan closed set", change: func(values url.Values) { values.Set("plan", "Unknown") }, errorField: "plan", preserved: []string{`value="Preserved Name"`, "preserved@example.com", `value="team-support" selected`}},
		{name: "email unique", change: func(values url.Values) { values.Set("email", "bob@example.com") }, errorField: "email", preserved: []string{`value="Preserved Name"`, "bob@example.com", `value="team-support" selected`, `value="Enterprise" selected`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, repository := fixtureHandler(t)
			formPage := perform(handler, http.MethodGet, "/admin/users/user-alice/edit", nil, true)
			values := validEditValues(submissionToken(t, formPage.Body.String()))
			test.change(values)
			response := perform(handler, http.MethodPost, "/admin/users/user-alice/edit", values, true)
			assertStatus(t, response, http.StatusUnprocessableEntity)
			body := response.Body.String()
			assertContains(t, body, `data-error="`+test.errorField+`"`)
			for _, value := range test.preserved {
				assertContains(t, body, value)
			}
			stored, ok, err := repository.FindUser(t.Context(), "user-alice")
			if err != nil || !ok || stored.Name != "Alice" || stored.Email != "alice@example.com" {
				t.Fatalf("validation changed stored user: %#v, %v, %v", stored, ok, err)
			}
		})
	}

	handler, repository := fixtureHandler(t)
	formPage := perform(handler, http.MethodGet, "/admin/users/user-alice/edit", nil, true)
	values := validEditValues(submissionToken(t, formPage.Body.String()))
	repository.FailNext()
	failure := perform(handler, http.MethodPost, "/admin/users/user-alice/edit", values, true)
	assertStatus(t, failure, http.StatusInternalServerError)
	assertContains(t, failure.Body.String(), `data-state="failure"`)
	assertContains(t, failure.Body.String(), `value="Preserved Name"`)
}

func TestEditIsAppliedAtMostOnce(t *testing.T) {
	handler, repository := fixtureHandler(t)
	formPage := perform(handler, http.MethodGet, "/admin/users/user-alice/edit", nil, true)
	values := validEditValues(submissionToken(t, formPage.Body.String()))
	first := perform(handler, http.MethodPost, "/admin/users/user-alice/edit", values, true)
	second := perform(handler, http.MethodPost, "/admin/users/user-alice/edit", values, true)
	assertStatus(t, first, http.StatusSeeOther)
	assertStatus(t, second, http.StatusSeeOther)
	if count := repository.MutationCount("user-alice"); count != 1 {
		t.Fatalf("applied mutations = %d, want 1", count)
	}
}

func fixtureHandler(t *testing.T) (http.Handler, *store.Memory) {
	t.Helper()
	repository := store.NewMemory(
		[]domain.Team{{ID: "team-platform", Name: "Platform"}, {ID: "team-support", Name: "Support"}},
		[]domain.User{
			{ID: "user-alice", Name: "Alice", Email: "alice@example.com", TeamID: "team-platform", Plan: domain.PlanPro, Status: domain.StatusActive},
			{ID: "user-bob", Name: "Bob", Email: "bob@example.com", TeamID: "team-support", Plan: domain.PlanFree, Status: domain.StatusPending},
			{ID: "user-sam-1", Name: "Sam", Email: "sam.first@example.com", TeamID: "team-platform", Plan: domain.PlanEnterprise, Status: domain.StatusConfirmed},
			{ID: "user-sam-2", Name: "Sam", Email: "sam.second@example.com", TeamID: "team-platform", Plan: domain.PlanFree, Status: domain.StatusSuspended},
		},
	)
	return newHandler(t, repository), repository
}

func newHandler(t *testing.T, repository store.Repository) http.Handler {
	t.Helper()
	server, err := New(repository)
	if err != nil {
		t.Fatal(err)
	}
	return server.Handler()
}

func perform(handler http.Handler, method, target string, values url.Values, admin bool) *httptest.ResponseRecorder {
	var body *strings.Reader
	if values == nil {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(values.Encode())
	}
	request := httptest.NewRequest(method, target, body)
	if values != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if admin {
		request.Header.Set("X-Principal", "admin-one")
		request.Header.Set("X-Role", "admin")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func validEditValues(token string) url.Values {
	return url.Values{
		"submission": {token}, "name": {"Preserved Name"}, "email": {"preserved@example.com"},
		"team": {"team-support"}, "plan": {"Enterprise"},
	}
}

var submissionPattern = regexp.MustCompile(`name="submission" value="([^"]+)"`)

func submissionToken(t *testing.T, body string) string {
	t.Helper()
	match := submissionPattern.FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("submission token missing from %s", body)
	}
	return match[1]
}

func assertStatus(t *testing.T, response *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if response.Code != expected {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, expected, response.Body.String())
	}
}

func assertContains(t *testing.T, body, value string) {
	t.Helper()
	if !strings.Contains(body, value) {
		t.Fatalf("body does not contain %q: %s", value, body)
	}
}

func assertNotContains(t *testing.T, body, value string) {
	t.Helper()
	if strings.Contains(body, value) {
		t.Fatalf("body unexpectedly contains %q: %s", value, body)
	}
}
