package handler

import (
	"fmt"
	"log"
	"math/rand"
	"strings"

	"github.com/kurosawa-dev/rollcall/internal/mention"
	"github.com/slack-go/slack"
)

// 全員完了時に表示するランダム引用文
var completionQuotes = []string{
	"You're all my hero.",
	"The question isn't what are we gonna do. You already did it.",
	"Abe Froman would be proud.",
}

// 「完了」として扱うリアクション一覧
var CompletionReactions = []string{
	"white_check_mark",
	"taiouzumi",
	"済",
	"対応しました",
	"確認_済",
	"完了",
	"承知_しました",
	"kakuninzumi",
}

// 集計結果を保持する構造体
type CheckResult struct {
	MessageText string
	TargetUsers []string
	DoneUsers   []string
	UndoneUsers []string
}

// RunCheck 対象メッセージのリアクションを集計して進捗を投稿する
func (h *CommandHandler) RunCheck(channelID, messageTS, userID string, explicitGroupMembers []string) {
	// 対象メッセージを取得
	msgs, err := h.client.GetConversationHistory(&slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Latest:    messageTS,
		Inclusive: true,
		Limit:     1,
	})
	if err != nil {
		h.respondEphemeral(channelID, userID, fmt.Sprintf("error fetching message: %v", err))
		return
	}
	if len(msgs.Messages) == 0 {
		h.respondEphemeral(channelID, userID, "message not found")
		return
	}
	msg := msgs.Messages[0]

	// メンションやユーザーグループから対象ユーザーを特定
	targetUsers, err := h.resolveTargetUsers(channelID, msg.Text, explicitGroupMembers)
	if err != nil {
		h.respondEphemeral(channelID, userID, fmt.Sprintf("error resolving target users: %v", err))
		return
	}
	if len(targetUsers) == 0 {
		h.respond(channelID, "Anyone? Anyone? ... No one's here.\nメッセージにメンション（@ユーザー）が含まれていないため、集計対象が見つかりませんでした。", messageTS)
		return
	}

	// botユーザーを除外
	targetUsers, err = h.filterBots(targetUsers)
	if err != nil {
		log.Printf("warning: failed to filter bots: %v", err)
	}
	if len(targetUsers) == 0 {
		h.respond(channelID, "Anyone? Anyone? ... No one's here.\nメッセージにメンション（@ユーザー）が含まれていないため、集計対象が見つかりませんでした。", messageTS)
		return
	}

	// リアクション済みユーザーを収集して結果を組み立てる
	doneSet := h.collectDoneUsers(channelID, messageTS)
	result := buildResult(msg.Text, targetUsers, doneSet)
	text := formatResult(result)

	// 既存のbot投稿があれば更新、なければ新規投稿
	if existingTS := h.findBotMessage(channelID, messageTS); existingTS != "" {
		h.updateMessage(channelID, existingTS, text)
	} else {
		h.respond(channelID, text, messageTS)
	}
}

// findBotMessage スレッド内の既存bot投稿のタイムスタンプを返す
func (h *CommandHandler) findBotMessage(channelID, threadTS string) string {
	// botの自身のユーザーIDを取得
	authTest, err := h.client.AuthTest()
	if err != nil {
		log.Printf("warning: failed to auth test: %v", err)
		return ""
	}
	botUserID := authTest.UserID

	msgs, _, _, err := h.client.GetConversationReplies(&slack.GetConversationRepliesParameters{
		ChannelID: channelID,
		Timestamp: threadTS,
	})
	if err != nil {
		log.Printf("warning: failed to get thread replies: %v", err)
		return ""
	}

	for _, m := range msgs {
		if m.User == botUserID && m.Timestamp != threadTS {
			return m.Timestamp
		}
	}
	return ""
}

// updateMessage 既存メッセージを更新する
func (h *CommandHandler) updateMessage(channelID, messageTS, text string) {
	_, _, _, err := h.client.UpdateMessage(channelID, messageTS, slack.MsgOptionText(text, false))
	if err != nil {
		log.Printf("failed to update message: %v", err)
	}
}

// resolveTargetUsers 対象ユーザーを特定する（コマンド引数のグループ > 本文メンション > 本文グループ）
func (h *CommandHandler) resolveTargetUsers(channelID, messageText string, explicitGroupMembers []string) ([]string, error) {
	// コマンド引数でグループが指定されていればそれを優先
	if len(explicitGroupMembers) > 0 {
		return explicitGroupMembers, nil
	}

	seen := make(map[string]bool)
	var users []string

	// 本文中の個人メンションを抽出
	for _, uid := range mention.Parse(messageText) {
		if !seen[uid] {
			seen[uid] = true
			users = append(users, uid)
		}
	}

	// 本文中のユーザーグループを展開してマージ
	groupMembers, err := h.expandUserGroups(messageText)
	if err != nil {
		return nil, err
	}
	for _, uid := range groupMembers {
		if !seen[uid] {
			seen[uid] = true
			users = append(users, uid)
		}
	}

	if len(users) > 0 {
		return users, nil
	}

	// チャンネル全員へのフォールバック（誤爆防止のため無効化中）
	// return h.getChannelMembers(channelID)
	return nil, nil
}

