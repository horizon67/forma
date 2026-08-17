package store

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	"example.com/forma-admin-target/internal/domain"
	"example.com/forma-admin-target/internal/identity"
)

var (
	ErrUnavailable    = errors.New("store unavailable")
	ErrDuplicateEmail = errors.New("email already exists")
)

type UserQuery struct {
	Search   string
	TeamID   string
	Plan     domain.Plan
	Status   domain.Status
	Page     int
	PageSize int
}

type UserPage struct {
	Users   []domain.User
	Total   int
	Page    int
	HasNext bool
}

type Repository interface {
	ListUsers(context.Context, UserQuery) (UserPage, error)
	FindUser(context.Context, string) (domain.User, bool, error)
	ListTeams(context.Context) ([]domain.Team, error)
	FindTeam(context.Context, string) (domain.Team, bool, error)
	EmailExists(context.Context, string, string) (bool, error)
	UpdateUser(context.Context, domain.User) error
}

type Memory struct {
	mu            sync.RWMutex
	users         map[string]domain.User
	order         []string
	teams         map[string]domain.Team
	teamOrder     []string
	failures      int
	mutationCount map[string]int

	credentials     map[string]identity.Credential
	evidence        map[string]identity.Evidence
	evidenceOrder   []string
	evidenceByToken map[string]string
	emissions       map[string]identity.Emission
	emissionOrder   []string
	sessions        map[string]identity.Session
	sequence        int
}

func NewMemory(teams []domain.Team, users []domain.User) *Memory {
	repository := &Memory{
		users: map[string]domain.User{}, teams: map[string]domain.Team{}, mutationCount: map[string]int{},
		credentials: map[string]identity.Credential{}, evidence: map[string]identity.Evidence{},
		evidenceByToken: map[string]string{}, emissions: map[string]identity.Emission{},
		sessions: map[string]identity.Session{},
	}
	for _, team := range teams {
		repository.teams[team.ID] = team
		repository.teamOrder = append(repository.teamOrder, team.ID)
	}
	for _, user := range users {
		repository.users[user.ID] = user
		repository.order = append(repository.order, user.ID)
	}
	return repository
}

func (repository *Memory) FailNext() {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.failures++
}

func (repository *Memory) MutationCount(userID string) int {
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	return repository.mutationCount[userID]
}

func (repository *Memory) ListUsers(_ context.Context, query UserQuery) (UserPage, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.consumeFailure() {
		return UserPage{}, ErrUnavailable
	}
	users := make([]domain.User, 0, len(repository.order))
	search := strings.ToLower(strings.TrimSpace(query.Search))
	for _, id := range repository.order {
		user := repository.users[id]
		if search != "" && !strings.Contains(strings.ToLower(user.Name), search) &&
			!strings.Contains(strings.ToLower(user.Nickname), search) && !strings.Contains(strings.ToLower(user.Email), search) {
			continue
		}
		if query.TeamID != "" && user.TeamID != query.TeamID {
			continue
		}
		if query.Plan != "" && user.Plan != query.Plan {
			continue
		}
		if query.Status != "" && user.Status != query.Status {
			continue
		}
		users = append(users, user)
	}
	sort.SliceStable(users, func(i, j int) bool {
		return strings.ToLower(users[i].Name) < strings.ToLower(users[j].Name)
	})
	total := len(users)
	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return UserPage{Users: append([]domain.User(nil), users[start:end]...), Total: total, Page: page, HasNext: end < total}, nil
}

func (repository *Memory) FindUser(_ context.Context, id string) (domain.User, bool, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.consumeFailure() {
		return domain.User{}, false, ErrUnavailable
	}
	user, ok := repository.users[id]
	return user, ok, nil
}

func (repository *Memory) ListTeams(_ context.Context) ([]domain.Team, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.consumeFailure() {
		return nil, ErrUnavailable
	}
	teams := make([]domain.Team, 0, len(repository.teamOrder))
	for _, id := range repository.teamOrder {
		teams = append(teams, repository.teams[id])
	}
	return teams, nil
}

func (repository *Memory) FindTeam(_ context.Context, id string) (domain.Team, bool, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.consumeFailure() {
		return domain.Team{}, false, ErrUnavailable
	}
	team, ok := repository.teams[id]
	return team, ok, nil
}

func (repository *Memory) EmailExists(_ context.Context, email, excludingID string) (bool, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.consumeFailure() {
		return false, ErrUnavailable
	}
	for id, user := range repository.users {
		// Identity and the admin surface must agree on what one identifier is,
		// so both go through CanonicalIdentifier.
		if id != excludingID && CanonicalIdentifier(user.Email) == CanonicalIdentifier(email) {
			return true, nil
		}
	}
	return false, nil
}

func (repository *Memory) UpdateUser(_ context.Context, user domain.User) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.consumeFailure() {
		return ErrUnavailable
	}
	if _, ok := repository.users[user.ID]; !ok {
		return errors.New("user not found")
	}
	for id, existing := range repository.users {
		if id != user.ID && CanonicalIdentifier(existing.Email) == CanonicalIdentifier(user.Email) {
			return ErrDuplicateEmail
		}
	}
	repository.users[user.ID] = user
	repository.mutationCount[user.ID]++
	return nil
}

// consumeFailure is called with repository.mu held.
func (repository *Memory) consumeFailure() bool {
	if repository.failures == 0 {
		return false
	}
	repository.failures--
	return true
}
