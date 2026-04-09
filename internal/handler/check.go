package handler

import (
	"fmt"
	"log"
	"math/rand"
	"strings"

	"github.com/kurosawa-dev/rollcall/internal/mention"
	"github.com/slack-go/slack"
)

var completionQuotes = []string{
	"You're all my hero.",
	"The question isn't what are we gonna do. You already did it.",
	"Abe Froman would be proud.",
}

// CompletionReactions are reactions that count as "done".
var CompletionReactions = []string{
	"white_check_mark",
	"taiouzumi",
	"済",
	"太い丸",
	"対応しました",
	"確認_済",
	"完了",
	"承知_しました",
	"kakuninzumi",
}

type CheckResult struct {
	MessageText string
	TargetUsers []string
	DoneUsers   []string
	UndoneUsers []string
}

func (h *CommandHandler) RunCheck(channelID, messageTS, userID string, explicitGroupMembers []string) {
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

	targetUsers, err := h.resolveTargetUsers(channelID, msg.Text, explicitGroupMembers)
	if err != nil {
		h.respondEphemeral(channelID, userID, fmt.Sprintf("error resolving target users: %v", err))
		return
	}
	if len(targetUsers) == 0 {
		h.respondEphemeral(channelID, userID, "no target users found")
		return
	}

	targetUsers, err = h.filterBots(targetUsers)
	if err != nil {
		log.Printf("warning: failed to filter bots: %v", err)
	}

	doneSet := h.collectDoneUsers(channelID, messageTS)
	result := buildResult(msg.Text, targetUsers, doneSet)
	text := formatResult(result)

	// Update existing bot message in thread, or post new one
	if existingTS := h.findBotMessage(channelID, messageTS); existingTS != "" {
		h.updateMessage(channelID, existingTS, text)
	} else {
		h.respond(channelID, text, messageTS)
	}
}

func (h *CommandHandler) findBotMessage(channelID, threadTS string) string {
	// Get bot's own user ID
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

func (h *CommandHandler) updateMessage(channelID, messageTS, text string) {
	_, _, _, err := h.client.UpdateMessage(channelID, messageTS, slack.MsgOptionText(text, false))
	if err != nil {
		log.Printf("failed to update message: %v", err)
	}
}

func (h *CommandHandler) resolveTargetUsers(channelID, messageText string, explicitGroupMembers []string) ([]string, error) {
	if len(explicitGroupMembers) > 0 {
		return explicitGroupMembers, nil
	}

	mentioned := mention.Parse(messageText)
	if len(mentioned) > 0 {
		return mentioned, nil
	}

	return h.getChannelMembers(channelID)
}

func (h *CommandHandler) getChannelMembers(channelID string) ([]string, error) {
	var allMembers []string
	cursor := ""
	for {
		params := &slack.GetUsersInConversationParameters{
			ChannelID: channelID,
			Cursor:    cursor,
			Limit:     200,
		}
		members, nextCursor, err := h.client.GetUsersInConversation(params)
		if err != nil {
			return nil, err
		}
		allMembers = append(allMembers, members...)
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}
	return allMembers, nil
}

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

func isCompletionReaction(name string) bool {
	for _, r := range CompletionReactions {
		if name == r {
			return true
		}
	}
	return false
}

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

func formatResult(r CheckResult) string {
	total := len(r.TargetUsers)
	doneCount := len(r.DoneUsers)

	var b strings.Builder
	fmt.Fprintf(&b, "Bueller?... Bueller?... Anyone?\n確認・対応が済んだら、✅リアクションをつけてください\n")
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

func formatUserList(userIDs []string) string {
	mentions := make([]string, len(userIDs))
	for i, uid := range userIDs {
		mentions[i] = fmt.Sprintf("<@%s>", uid)
	}
	return strings.Join(mentions, "、")
}
