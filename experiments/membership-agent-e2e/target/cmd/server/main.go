package main

import (
	"log"
	"net/http"

	"example.com/forma-admin-target/internal/domain"
	"example.com/forma-admin-target/internal/store"
	"example.com/forma-admin-target/internal/web"
)

func main() {
	handler, err := buildHandler()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("target listening on http://localhost:8080/admin/users and /members/signup")
	log.Fatal(http.ListenAndServe(":8080", handler))
}

// buildHandler assembles exactly what the executable serves. Tests call this
// function so a change here cannot leave a surface unreachable while the suite
// stays green.
func buildHandler() (http.Handler, error) {
	repository := store.NewMemory(
		[]domain.Team{{ID: "team-platform", Name: "Platform"}, {ID: "team-support", Name: "Support"}},
		[]domain.User{
			{ID: "user-alice", Name: "Alice", Email: "alice@example.com", TeamID: "team-platform", Plan: domain.PlanPro, Status: domain.StatusActive},
			{ID: "user-bob", Name: "Bob", Email: "bob@example.com", TeamID: "team-support", Plan: domain.PlanFree, Status: domain.StatusPending},
		},
	)
	server, err := web.NewWithMembership(repository, web.MembershipOptions{Identity: repository})
	if err != nil {
		return nil, err
	}
	return server.Handler(), nil
}
