package mention

import (
	"regexp"
)

// Slack usergroup mention: <!subteam^S12345> or <!subteam^S12345|@group-name>
var usergroupRe = regexp.MustCompile(`<!subteam\^(S[A-Z0-9]+)(?:\|[^>]*)?>`)

// ParseUserGroups extracts usergroup IDs from text.
func ParseUserGroups(text string) []string {
	matches := usergroupRe.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var groups []string
	for _, m := range matches {
		gid := m[1]
		if !seen[gid] {
			seen[gid] = true
			groups = append(groups, gid)
		}
	}
	return groups
}
