package auth

import (
	"net/http"
	"strings"
)

type Principal struct {
	ID    string
	Roles map[string]bool
}

func FromRequest(request *http.Request) Principal {
	id := request.Header.Get("X-Principal")
	if id == "" {
		id = "anonymous"
	}
	roles := map[string]bool{}
	roleValue := request.Header.Get("X-Role")
	if roleValue == "" {
		if cookie, err := request.Cookie("role"); err == nil {
			roleValue = cookie.Value
		}
	}
	for _, role := range strings.Split(roleValue, ",") {
		role = strings.TrimSpace(role)
		if role != "" {
			roles[role] = true
		}
	}
	return Principal{ID: id, Roles: roles}
}

func (principal Principal) HasRole(role string) bool {
	return principal.Roles[role]
}
