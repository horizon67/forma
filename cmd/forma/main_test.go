package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
