package handler

import (
	"log"

	"github.com/slack-go/slack/slackevents"
)

// MentionHandler botへのメンションで集計を実行するハンドラー
type MentionHandler struct {
	cmdHandler *CommandHandler
}

// NewMentionHandler メンションハンドラーを生成する
func NewMentionHandler(cmdHandler *CommandHandler) *MentionHandler {
	return &MentionHandler{cmdHandler: cmdHandler}
}

// Handle botがメンションされたメッセージで集計を実行する（スレッド内は無視）
func (h *MentionHandler) Handle(event *slackevents.AppMentionEvent) {
	// スレッド内のメンションは無視
	if event.ThreadTimeStamp != "" {
		return
	}

	channelID := event.Channel
	userID := event.User
	messageTS := event.TimeStamp

	log.Printf("mention trigger: channel=%s ts=%s by=%s", channelID, messageTS, userID)
	h.cmdHandler.RunCheck(channelID, messageTS, userID, nil, false)
}

// ExtractMentionEvent EventsAPIイベントからAppMentionEventを取り出す
func ExtractMentionEvent(eventsAPIEvent slackevents.EventsAPIEvent) (*slackevents.AppMentionEvent, bool) {
	inner, ok := eventsAPIEvent.InnerEvent.Data.(*slackevents.AppMentionEvent)
	return inner, ok
}
