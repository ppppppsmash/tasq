package handler

import (
	"log"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

// 集計を発火させるリアクション名
const TriggerReaction = "clipboard"

// ReactionHandler リアクション追加イベントで集計を実行するハンドラー
type ReactionHandler struct {
	client     *slack.Client
	cmdHandler *CommandHandler
}

// NewReactionHandler リアクションハンドラーを生成する
func NewReactionHandler(client *slack.Client, cmdHandler *CommandHandler) *ReactionHandler {
	return &ReactionHandler{client: client, cmdHandler: cmdHandler}
}

// Handle トリガーリアクションが付いたらそのメッセージの集計を実行する
func (h *ReactionHandler) Handle(innerEvent *slackevents.ReactionAddedEvent) {
	if innerEvent.Reaction != TriggerReaction {
		return
	}

	if innerEvent.Item.Type != "message" {
		return
	}

	channelID := innerEvent.Item.Channel
	messageTS := innerEvent.Item.Timestamp

	log.Printf("reaction trigger: channel=%s ts=%s by=%s", channelID, messageTS, innerEvent.User)
	h.cmdHandler.RunCheck(channelID, messageTS, innerEvent.User, nil)
}

// ExtractReactionEvent EventsAPIイベントからReactionAddedEventを取り出す
func ExtractReactionEvent(eventsAPIEvent slackevents.EventsAPIEvent) (*slackevents.ReactionAddedEvent, bool) {
	inner, ok := eventsAPIEvent.InnerEvent.Data.(*slackevents.ReactionAddedEvent)
	return inner, ok
}

// PostTriggerAck トリガーリアクション検知時に「集計中」のメッセージを投稿する
func (h *ReactionHandler) PostTriggerAck(channelID string) {
	_, _, err := h.client.PostMessage(channelID, slack.MsgOptionText("📋 集計中...", false))
	if err != nil {
		log.Printf("failed to post ack: %v", err)
	}
}
