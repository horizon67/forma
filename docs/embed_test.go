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

func TestAlphaReleaseVersionMatchesPublicInstallationDocuments(t *testing.T) {
	releaseVersion := strings.TrimPrefix(AlphaLanguageProfile, "v")
	tests := []struct {
		name  string
		path  string
		wants []string
	}{
		{name: "profile", path: "alpha-language-profile.md", wants: []string{"# Forma `" + AlphaLanguageProfile + "` Language Profile"}},
		{name: "guide", path: "language-guide.md", wants: []string{"`" + AlphaLanguageProfile + "` reference front-end"}},
		{name: "reference", path: "language-reference.md", wants: []string{"`" + AlphaLanguageProfile + "` release candidate"}},
		{name: "cli", path: "cli.md", wants: []string{"`" + AlphaLanguageProfile + "` release candidate"}},
		{name: "quickstart", path: "quickstart.md", wants: []string{
			"workflow for `" + AlphaLanguageProfile + "`",
			"github.com/horizon67/forma/cmd/forma@" + AlphaLanguageProfile,
			"forma " + AlphaLanguageProfile,
			"horizon67/forma/" + AlphaLanguageProfile + "/docs/examples/alpha-quickstart.forma",
		}},
		{name: "security", path: "security.md", wants: []string{"`" + AlphaLanguageProfile + "` thin runner"}},
		{name: "install", path: "install.md", wants: []string{
			"`" + AlphaLanguageProfile + "` release candidate",
			"github.com/horizon67/forma/cmd/forma@" + AlphaLanguageProfile,
			"forma_" + releaseVersion + "_darwin_amd64.tar.gz",
			"forma_" + releaseVersion + "_darwin_arm64.tar.gz",
			"forma_" + releaseVersion + "_linux_amd64.tar.gz",
			"forma_" + releaseVersion + "_linux_arm64.tar.gz",
		}},
		{name: "readme", path: filepath.Join("..", "README.md"), wants: []string{
			"unstable `" + AlphaLanguageProfile + "` distribution",
			"github.com/horizon67/forma/cmd/forma@" + AlphaLanguageProfile,
		}},
		{name: "readme-ja", path: filepath.Join("..", "README.ja.md"), wants: []string{
			"不安定な`" + AlphaLanguageProfile + "`配布",
			"github.com/horizon67/forma/cmd/forma@" + AlphaLanguageProfile,
		}},
		{name: "release-notes", path: filepath.Join("releases", AlphaLanguageProfile+".md"), wants: []string{
			"# Forma " + AlphaLanguageProfile,
			"blob/" + AlphaLanguageProfile + "/docs/alpha-language-profile.md",
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content, err := os.ReadFile(test.path)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range test.wants {
				if !strings.Contains(string(content), want) {
					t.Fatalf("%s does not contain %q", test.path, want)
				}
			}
		})
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
