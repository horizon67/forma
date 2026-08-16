package web

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"example.com/forma-admin-target/internal/auth"
	"example.com/forma-admin-target/internal/domain"
	"example.com/forma-admin-target/internal/store"
)

const pageSize = 20

type Server struct {
	repository  store.Repository
	templates   *template.Template
	submissions *submissionGuard
}

func New(repository store.Repository) (*Server, error) {
	templates, err := template.New("pages").Funcs(template.FuncMap{"eq": func(left, right any) bool {
		return fmt.Sprint(left) == fmt.Sprint(right)
	}}).Parse(pageTemplates)
	if err != nil {
		return nil, err
	}
	return &Server{repository: repository, templates: templates, submissions: newSubmissionGuard(512)}, nil
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/users", server.listUsers)
	mux.HandleFunc("GET /admin/users/{id}", server.userDetail)
	mux.HandleFunc("GET /admin/users/{id}/edit", server.editUser)
	mux.HandleFunc("POST /admin/users/{id}/edit", server.updateUser)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal := auth.FromRequest(request)
		if !principal.HasRole("admin") {
			http.Error(writer, "forbidden", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(writer, request)
	})
}

type presentedUser struct {
	ID       string
	Name     string
	Email    string
	TeamName string
	Plan     domain.Plan
	Status   domain.Status
}

type listData struct {
	Users    []presentedUser
	Teams    []domain.Team
	Plans    []domain.Plan
	Statuses []domain.Status
	Search   string
	Team     string
	Plan     domain.Plan
	Status   domain.Status
	Page     int
	Previous string
	Next     string
	Empty    bool
	Failure  string
	Total    int
}

