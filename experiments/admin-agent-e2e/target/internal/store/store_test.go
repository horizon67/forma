package store

import (
	"errors"
	"testing"

	"example.com/forma-admin-target/internal/domain"
)

func TestUpdateUserEnforcesUniqueEmail(t *testing.T) {
	repository := NewMemory(nil, []domain.User{
		{ID: "first", Name: "First", Email: "first@example.com", Plan: domain.PlanFree, Status: domain.StatusPending},
		{ID: "second", Name: "Second", Email: "second@example.com", Plan: domain.PlanPro, Status: domain.StatusActive},
	})
	second, ok, err := repository.FindUser(t.Context(), "second")
	if err != nil || !ok {
		t.Fatalf("find second = %#v, %v, %v", second, ok, err)
	}
	second.Email = "FIRST@example.com"
	if err := repository.UpdateUser(t.Context(), second); !errors.Is(err, ErrDuplicateEmail) {
		t.Fatalf("update error = %v, want ErrDuplicateEmail", err)
	}
	stored, ok, err := repository.FindUser(t.Context(), "second")
	if err != nil || !ok || stored.Email != "second@example.com" {
		t.Fatalf("duplicate update changed record: %#v, %v, %v", stored, ok, err)
	}
}
