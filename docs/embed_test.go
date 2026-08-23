package formadocs

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/compiler"
)

func TestAuthoringContextContainsTheEmbeddedGuideExampleAndVersion(t *testing.T) {
	got := AuthoringContext("v0.1.0-alpha.1")
	for _, want := range []string{
		"Context schema: `" + AuthoringContextSchema + "`",
		"Forma binary: `v0.1.0-alpha.1`",
		"Language profile: `" + AlphaLanguageProfile + "`",
		strings.TrimSpace(languageGuide),
		"```forma\n" + strings.TrimSpace(alphaQuickstart) + "\n```",
		"```forma\n" + strings.TrimSpace(emailVerifiedMembership) + "\n```",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("authoring context does not contain %q", want)
		}
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatal("authoring context must end with one newline")
	}
	if got != AuthoringContext("v0.1.0-alpha.1") {
		t.Fatal("authoring context is not deterministic for one binary version")
	}
	if strings.Contains(got, "https://") || strings.Contains(got, "http://") {
		t.Fatal("authoring context must not require a live web document")
	}
	if link := regexp.MustCompile(`\[[^]]+\]\([^)]*\.md(?:#[^)]*)?\)`).FindString(got); link != "" {
		t.Fatalf("authoring context references a document that it does not bundle: %s", link)
	}

	profile, err := os.ReadFile("alpha-language-profile.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(profile), "# Forma `"+AlphaLanguageProfile+"` Language Profile") {
		t.Fatalf("embedded profile version %q differs from alpha-language-profile.md", AlphaLanguageProfile)
	}
}

func TestAuthoringGuideDocumentsRequiredBlockNewlines(t *testing.T) {
	const rule = "A declaration body opens with `{` followed by a newline, and each member is written on its own line."
	if !strings.Contains(strings.Join(strings.Fields(languageGuide), " "), rule) {
		t.Fatalf("authoring guide does not document the block-newline rule")
	}

	result := compiler.Compile([]compiler.SourceFile{
		compiler.NewSourceFile("compressed.forma", "entity Tag { name String required label }\n"),
	})
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "F1002" && strings.Contains(diagnostic.Message, "expected a newline after `{`") {
			return
		}
	}
	t.Fatalf("one-line declaration body diagnostics: %#v", result.Diagnostics)
}

func TestBundledMembershipExampleMatchesThePublicExample(t *testing.T) {
	public, err := os.ReadFile(filepath.Join("..", "examples", "email-verified-membership.forma"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := EmailVerifiedMembership(), strings.TrimSpace(string(public))+"\n"; got != want {
		t.Fatal("bundled membership example differs from examples/email-verified-membership.forma")
	}

	result := compiler.Compile([]compiler.SourceFile{
		compiler.NewSourceFile("email-verified-membership.forma", EmailVerifiedMembership()),
	})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("bundled membership example diagnostics: %#v", result.Diagnostics)
	}
}

func TestBundledAlphaQuickstartCompilesAndDemonstratesGuide(t *testing.T) {
	quickstart := AlphaQuickstart()
	result := compiler.Compile([]compiler.SourceFile{
		compiler.NewSourceFile("alpha-quickstart.forma", quickstart),
	})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("bundled alpha quickstart diagnostics: %#v", result.Diagnostics)
	}
	if result.Intent == nil {
		t.Fatalf("bundled alpha quickstart intent = %#v", result.Intent)
	}

	for _, demonstration := range []string{
		"role manager",
		"entry Welcome",
		"type ProjectCode = String matches",
		"required unique label",
		"tasks [Task]",
		"state status Planned | Active | Done initial Planned",
		"action Task.complete:",
		"confirm allow manager, member",
		"continue Projects",
		"search code, name",
		"filter project, assignee, status",
		"sort code asc",
		"paginate 20",
		"actions create, view, edit, delete",
		"submit create",
		"submit edit",
	} {
		if !strings.Contains(quickstart, demonstration) {
			t.Errorf("bundled alpha quickstart does not demonstrate %q", demonstration)
		}
	}
}
