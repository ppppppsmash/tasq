package handler

import (
	"log"

	"github.com/slack-go/slack/slackevents"
)

type MentionHandler struct {
	cmdHandler *CommandHandler
}

func NewMentionHandler(cmdHandler *CommandHandler) *MentionHandler {
	return &MentionHandler{cmdHandler: cmdHandler}
}

func (h *MentionHandler) Handle(event *slackevents.AppMentionEvent) {
	channelID := event.Channel
	userID := event.User

	// If mentioned in a thread, check the parent message
	// If mentioned in a top-level message, check that message itself
	messageTS := event.ThreadTimeStamp
	if messageTS == "" {
		messageTS = event.TimeStamp
	}

	log.Printf("mention trigger: channel=%s ts=%s by=%s", channelID, messageTS, userID)
	h.cmdHandler.RunCheck(channelID, messageTS, userID, nil)
}

// ExtractMentionEvent extracts an AppMentionEvent from an EventsAPI envelope.
func ExtractMentionEvent(eventsAPIEvent slackevents.EventsAPIEvent) (*slackevents.AppMentionEvent, bool) {
	inner, ok := eventsAPIEvent.InnerEvent.Data.(*slackevents.AppMentionEvent)
	return inner, ok
}
