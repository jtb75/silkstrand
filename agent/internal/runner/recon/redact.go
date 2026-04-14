package recon

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
)

// Redact strips common secret patterns and truncates large response
// bodies before evidence flows over the WSS tunnel and into the API
// (ADR 003 audit 4.3). Customers can override the body cap via
// SILKSTRAND_EVIDENCE_BODY_MAX.
//
// Patterns are intentionally conservative — false-positive masking is
// preferable to leaking secrets. R2 will add customer-overridable
// rules from /etc/silkstrand/redaction-rules.yaml.
var redactPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),                          // AWS access keys
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`), // JWTs
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-+/=]{16,}`),
	regexp.MustCompile(`(?i)password\s*[=:]\s*\S+`),
	regexp.MustCompile(`postgres(?:ql)?://[^\s"'@]+:[^\s"'@]+@`),
	regexp.MustCompile(`mysql://[^\s"'@]+:[^\s"'@]+@`),
	regexp.MustCompile(`mongodb(?:\+srv)?://[^\s"'@]+:[^\s"'@]+@`),
}

const redactionMarker = "[REDACTED]"

func bodyMax() int {
	if v := os.Getenv("SILKSTRAND_EVIDENCE_BODY_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 4 * 1024
}

// String applies the redact patterns to a single string and truncates.
func String(s string) string {
	for _, re := range redactPatterns {
		s = re.ReplaceAllString(s, redactionMarker)
	}
	if max := bodyMax(); len(s) > max {
		s = s[:max] + "...[truncated]"
	}
	return s
}

// JSON walks a JSONB blob and applies String to every string value
// recursively. Arrays and objects are preserved structurally. Returns
// the redacted JSON in canonical form.
func JSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		// Not parseable JSON — treat as string and redact.
		return json.RawMessage(strconvQuote(String(string(raw))))
	}
	v = redactValue(v)
	out, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return out
}

func redactValue(v any) any {
	switch t := v.(type) {
	case string:
		return String(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = redactValue(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redactValue(val)
		}
		return out
	default:
		return v
	}
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
