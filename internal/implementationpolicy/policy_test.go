package implementationpolicy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const manifestYAML = `schema: forma/implementation-policy/v0alpha1
policies:
  - id: implementation/server-rendering
    policy: required
    value: html/template
  - id: implementation/persistence
    policy: preferred
    value: database/sql
  - id: implementation/router
    policy: forbidden
    value: github.com/gorilla/mux
conventions:
  - keep internal packages
`

func TestParseYAMLNormalizesPolicies(t *testing.T) {
	manifest, err := ParseYAML([]byte(manifestYAML))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != Schema || len(manifest.Policies) != 3 {
		t.Fatalf("manifest = %#v", manifest)
	}
	if manifest.Policies[0].ID != "implementation/persistence" || manifest.Policies[2].ID != "implementation/server-rendering" {
		t.Fatalf("policies are not canonical: %#v", manifest.Policies)
	}
	if err := ValidateCanonical(manifest); err != nil {
		t.Fatal(err)
	}
}

func TestParseYAMLRejectsUnknownAndInvalidFields(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "unknown", content: manifestYAML + "unknown: true\n", want: "field unknown not found"},
		{name: "schema", content: strings.Replace(manifestYAML, Schema, "forma/implementation-policy/v9", 1), want: "schema"},
		{name: "mode", content: strings.Replace(manifestYAML, "policy: required", "policy: optional", 1), want: "unknown mode"},
		{name: "duplicate", content: strings.Replace(manifestYAML, "implementation/persistence", "implementation/server-rendering", 1), want: "duplicate policy"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseYAML([]byte(test.content)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateCoverageChecksRequiredPreferredAndForbidden(t *testing.T) {
	manifest, err := ParseYAML([]byte(manifestYAML))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeFile(t, root, "internal/web/server.go", "import \"html/template\"\n")
	coverage := []Coverage{
		{PolicyID: "implementation/server-rendering", Status: "satisfied", Evidence: []string{"internal/web/server.go"}},
		{PolicyID: "implementation/persistence", Status: "deviated", Reason: "the controlled target remains in memory"},
		{PolicyID: "implementation/router", Status: "satisfied"},
	}
	if err := ValidateCoverage(manifest, coverage, root); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCoverageRejectsSilentOrUnprovenClaims(t *testing.T) {
	manifest, err := ParseYAML([]byte(manifestYAML))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeFile(t, root, "internal/web/server.go", "import \"html/template\"\n")
	writeFile(t, root, "internal/web/plain.go", "package web\n")
	base := []Coverage{
		{PolicyID: "implementation/server-rendering", Status: "satisfied", Evidence: []string{"internal/web/server.go"}},
		{PolicyID: "implementation/persistence", Status: "deviated", Reason: "the controlled target remains in memory"},
		{PolicyID: "implementation/router", Status: "satisfied"},
	}
	tests := []struct {
		name   string
		mutate func([]Coverage) []Coverage
		want   string
	}{
		{name: "missing", mutate: func(items []Coverage) []Coverage { return items[1:] }, want: "policy implementation/server-rendering is missing"},
		{name: "unknown", mutate: func(items []Coverage) []Coverage { items[0].PolicyID = "implementation/unknown"; return items }, want: "unknown policy"},
		{name: "empty deviation", mutate: func(items []Coverage) []Coverage { items[1].Reason = ""; return items }, want: "requires a non-empty"},
		{name: "value absent", mutate: func(items []Coverage) []Coverage {
			items[0].Evidence = []string{"internal/web/plain.go"}
			return items
		}, want: "does not appear"},
		{name: "path escape", mutate: func(items []Coverage) []Coverage { items[0].Evidence = []string{"../server.go"}; return items }, want: "canonical repository-relative"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			items := append([]Coverage(nil), base...)
			if err := ValidateCoverage(manifest, test.mutate(items), root); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateCoverageReportsForbiddenScanHits(t *testing.T) {
	manifest, err := ParseYAML([]byte(manifestYAML))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeFile(t, root, "internal/web/server.go", "import \"html/template\"\n")
	writeFile(t, root, "go.mod", "require github.com/gorilla/mux v1.8.1\n")
	base := []Coverage{
		{PolicyID: "implementation/server-rendering", Status: "satisfied", Evidence: []string{"internal/web/server.go"}},
		{PolicyID: "implementation/persistence", Status: "deviated", Reason: "the controlled target remains in memory"},
		{PolicyID: "implementation/router", Status: "satisfied"},
	}
	if err := ValidateCoverage(manifest, base, root); err == nil || !strings.Contains(err.Error(), "must be flagged") {
		t.Fatalf("error = %v", err)
	}
	base[2] = Coverage{PolicyID: "implementation/router", Status: "flagged", Hits: []string{"go.mod"}}
	if err := ValidateCoverage(manifest, base, root); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, root, relative, content string) {
	t.Helper()
	filePath := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
