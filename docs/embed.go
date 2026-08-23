// Package formadocs exposes the versioned documentation bundled with the
// Forma CLI. The Markdown files remain the public source of truth; embedding
// them lets an application give an authoring AI the exact guide shipped with
// the installed binary without fetching a website.
package formadocs

import (
	_ "embed"
	"fmt"
	"strings"
)

const (
	AuthoringContextSchema = "forma/authoring-context/v0alpha1"
	AlphaLanguageProfile   = "v0.1.0-alpha.1"
)

//go:embed language-guide.md
var languageGuide string

//go:embed examples/alpha-quickstart.forma
var alphaQuickstart string

//go:embed examples/email-verified-membership.forma
var emailVerifiedMembership string

// AuthoringContext renders the self-contained Markdown context intended for
// an AI that translates a person's application request into Forma source.
func AuthoringContext(binaryVersion string) string {
	var output strings.Builder
	fmt.Fprintln(&output, "# Forma AI Authoring Context")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "- Context schema: `%s`\n", AuthoringContextSchema)
	fmt.Fprintf(&output, "- Forma binary: `%s`\n", binaryVersion)
	fmt.Fprintf(&output, "- Language profile: `%s`\n", AlphaLanguageProfile)
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "Use this bundled context instead of assuming syntax from a different Forma version.")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, strings.TrimSpace(languageGuide))
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Bundled complete example")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "```forma")
	fmt.Fprintln(&output, strings.TrimSpace(alphaQuickstart))
	fmt.Fprintln(&output, "```")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Bundled email-verified membership example")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "Use this exact closed Identity shape as the starting point for membership authoring.")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "```forma")
	fmt.Fprintln(&output, strings.TrimSpace(emailVerifiedMembership))
	fmt.Fprintln(&output, "```")
	return output.String()
}

// AlphaQuickstart returns the exact example included in AuthoringContext.
func AlphaQuickstart() string {
	return strings.TrimSpace(alphaQuickstart) + "\n"
}

// EmailVerifiedMembership returns the exact Identity example bundled in the
// authoring context.
func EmailVerifiedMembership() string {
	return strings.TrimSpace(emailVerifiedMembership) + "\n"
}
