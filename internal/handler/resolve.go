package handler

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/slack-go/slack"
)

// Slack message link pattern:
// https://xxx.slack.com/archives/C12345/p1234567890123456
var messageLinkRe = regexp.MustCompile(`/archives/([A-Z0-9]+)/p(\d{10})(\d{6})`)

func resolveTargetMessage(cmd slack.SlashCommand, args string) (string, error) {
	// Case 1: message link in args
	if link := extractMessageLink(args); link != "" {
		return link, nil
	}

	// Case 2: invoked in a thread — use the thread parent
	if cmd.ChannelID != "" && isThreadReply(cmd) {
		// SlashCommand doesn't directly expose thread_ts,
		// but if triggered from a thread, the trigger_id context may help.
		// For now, we rely on the message link approach or future event-based detection.
	}

	return "", fmt.Errorf("specify a message link: `/tasq check <message URL>`")
}

func extractMessageLink(text string) string {
	// Slack auto-formats URLs as <URL|label> or <URL>
	cleaned := strings.Trim(text, "<>")
	if idx := strings.Index(cleaned, "|"); idx >= 0 {
		cleaned = cleaned[:idx]
	}

	matches := messageLinkRe.FindStringSubmatch(cleaned)
	if matches == nil {
		return ""
	}

	// Convert p1234567890123456 → 1234567890.123456
	return matches[2] + "." + matches[3]
}

func isThreadReply(cmd slack.SlashCommand) bool {
	// SlashCommand doesn't natively carry thread_ts.
	// This is a placeholder for future implementation.
	return false
}