// func (h *CommandHandler) getChannelMembers(channelID string) ([]string, error) {
// 	var allMembers []string
// 	cursor := ""
// 	for {
// 		params := &slack.GetUsersInConversationParameters{
// 			ChannelID: channelID,
// 			Cursor:    cursor,
// 			Limit:     200,
// 		}
// 		members, nextCursor, err := h.client.GetUsersInConversation(params)
// 		if err != nil {
// 			return nil, err
// 		}
// 		allMembers = append(allMembers, members...)
// 		if nextCursor == "" {
// 			break
// 		}
// 		cursor = nextCursor
// 	}
// 	return allMembers, nil
// }

// filterBots botユーザーを並行で判定して除外する
func (h *CommandHandler) filterBots(userIDs []string) ([]string, error) {
	type result struct {
		uid   string
		isBot bool
		err   error
	}

	ch := make(chan result, len(userIDs))
	for _, uid := range userIDs {
		go func(uid string) {
			info, err := h.client.GetUserInfo(uid)
			if err != nil {
				ch <- result{uid: uid, err: err}
				return
			}
			ch <- result{uid: uid, isBot: info.IsBot || info.IsAppUser}
		}(uid)
	}

	var humans []string
	for range userIDs {
		r := <-ch
		if r.err != nil {
			return nil, fmt.Errorf("get user info %s: %w", r.uid, r.err)
		}
		if !r.isBot {
			humans = append(humans, r.uid)
		}
	}
	return humans, nil
}

// collectDoneUsers 対象メッセージの完了リアクションをつけたユーザーを収集する
func (h *CommandHandler) collectDoneUsers(channelID, messageTS string) map[string]bool {
	done := make(map[string]bool)

	item, err := h.client.GetReactions(slack.ItemRef{
		Channel:   channelID,
		Timestamp: messageTS,
	}, slack.GetReactionsParameters{Full: true})
	if err != nil {
		log.Printf("warning: failed to get reactions: %v", err)
		return done
	}

	for _, r := range item.Reactions {
		if isCompletionReaction(r.Name) {
			for _, u := range r.Users {
				done[u] = true
			}
		}
	}
	return done
}

// isCompletionReaction リアクション名が完了扱いかどうか判定する
func isCompletionReaction(name string) bool {
	for _, r := range CompletionReactions {
		if name == r {
			return true
		}
	}
	return false
}

// buildResult 対象ユーザーを完了/未完了に振り分けて結果を構築する
func buildResult(messageText string, targetUsers []string, doneSet map[string]bool) CheckResult {
	var done, undone []string
	for _, uid := range targetUsers {
		if doneSet[uid] {
			done = append(done, uid)
		} else {
			undone = append(undone, uid)
		}
	}
	return CheckResult{
		MessageText: messageText,
		TargetUsers: targetUsers,
		DoneUsers:   done,
		UndoneUsers: undone,
	}
}

// formatResult 集計結果をSlack投稿用テキストに整形する
func formatResult(r CheckResult) string {
	total := len(r.TargetUsers)
	doneCount := len(r.DoneUsers)

	var b strings.Builder
	reactions := make([]string, len(CompletionReactions))
	for i, r := range CompletionReactions {
		reactions[i] = fmt.Sprintf(":%s:", r)
	}
	fmt.Fprintf(&b, "Bueller?... Bueller?... Anyone?\n%sリアクションで完了を教えてね！\n", strings.Join(reactions, " "))
	fmt.Fprintf(&b, "\n\n")
	fmt.Fprintf(&b, "対象: %d名\n", total)

	if len(r.UndoneUsers) > 0 {
		fmt.Fprintf(&b, "❌ 未完了（%d名）", len(r.UndoneUsers))
		fmt.Fprintf(&b, "%s\n", formatUserList(r.UndoneUsers))
	}

	pct := 0
	if total > 0 {
		pct = doneCount * 100 / total
	}
	fmt.Fprintf(&b, "\n")
	fmt.Fprintf(&b, "進捗: %d/%d（%d%%）", doneCount, total, pct)

	if total > 0 && doneCount == total {
		quote := completionQuotes[rand.Intn(len(completionQuotes))]
		fmt.Fprintf(&b, "\n\n🎉%s", quote)
	}

	return b.String()
}

// formatUserList ユーザーIDリストを「、」区切りのメンション文字列にする
func formatUserList(userIDs []string) string {
	mentions := make([]string, len(userIDs))
	for i, uid := range userIDs {
		mentions[i] = fmt.Sprintf("<@%s>", uid)
	}
	return strings.Join(mentions, "、")
}
