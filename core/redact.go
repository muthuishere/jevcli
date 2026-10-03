package core

import (
	"net"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Hosted reports whether a profile's endpoint leaves this machine's network: anything that is not loopback, a private
// (RFC 1918 / unique-local) or link-local address, a CGNAT/Tailscale address, or a *.local / *.internal name. Agent
// state sent there is scrubbed first (Scrub). ForceHosted lets tests treat a local test server as hosted.
var ForceHosted bool

func Hosted(p Profile) bool {
	if ForceHosted {
		return true
	}
	u, err := url.Parse(p.Norm().URL)
	if err != nil || u.Hostname() == "" {
		return true // unknown: fail safe, scrub
	}
	h := strings.ToLower(u.Hostname())
	if h == "localhost" || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") || strings.HasSuffix(h, ".localhost") {
		return false
	}
	ip := net.ParseIP(h)
	if ip == nil {
		return true // a public DNS name
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return false
	}
	if _, cgnat, _ := net.ParseCIDR("100.64.0.0/10"); cgnat.Contains(ip) {
		return false
	}
	return true
}

// Scrub removes secrets, email addresses and phone numbers from text bound for a hosted endpoint, returning the clean
// text and how many items it replaced. Secrets: the live value of every env var whose name looks secret (KEY, TOKEN,
// SECRET, PASSWORD, _PW), well-known token formats, private-key blocks and credential-bearing URLs. It never logs or
// returns what it removed. An equivalent of `sec seal`, kept in-process so a hook or a cron has no extra dependency.
func Scrub(s string) (string, int) {
	n := 0
	for _, v := range secretEnvValues() {
		if c := strings.Count(s, v); c > 0 {
			s = strings.ReplaceAll(s, v, "[REDACTED:secret]")
			n += c
		}
	}
	for _, r := range redactRules {
		if c := len(r.re.FindAllStringIndex(s, -1)); c > 0 {
			n += c
			s = r.re.ReplaceAllString(s, r.with)
		}
	}
	return s, n
}

type redactRule struct {
	re   *regexp.Regexp
	with string
}

var redactRules = []redactRule{
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`), "[REDACTED:private-key]"},
	{regexp.MustCompile(`\b(?:sk|pk|rk)-(?:ant-|proj-|live-|test-)?[A-Za-z0-9_-]{16,}`), "[REDACTED:api-key]"},
	{regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr|github_pat)_[A-Za-z0-9_]{20,}`), "[REDACTED:github-token]"},
	{regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`), "[REDACTED:slack-token]"},
	{regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`), "[REDACTED:aws-key]"},
	{regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`), "[REDACTED:google-key]"},
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`), "[REDACTED:jwt]"},
	{regexp.MustCompile(`(?i)\b(bearer|token|api[_-]?key|secret|password|passwd|pwd)(\s*[:=]\s*|\s+)["']?[A-Za-z0-9_./+=-]{12,}`), "$1 [REDACTED:secret]"},
	{regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^\s/:@]+:[^\s/@]+@`), "${1}[REDACTED:credentials]@"},
	{regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`), "[REDACTED:email]"},
	// phones: +country forms, US (xxx) xxx-xxxx, Indian 10-digit mobiles. Not dates, times, prices or IDs.
	{regexp.MustCompile(`\+\d{1,3}[\s.-]?\(?\d{2,5}\)?(?:[\s.-]?\d{2,5}){1,4}\b`), "[REDACTED:phone]"},
	{regexp.MustCompile(`\(\d{3}\)\s?\d{3}[\s.-]\d{4}\b`), "[REDACTED:phone]"},
	{regexp.MustCompile(`(^|[^\d.:/-])[6-9]\d{9}($|[^\d.:/-])`), "${1}[REDACTED:phone]${2}"},
}

func secretEnvValues() []string {
	var vals []string
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || len(v) < 8 {
			continue
		}
		u := strings.ToUpper(k)
		if strings.Contains(u, "KEY") || strings.Contains(u, "TOKEN") || strings.Contains(u, "SECRET") ||
			strings.Contains(u, "PASSWORD") || strings.HasSuffix(u, "_PW") {
			vals = append(vals, v)
		}
	}
	sort.Slice(vals, func(i, j int) bool { return len(vals[i]) > len(vals[j]) }) // longest first
	return vals
}

// redactQuestions scrubs every string inside a questions map (instructions, criteria) in place of a copy.
func redactQuestions(qs map[string]any) (map[string]any, int) {
	n := 0
	var walk func(v any) any
	walk = func(v any) any {
		switch t := v.(type) {
		case string:
			s, c := Scrub(t)
			n += c
			return s
		case map[string]any:
			m := make(map[string]any, len(t))
			for k, x := range t {
				m[k] = walk(x)
			}
			return m
		case map[string]string:
			m := make(map[string]string, len(t))
			for k, x := range t {
				s, c := Scrub(x)
				n += c
				m[k] = s
			}
			return m
		case []any:
			a := make([]any, len(t))
			for i, x := range t {
				a[i] = walk(x)
			}
			return a
		}
		return v
	}
	out := make(map[string]any, len(qs))
	for k, v := range qs {
		out[k] = walk(v)
	}
	return out, n
}
