package handler

import (
	"log"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

const TriggerReaction = "clipboard"

type ReactionHandler struct {
	client     *socketmode.Client
	cmdHandler *CommandHandler
}

func NewReactionHandler(client *socketmode.Client, cmdHandler *CommandHandler) *ReactionHandler {
	return &ReactionHandler{client: client, cmdHandler: cmdHandler}
}

func (h *ReactionHandler) Handle(evt socketmode.Event, innerEvent *slackevents.ReactionAddedEvent) {
	if innerEvent.Reaction != TriggerReaction {
		return
	}

	if innerEvent.Item.Type != "message" {
		return
	}

	channelID := innerEvent.Item.Channel
	messageTS := innerEvent.Item.Timestamp

	log.Printf("reaction trigger: channel=%s ts=%s by=%s", channelID, messageTS, innerEvent.User)
	h.cmdHandler.runCheck(channelID, messageTS, nil)
}

// ExtractReactionEvent extracts a ReactionAddedEvent from an EventsAPI envelope.
func ExtractReactionEvent(eventsAPIEvent slackevents.EventsAPIEvent) (*slackevents.ReactionAddedEvent, bool) {
	inner, ok := eventsAPIEvent.InnerEvent.Data.(*slackevents.ReactionAddedEvent)
	return inner, ok
}

// PostTriggerAck posts a brief acknowledgement when the trigger reaction is detected.
func (h *ReactionHandler) PostTriggerAck(channelID string) {
	_, _, err := h.client.PostMessage(channelID, slack.MsgOptionText("📋 集計中...", false))
	if err != nil {
		log.Printf("failed to post ack: %v", err)
	}
}
