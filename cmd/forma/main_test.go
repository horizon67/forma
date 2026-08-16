package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"resolve", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	var intent struct {
		Version string `json:"version"`
		Pages   []any  `json:"pages"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &intent); err != nil {
		t.Fatalf("resolve output is not JSON: %v\n%s", err, stdout.String())
	}
	if intent.Version != "forma/resolved-intent/v0.4" || len(intent.Pages) == 0 {
		t.Fatalf("resolved intent = %#v", intent)
	}
}

func TestCheckCommand(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "users.forma")
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if got := stdout.String(); got != "checked 1 file: no errors\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestCheckCommandRendersDiagnostic(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "invalid.forma")
	source := "entity User {\n    name String\n}\npage Users {\n    list User {\n        columns missing\n    }\n}\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", path}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("exit code %d; stderr:\n%s", exitCode, stderr.String())
	}
	for _, expected := range []string{"error[F2402]", "columns missing", "help:", "forma check failed with 1 error"} {
		if !strings.Contains(stderr.String(), expected) {
			t.Fatalf("stderr does not contain %q:\n%s", expected, stderr.String())
		}
	}
}

func TestCheckCommandAcceptsSelfOnlyInvariant(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "stock.forma")
	source := `type Quantity = Int min 0
entity StockItem {
    onHand Quantity required
    reserved Quantity required
    invariant stockAvailable: reserved <= onHand
}
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", path}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code %d\nstderr:\n%s", exitCode, stderr.String())
	}
	if got := stdout.String(); got != "checked 1 file: no errors\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestCheckRequiresAnExplicitCompilationUnit(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("exit code %d; stderr:\n%s", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no source files or directories specified") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestDirectoryArgumentIsOneCompilationUnit(t *testing.T) {
	directory := t.TempDir()
	first := filepath.Join(directory, "first.forma")
	nested := filepath.Join(directory, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(nested, "second.forma")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("role admin\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, path := range []string{first, second} {
		var stdout, stderr bytes.Buffer
		if exitCode := run([]string{"check", path}, &stdout, &stderr); exitCode != 0 {
			t.Fatalf("independent unit %s failed with %d:\n%s", path, exitCode, stderr.String())
		}
	}

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", directory}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("combined directory exit code %d; stderr:\n%s", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error[F2001]: duplicate role `admin`") {
		t.Fatalf("directory was not compiled as one unit:\n%s", stderr.String())
	}
}
