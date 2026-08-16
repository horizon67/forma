package main

import (
	"log"
	"net/http"

	"example.com/forma-admin-target/internal/domain"
	"example.com/forma-admin-target/internal/store"
	"example.com/forma-admin-target/internal/web"
)

func main() {
	repository := store.NewMemory(
		[]domain.Team{{ID: "team-platform", Name: "Platform"}, {ID: "team-support", Name: "Support"}},
		[]domain.User{
			{ID: "user-alice", Name: "Alice", Email: "alice@example.com", TeamID: "team-platform", Plan: domain.PlanPro, Status: domain.StatusActive},
			{ID: "user-bob", Name: "Bob", Email: "bob@example.com", TeamID: "team-support", Plan: domain.PlanFree, Status: domain.StatusPending},
		},
	)
	server, err := web.New(repository)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("admin target listening on http://localhost:8080/admin/users")
	log.Fatal(http.ListenAndServe(":8080", server.Handler()))
}
