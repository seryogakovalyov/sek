package redact

import "regexp"

const placeholder = "[REDACTED]"

type rule struct {
	pattern     *regexp.Regexp
	replacement string
}

var rules = []rule{
	{regexp.MustCompile(`(?i)\b(authorization\s*:\s*bearer)\s+([A-Za-z0-9._~+/=-]+)`), "${1} " + placeholder},
	{regexp.MustCompile(`(?i)\b(bearer)\s+([A-Za-z0-9._~+/=-]{8,})`), "${1} " + placeholder},
	{regexp.MustCompile(`(?i)\b(api[_-]?key|token|access[_-]?token|refresh[_-]?token|secret|client[_-]?secret|password|passwd|pwd|OPENAI_API_KEY|ANTHROPIC_API_KEY|GITHUB_TOKEN)\b(\s*[:=]\s*["']?)([^"'\s,;&#]+)`), "${1}${2}" + placeholder},
	{regexp.MustCompile(`\b(sk-[A-Za-z0-9_-]{8,})\b`), placeholder},
	{regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9_]{8,})\b`), placeholder},
	{regexp.MustCompile(`(?i)(https?://)([^/\s:@]+):([^@\s/]+)@`), "${1}" + placeholder + "@"},
	{regexp.MustCompile(`(?i)([?&](?:api[_-]?key|token|access[_-]?token|refresh[_-]?token|secret|password|passwd|pwd)=)([^&#\s]+)`), "${1}" + placeholder},
}

func Secrets(s string) string {
	for _, r := range rules {
		s = r.pattern.ReplaceAllString(s, r.replacement)
	}
	return s
}
