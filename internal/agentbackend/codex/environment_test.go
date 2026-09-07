package codex

import (
	"reflect"
	"testing"
)

func TestCodexEnvironmentAllowsRuntimeInputsWithoutForwardingCredentialsOrApplicationSecrets(t *testing.T) {
	got := environmentAllowlist([]string{
		"PATH=/usr/bin",
		"HOME=/Users/person",
		"CODEX_HOME=/Users/person/.codex",
		"OPENAI_API_KEY=must-not-leak",
		"CODEX_ACCESS_TOKEN=must-not-leak",
		"DATABASE_URL=must-not-leak",
		"HTTPS_PROXY=http://proxy.invalid",
		"CODEX_CA_CERTIFICATE=/cert.pem",
	})
	want := []string{
		"PATH=/usr/bin",
		"HOME=/Users/person",
		"CODEX_HOME=/Users/person/.codex",
		"HTTPS_PROXY=http://proxy.invalid",
		"CODEX_CA_CERTIFICATE=/cert.pem",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment = %#v, want %#v", got, want)
	}
}
