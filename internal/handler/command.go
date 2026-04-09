package handler

import (
	"fmt"
	"log"
	"strings"

	"github.com/kurosawa-dev/rollcall/internal/mention"
	"github.com/slack-go/slack"
)

type CommandHandler struct {
	client *slack.Client
}

func NewCommandHandler(client *slack.Client) *CommandHandler {
	return &CommandHandler{client: client}
}

func (h *CommandHandler) Handle(cmd slack.SlashCommand) {
	subcommand := strings.TrimSpace(cmd.Text)

	switch {
	case subcommand == "check" || strings.HasPrefix(subcommand, "check "):
		h.handleCheck(cmd)
	default:
		h.respondEphemeral(cmd.ChannelID, cmd.UserID, fmt.Sprintf("unknown subcommand: `%s`\nusage: `/rollcall check`", subcommand))
	}
}

func (h *CommandHandler) handleCheck(cmd slack.SlashCommand) {
	args := strings.TrimSpace(strings.TrimPrefix(cmd.Text, "check"))

	targetTS, err := resolveTargetMessage(cmd, args)
	if err != nil {
		h.respondEphemeral(cmd.ChannelID, cmd.UserID, fmt.Sprintf("error: %v", err))
		return
	}

	// Expand usergroup mentions in args
	groupMembers, err := h.expandUserGroups(args)
	if err != nil {
		h.respondEphemeral(cmd.ChannelID, cmd.UserID, fmt.Sprintf("error expanding usergroups: %v", err))
		return
	}

	log.Printf("check: channel=%s ts=%s args=%q groups=%d members", cmd.ChannelID, targetTS, args, len(groupMembers))
	h.runCheck(cmd.ChannelID, targetTS, cmd.UserID, groupMembers)
}

func (h *CommandHandler) expandUserGroups(text string) ([]string, error) {
	groupIDs := mention.ParseUserGroups(text)
	if len(groupIDs) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool)
	var members []string
	for _, gid := range groupIDs {
		users, err := h.client.GetUserGroupMembers(gid)
		if err != nil {
			return nil, fmt.Errorf("get members of %s: %w", gid, err)
		}
		for _, uid := range users {
			if !seen[uid] {
				seen[uid] = true
				members = append(members, uid)
			}
		}
	}
	return members, nil
}

func (h *CommandHandler) respondEphemeral(channel, userID, text string) {
	_, err := h.client.PostEphemeral(channel, userID, slack.MsgOptionText(text, false))
	if err != nil {
		log.Printf("failed to post ephemeral message: %v", err)
	}
}

func (h *CommandHandler) respond(channel, text string, threadTS ...string) {
	opts := []slack.MsgOption{slack.MsgOptionText(text, false)}
	if len(threadTS) > 0 && threadTS[0] != "" {
		opts = append(opts, slack.MsgOptionTS(threadTS[0]))
	}
	_, _, err := h.client.PostMessage(channel, opts...)
	if err != nil {
		log.Printf("failed to post message: %v", err)
	}
}
