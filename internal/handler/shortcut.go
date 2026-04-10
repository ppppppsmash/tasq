package handler

import (
	"log"

	"github.com/slack-go/slack"
)

// メッセージショートカットのコールバックID
const ShortcutCallbackID = "rollcall_check"

// ShortcutHandler メッセージショートカットで集計を実行するハンドラー
type ShortcutHandler struct {
	cmdHandler *CommandHandler
}

// NewShortcutHandler ショートカットハンドラーを生成する
func NewShortcutHandler(cmdHandler *CommandHandler) *ShortcutHandler {
	return &ShortcutHandler{cmdHandler: cmdHandler}
}

// Handle メッセージショートカットから集計を実行する（スレッド返信は無視）
func (h *ShortcutHandler) Handle(callback slack.InteractionCallback) {
	if callback.CallbackID != ShortcutCallbackID {
		return
	}

	// スレッド返信へのショートカットは無視
	if callback.Message.ThreadTimestamp != "" && callback.Message.ThreadTimestamp != callback.Message.Timestamp {
		return
	}

	channelID := callback.Channel.ID
	messageTS := callback.Message.Timestamp

	log.Printf("shortcut trigger: channel=%s ts=%s by=%s", channelID, messageTS, callback.User.ID)
	h.cmdHandler.RunCheck(channelID, messageTS, callback.User.ID, nil)
}
