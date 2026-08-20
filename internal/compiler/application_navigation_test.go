package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplicationNavigationRejectsInvalidOwnershipAndTargets(t *testing.T) {
	tests := []struct {
		name   string
		source string
		code   string
		want   string
	}{
		{
			name:   "duplicate application entry",
			source: "entry Home\nentry Other\npage Home {\n}\npage Other {\n}\n",
			code:   "F2504", want: "duplicate application `entry`",
		},
		{
			name:   "unknown application entry",
			source: "entry Missing\npage Home {\n}\n",
			code:   "F2504", want: "references unknown page `Missing`",
		},
		{
			name:   "parameterized application entry",
			source: "entity User {\n}\nentry Profile\npage Profile(user User) {\n}\n",
			code:   "F2504", want: "cannot enter parameterized page `Profile`",
		},
		{
			name:   "duplicate page transition",
			source: "page Start {\n    continue One\n    continue Two\n}\npage One {\n}\npage Two {\n}\n",
			code:   "F2505", want: "duplicate `continue` transition",
		},
		{
			name:   "unknown transition target",
			source: "page Start {\n    continue Missing\n}\n",
			code:   "F2504", want: "references unknown page `Missing`",
		},
		{
			name:   "parameterized transition target",
			source: "entity User {\n}\npage Start {\n    continue Profile\n}\npage Profile(user User) {\n}\n",
			code:   "F2504", want: "cannot enter parameterized page `Profile`",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Compile([]SourceFile{NewSourceFile("invalid-navigation.forma", test.source)})
			if !hasDiagnostic(result.Diagnostics, test.code, test.want) {
				t.Fatalf("diagnostics:\n%s\nwant %s containing %q", diagnosticMessages(result.Diagnostics), test.code, test.want)
			}
		})
	}
}

func TestApplicationNavigationRejectsLegacyAndPageLocalContinuationTogether(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "examples", "email-verified-membership.forma"))
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Replace(string(content), "success RegistrationComplete\n", "success RegistrationComplete\n        continue SignIn\n", 1)
	result := Compile([]SourceFile{NewSourceFile("duplicate-owner.forma", source)})
	if !hasDiagnostic(result.Diagnostics, "F2505", "continuation from page `RegistrationComplete` is declared twice") {
		t.Fatalf("diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
}

func TestValidateResolvedIntentRejectsUnsupportedSurfaceTransitionKind(t *testing.T) {
	result := Compile([]SourceFile{NewSourceFile("navigation.forma", "entry Start\npage Start {\n    continue End\n}\npage End {\n}\n")})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics:\n%s", diagnosticMessages(result.Diagnostics))
	}
	for index := range result.Intent.Pages {
		if result.Intent.Pages[index].Name == "Start" {
			result.Intent.Pages[index].SurfaceTransitions[0].Kind = "back"
		}
	}
	if err := ValidateResolvedIntent(result.Intent); err == nil || !strings.Contains(err.Error(), "unsupported kind") {
		t.Fatalf("validation error = %v", err)
	}
}
