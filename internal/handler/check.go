package handler

import (
	"fmt"
	"log"
	"strings"

	"github.com/kurosawa-dev/rollcall/internal/mention"
	"github.com/slack-go/slack"
)

// CompletionReactions are reactions that count as "done".
var CompletionReactions = []string{"white_check_mark"}

type CheckResult struct {
	MessageText string
	TargetUsers []string
	DoneUsers   []string
	UndoneUsers []string
}

func (h *CommandHandler) runCheck(channelID, messageTS string, explicitGroupMembers []string) {
	msgs, err := h.client.GetConversationHistory(&slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Latest:    messageTS,
		Inclusive: true,
		Limit:     1,
	})
	if err != nil {
		h.respond(channelID, fmt.Sprintf("error fetching message: %v", err))
		return
	}
	if len(msgs.Messages) == 0 {
		h.respond(channelID, "message not found")
		return
	}
	msg := msgs.Messages[0]

	targetUsers, err := h.resolveTargetUsers(channelID, msg.Text, explicitGroupMembers)
	if err != nil {
		h.respond(channelID, fmt.Sprintf("error resolving target users: %v", err))
		return
	}
	if len(targetUsers) == 0 {
		h.respond(channelID, "no target users found")
		return
	}

	targetUsers, err = h.filterBots(targetUsers)
	if err != nil {
		log.Printf("warning: failed to filter bots: %v", err)
	}

	doneSet := h.collectDoneUsers(channelID, messageTS)
	result := buildResult(msg.Text, targetUsers, doneSet)
	h.respond(channelID, formatResult(result))
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
	var humans []string
	for _, uid := range userIDs {
		info, err := h.client.GetUserInfo(uid)
		if err != nil {
			return nil, fmt.Errorf("get user info %s: %w", uid, err)
		}
		if !info.IsBot && !info.IsAppUser {
			humans = append(humans, uid)
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
	fmt.Fprintf(&b, "📋 確認・対応が済んだら、✅リアクションをつけてください\n")
	fmt.Fprintf(&b, "\n\n")
	fmt.Fprintf(&b, "対象: %d名\n", total)

	if doneCount > 0 {
		fmt.Fprintf(&b, "✅ 完了（%d名）", doneCount)
		fmt.Fprintf(&b, "%s\n", formatUserList(r.DoneUsers))
	}
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

	return b.String()
}

func formatUserList(userIDs []string) string {
	mentions := make([]string, len(userIDs))
	for i, uid := range userIDs {
		mentions[i] = fmt.Sprintf("<@%s>", uid)
	}
	return strings.Join(mentions, "、")
}
