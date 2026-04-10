package mention

import (
	"regexp"
	"strings"
)

// Slackの個人メンションパターン（例: <@U12345>、<@U12345|name>）
var userMentionRe = regexp.MustCompile(`<@(U[A-Z0-9]+)(?:\|[^>]*)?>`)

// 半角・全角カッコのパターン
var parenRe = regexp.MustCompile(`(?:\([^)]*\)|（[^）]*）)`)

// CC:以降を除外するパターン
var ccRe = regexp.MustCompile(`(?i)cc\s*[:：].*`)

// Parse テキストからメンションされたユーザーIDを抽出する（カッコ内やCC:以降は除外）
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

// excludeRegions カッコ内とCC:以降を除去する
func excludeRegions(text string) string {
	// 各行のCC:以降を除去
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = ccRe.ReplaceAllString(line, "")
	}
	text = strings.Join(lines, "\n")

	// カッコ内を除去
	text = parenRe.ReplaceAllString(text, "")

	return text
}
