package redact

import (
	"strings"
	"testing"
)

func TestSecretsRedactsCommonSecretForms(t *testing.T) {
	input := strings.Join([]string{
		`OPENAI_API_KEY=sk-testSECRET12345`,
		`password: "hunter2"`,
		`Authorization: Bearer abcdefghijklmnop`,
		`github token ghp_abcdefghijklmnopqrstuvwxyz`,
		`https://user:pass@example.com/path?token=abc123&debug=true`,
	}, "\n")

	got := Secrets(input)

	for _, leaked := range []string{
		"sk-testSECRET12345",
		"hunter2",
		"abcdefghijklmnop",
		"ghp_abcdefghijklmnopqrstuvwxyz",
		"user:pass",
		"token=abc123",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("redacted output leaked %q:\n%s", leaked, got)
		}
	}

	for _, kept := range []string{
		"OPENAI_API_KEY=" + placeholder,
		"password: \"" + placeholder,
		"Authorization: Bearer " + placeholder,
		"https://" + placeholder + "@example.com/path?token=" + placeholder + "&debug=true",
	} {
		if !strings.Contains(got, kept) {
			t.Fatalf("redacted output missing %q:\n%s", kept, got)
		}
	}
}

func TestSecretsPreservesTechnicalContext(t *testing.T) {
	input := "run GOCACHE=/tmp/sek-go-cache go test ./... after error exit status 128 in internal/distill/distill.go"
	got := Secrets(input)
	if got != input {
		t.Fatalf("non-secret technical context changed:\nwant: %s\n got: %s", input, got)
	}
}