func (server *Server) listUsers(writer http.ResponseWriter, request *http.Request) {
	page, _ := strconv.Atoi(request.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	query := store.UserQuery{
		Search: request.URL.Query().Get("q"), TeamID: request.URL.Query().Get("team"),
		Plan: domain.Plan(request.URL.Query().Get("plan")), Status: domain.Status(request.URL.Query().Get("status")),
		Page: page, PageSize: pageSize,
	}
	result, err := server.repository.ListUsers(request.Context(), query)
	if err != nil {
		server.render(writer, http.StatusInternalServerError, "list", listData{Failure: "Users could not be loaded."})
		return
	}
	teams, err := server.repository.ListTeams(request.Context())
	if err != nil {
		server.render(writer, http.StatusInternalServerError, "list", listData{Failure: "Users could not be loaded."})
		return
	}
	labels := map[string]string{}
	for _, team := range teams {
		labels[team.ID] = team.Name
	}
	presented := make([]presentedUser, 0, len(result.Users))
	for _, user := range result.Users {
		presented = append(presented, presentUser(user, labels[user.TeamID]))
	}
	data := listData{
		Users: presented, Teams: teams, Plans: domain.Plans, Statuses: domain.Statuses,
		Search: query.Search, Team: query.TeamID, Plan: query.Plan, Status: query.Status,
		Page: result.Page, Empty: len(result.Users) == 0, Total: result.Total,
	}
	if result.Page > 1 {
		data.Previous = listPageURL(request.URL.Query(), result.Page-1)
	}
	if result.HasNext {
		data.Next = listPageURL(request.URL.Query(), result.Page+1)
	}
	server.render(writer, http.StatusOK, "list", data)
}

type detailData struct {
	User    presentedUser
	Empty   bool
	Failure string
}

func (server *Server) userDetail(writer http.ResponseWriter, request *http.Request) {
	user, ok, err := server.repository.FindUser(request.Context(), request.PathValue("id"))
	if err != nil {
		server.render(writer, http.StatusInternalServerError, "detail", detailData{Failure: "User could not be loaded."})
		return
	}
	if !ok {
		server.render(writer, http.StatusNotFound, "detail", detailData{Empty: true})
		return
	}
	teamName := ""
	if user.TeamID != "" {
		team, exists, teamErr := server.repository.FindTeam(request.Context(), user.TeamID)
		if teamErr != nil {
			server.render(writer, http.StatusInternalServerError, "detail", detailData{Failure: "User could not be loaded."})
			return
		}
		if exists {
			teamName = team.Name
		}
	}
	server.render(writer, http.StatusOK, "detail", detailData{User: presentUser(user, teamName)})
}

type formValues struct {
	Name   string
	Email  string
	TeamID string
	Plan   domain.Plan
}

type formData struct {
	UserID  string
	Form    formValues
	Teams   []domain.Team
	Plans   []domain.Plan
	Errors  map[string]string
	Token   string
	Failure string
}

func (server *Server) editUser(writer http.ResponseWriter, request *http.Request) {
	user, ok, err := server.repository.FindUser(request.Context(), request.PathValue("id"))
	if err != nil {
		server.render(writer, http.StatusInternalServerError, "edit", formData{Failure: "User could not be loaded."})
		return
	}
	if !ok {
		http.NotFound(writer, request)
		return
	}
	server.renderEdit(writer, request, http.StatusOK, user.ID, formValuesFromUser(user), nil, "")
}

func (server *Server) updateUser(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil {
		http.Error(writer, "invalid form", http.StatusBadRequest)
		return
	}
	userID := request.PathValue("id")
	values := formValues{
		Name: strings.TrimSpace(request.FormValue("name")), Email: strings.TrimSpace(request.FormValue("email")),
		TeamID: request.FormValue("team"), Plan: domain.Plan(request.FormValue("plan")),
	}
	user, ok, err := server.repository.FindUser(request.Context(), userID)
	if err != nil {
		server.renderEdit(writer, request, http.StatusInternalServerError, userID, values, nil, "User could not be saved.")
		return
	}
	if !ok {
		http.NotFound(writer, request)
		return
	}
	errorsByField, validationErr := server.validateUser(request, userID, values)
	if validationErr != nil {
		server.renderEdit(writer, request, http.StatusInternalServerError, userID, values, nil, "User could not be saved.")
		return
	}
	if len(errorsByField) != 0 {
		server.renderEdit(writer, request, http.StatusUnprocessableEntity, userID, values, errorsByField, "")
		return
	}
	principal := auth.FromRequest(request)
	scope := principal.ID + "\x00" + userID
	claim, location := server.submissions.begin(request.FormValue("submission"), scope)
	switch claim {
	case submissionCompleted:
		http.Redirect(writer, request, location, http.StatusSeeOther)
		return
	case submissionRejected:
		server.renderEdit(writer, request, http.StatusConflict, userID, values, map[string]string{"submission": "This form expired. Review the values and submit again."}, "")
		return
	case submissionProcessing:
		server.renderEdit(writer, request, http.StatusConflict, userID, values, map[string]string{"submission": "This update is already being processed."}, "")
		return
	}
	user.Name = values.Name
	user.Email = values.Email
	user.TeamID = values.TeamID
	user.Plan = values.Plan
	if err := server.repository.UpdateUser(request.Context(), user); err != nil {
		server.submissions.fail(request.FormValue("submission"), scope)
		if errors.Is(err, store.ErrDuplicateEmail) {
			server.renderEdit(writer, request, http.StatusUnprocessableEntity, userID, values, map[string]string{"email": "Email must be unique."}, "")
			return
		}
		server.renderEdit(writer, request, http.StatusInternalServerError, userID, values, nil, "User could not be saved.")
		return
	}
	location = "/admin/users/" + url.PathEscape(userID)
	server.submissions.complete(request.FormValue("submission"), scope, location)
	http.Redirect(writer, request, location, http.StatusSeeOther)
}

func (server *Server) validateUser(request *http.Request, userID string, values formValues) (map[string]string, error) {
	errorsByField := map[string]string{}
	if values.Name == "" {
		errorsByField["name"] = "Name is required."
	}
	if values.Email == "" {
		errorsByField["email"] = "Email is required."
	} else if at := strings.IndexByte(values.Email, '@'); at < 1 || at == len(values.Email)-1 {
		errorsByField["email"] = "Email must contain text on both sides of @."
	}
	if values.Plan == "" {
		errorsByField["plan"] = "Plan is required."
	} else if !domain.ValidPlan(values.Plan) {
		errorsByField["plan"] = "Plan must be Free, Pro, or Enterprise."
	}
	if values.TeamID != "" {
		_, exists, err := server.repository.FindTeam(request.Context(), values.TeamID)
		if err != nil {
			return nil, err
		}
		if !exists {
			errorsByField["team"] = "Team does not exist."
		}
	}
	if values.Email != "" {
		exists, err := server.repository.EmailExists(request.Context(), values.Email, userID)
		if err != nil {
			return nil, err
		}
		if exists {
			errorsByField["email"] = "Email must be unique."
		}
	}
	return errorsByField, nil
}

func (server *Server) renderEdit(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	userID string,
	values formValues,
	errorsByField map[string]string,
	failure string,
) {
	teams, err := server.repository.ListTeams(request.Context())
	if err != nil {
		server.render(writer, http.StatusInternalServerError, "edit", formData{UserID: userID, Form: values, Failure: "User could not be saved."})
		return
	}
	principal := auth.FromRequest(request)
	token, err := server.submissions.issue(principal.ID + "\x00" + userID)
	if err != nil {
		server.render(writer, http.StatusInternalServerError, "edit", formData{UserID: userID, Form: values, Teams: teams, Failure: "User could not be saved."})
		return
	}
	server.render(writer, status, "edit", formData{
		UserID: userID, Form: values, Teams: teams, Plans: domain.Plans,
		Errors: errorsByField, Token: token, Failure: failure,
	})
}

func (server *Server) render(writer http.ResponseWriter, status int, name string, data any) {
	var output bytes.Buffer
	if err := server.templates.ExecuteTemplate(&output, name, data); err != nil {
		http.Error(writer, "render failed", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(status)
	_, _ = writer.Write(output.Bytes())
}

func presentUser(user domain.User, teamName string) presentedUser {
	return presentedUser{ID: user.ID, Name: user.Name, Email: user.Email, TeamName: teamName, Plan: user.Plan, Status: user.Status}
}

func formValuesFromUser(user domain.User) formValues {
	return formValues{Name: user.Name, Email: user.Email, TeamID: user.TeamID, Plan: user.Plan}
}

func listPageURL(values url.Values, page int) string {
	copy := url.Values{}
	for key, entries := range values {
		copy[key] = append([]string(nil), entries...)
	}
	copy.Set("page", strconv.Itoa(page))
	return "/admin/users?" + copy.Encode()
}

type submissionClaim int

const (
	submissionAccepted submissionClaim = iota
	submissionCompleted
	submissionProcessing
	submissionRejected
)

type completedSubmission struct {
	scope    string
	location string
}

type submissionGuard struct {
	mu         sync.Mutex
	limit      int
	order      []string
	issued     map[string]string
	processing map[string]string
	completed  map[string]completedSubmission
}

func newSubmissionGuard(limit int) *submissionGuard {
	return &submissionGuard{
		limit: limit, issued: map[string]string{}, processing: map[string]string{}, completed: map[string]completedSubmission{},
	}
}

func (guard *submissionGuard) issue(scope string) (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buffer)
	guard.mu.Lock()
	defer guard.mu.Unlock()
	guard.issued[token] = scope
	guard.order = append(guard.order, token)
	for len(guard.order) > guard.limit {
		oldest := guard.order[0]
		guard.order = guard.order[1:]
		delete(guard.issued, oldest)
		delete(guard.processing, oldest)
		delete(guard.completed, oldest)
	}
	return token, nil
}

func (guard *submissionGuard) begin(token, scope string) (submissionClaim, string) {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	if completed, ok := guard.completed[token]; ok {
		if completed.scope == scope {
			return submissionCompleted, completed.location
		}
		return submissionRejected, ""
	}
	if processingScope, ok := guard.processing[token]; ok {
		if processingScope == scope {
			return submissionProcessing, ""
		}
		return submissionRejected, ""
	}
	if guard.issued[token] != scope || token == "" {
		return submissionRejected, ""
	}
	guard.processing[token] = scope
	return submissionAccepted, ""
}

func (guard *submissionGuard) fail(token, scope string) {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	if guard.processing[token] == scope {
		delete(guard.processing, token)
	}
}

func (guard *submissionGuard) complete(token, scope, location string) {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	if guard.processing[token] != scope {
		return
	}
	delete(guard.processing, token)
	delete(guard.issued, token)
	guard.completed[token] = completedSubmission{scope: scope, location: location}
}
