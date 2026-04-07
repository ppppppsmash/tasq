package mention

import (
	"regexp"
	"strings"
)

// Slack user mention: <@U12345> or <@U12345|name>
var userMentionRe = regexp.MustCompile(`<@(U[A-Z0-9]+)(?:\|[^>]*)?>`)

// Parentheses (half-width and full-width)
var parenRe = regexp.MustCompile(`(?:\([^)]*\)|（[^）]*）)`)

// CC: prefix — everything after CC: or cc: is excluded
var ccRe = regexp.MustCompile(`(?i)cc\s*[:：].*`)

// Parse extracts user IDs mentioned in text,
// excluding mentions inside parentheses and after CC:/cc:.
func Parse(text string) []string {
	cleaned := excludeRegions(text)

	matches := userMentionRe.FindAllStringSubmatch(cleaned, -1)
	seen := make(map[string]bool)
	var users []string
	for _, m := range matches {
		uid := m[1]
		if !seen[uid] {
			seen[uid] = true
			users = append(users, uid)
		}
	}
	return users
}

// excludeRegions removes parenthesized regions and CC: suffix from text.
func excludeRegions(text string) string {
	// Remove CC: and everything after it (per line)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = ccRe.ReplaceAllString(line, "")
	}
	text = strings.Join(lines, "\n")

	// Remove parenthesized regions
	text = parenRe.ReplaceAllString(text, "")

	return text
}
