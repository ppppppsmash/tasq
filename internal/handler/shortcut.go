package handler

import (
	"log"

	"github.com/slack-go/slack"
)

const ShortcutCallbackID = "rollcall_check"

type ShortcutHandler struct {
	cmdHandler *CommandHandler
}

func NewShortcutHandler(cmdHandler *CommandHandler) *ShortcutHandler {
	return &ShortcutHandler{cmdHandler: cmdHandler}
}

func (h *ShortcutHandler) Handle(callback slack.InteractionCallback) {
	if callback.CallbackID != ShortcutCallbackID {
		return
	}

	channelID := callback.Channel.ID
	messageTS := callback.Message.Timestamp

	log.Printf("shortcut trigger: channel=%s ts=%s by=%s", channelID, messageTS, callback.User.ID)
	h.cmdHandler.RunCheck(channelID, messageTS, callback.User.ID, nil)
}
