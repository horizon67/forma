package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestExecutableServesBothSurfaces exercises the assembly the binary uses. If
// the executable stopped wiring membership, or stopped guarding the admin
// surface, this test fails rather than a private harness staying green.
func TestExecutableServesBothSurfaces(t *testing.T) {
	handler, err := buildHandler()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		target string
		want   int
	}{
		{target: "/members/signup", want: http.StatusOK},
		{target: "/members/signin", want: http.StatusOK},
		{target: "/members/check-email", want: http.StatusOK},
		// The admin surface stays behind the role gate for an anonymous visitor.
		{target: "/admin/users", want: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.target, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.target, nil))
			if recorder.Code != test.want {
				t.Fatalf("GET %s = %d, want %d", test.target, recorder.Code, test.want)
			}
		})
	}
}
