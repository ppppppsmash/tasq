package handler

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/slack-go/slack"
)

// Slackメッセージリンクのパターン（例: https://xxx.slack.com/archives/C12345/p1234567890123456）
var messageLinkRe = regexp.MustCompile(`/archives/([A-Z0-9]+)/p(\d{10})(\d{6})`)

// resolveTargetMessage コマンド引数からチェック対象メッセージのタイムスタンプを特定する
func resolveTargetMessage(cmd slack.SlashCommand, args string) (string, error) {
	// 引数にメッセージリンクがあればそれを使う
	if link := extractMessageLink(args); link != "" {
		return link, nil
	}

	// スレッドからの実行時は親メッセージを使う（未実装）
	if cmd.ChannelID != "" && isThreadReply(cmd) {
	}

	return "", fmt.Errorf("specify a message link: `/rollcall check <message URL>`")
}

// extractMessageLink テキストからSlackメッセージリンクを抽出してタイムスタンプに変換する
func extractMessageLink(text string) string {
	// Slackが自動的にURLを<URL|label>や<URL>形式にする
	cleaned := strings.Trim(text, "<>")
	if idx := strings.Index(cleaned, "|"); idx >= 0 {
		cleaned = cleaned[:idx]
	}

	matches := messageLinkRe.FindStringSubmatch(cleaned)
	if matches == nil {
		return ""
	}

	// p1234567890123456 → 1234567890.123456に変換
	return matches[2] + "." + matches[3]
}

// isThreadReply スレッド返信からの実行かどうか判定する（SlashCommandにthread_tsがないため未実装）
func isThreadReply(cmd slack.SlashCommand) bool {
	return false
}
