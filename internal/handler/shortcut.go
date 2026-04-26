package handler

import (
	"encoding/json"
	"log"

	"github.com/slack-go/slack"
)

// メッセージショートカットのコールバックID
const ShortcutCallbackID = "rollcall_check"

// モーダルのコールバックID・ブロックID・アクションID
const (
	ModalCallbackID  = "rollcall_check_modal"
	PostModeBlockID  = "post_mode_block"
	PostModeActionID = "post_mode_action"
	ReminderBlockID  = "reminder_block"
	ReminderActionID = "reminder_action"
)

// ModalMetadata モーダルのprivate_metadataに埋め込むコンテキスト情報
type ModalMetadata struct {
	ChannelID string `json:"channel_id"`
	MessageTS string `json:"message_ts"`
	UserID    string `json:"user_id"`
}

// ShortcutHandler メッセージショートカットで集計を実行するハンドラー
type ShortcutHandler struct {
	client     *slack.Client
	cmdHandler *CommandHandler
}

// NewShortcutHandler ショートカットハンドラーを生成する
func NewShortcutHandler(client *slack.Client, cmdHandler *CommandHandler) *ShortcutHandler {
	return &ShortcutHandler{client: client, cmdHandler: cmdHandler}
}

// Handle メッセージショートカットとモーダル送信を振り分ける
func (h *ShortcutHandler) Handle(callback slack.InteractionCallback) {
	log.Printf("interactive callback: type=%s callbackID=%s", callback.Type, callback.CallbackID)

	switch callback.Type {
	case slack.InteractionTypeMessageAction:
		if callback.CallbackID != ShortcutCallbackID {
			return
		}
		h.OpenModal(callback)
	case slack.InteractionTypeViewSubmission:
		if callback.View.CallbackID != ModalCallbackID {
			return
		}
		h.handleModalSubmission(callback)
	default:
		log.Printf("unhandled interaction type: %s", callback.Type)
	}
}

// OpenModal ショートカットから投稿モード選択モーダルを表示する（既存bot投稿がなければ即実行）
func (h *ShortcutHandler) OpenModal(callback slack.InteractionCallback) {
	// スレッド返信の場合は親メッセージを対象にする
	targetTS := callback.Message.Timestamp
	if callback.Message.ThreadTimestamp != "" && callback.Message.ThreadTimestamp != callback.Message.Timestamp {
		targetTS = callback.Message.ThreadTimestamp
	}

	channelID := callback.Channel.ID
	userID := callback.User.ID

	// 既存のbot投稿がなければモーダルなしで即実行
	if len(h.cmdHandler.findAllBotMessages(channelID, targetTS)) == 0 {
		log.Printf("shortcut trigger (no existing reply): channel=%s ts=%s by=%s", channelID, targetTS, userID)
		h.cmdHandler.RunCheck(channelID, targetTS, userID, nil, false)
		return
	}

	meta, _ := json.Marshal(ModalMetadata{
		ChannelID: channelID,
		MessageTS: targetTS,
		UserID:    userID,
	})

	optUpdate := slack.NewOptionBlockObject("update",
		slack.NewTextBlockObject(slack.PlainTextType, "上書き更新", false, false), nil)
	optNew := slack.NewOptionBlockObject("new",
		slack.NewTextBlockObject(slack.PlainTextType, "新規投稿", false, false), nil)

	radio := slack.NewRadioButtonsBlockElement(PostModeActionID, optUpdate, optNew)
	radio.InitialOption = optUpdate

	postModeBlock := slack.NewInputBlock(
		PostModeBlockID,
		slack.NewTextBlockObject(slack.PlainTextType, "投稿モード", false, false),
		nil,
		radio,
	)

	reminderPicker := slack.NewDateTimePickerBlockElement(ReminderActionID)
	reminderBlock := slack.NewInputBlock(
		ReminderBlockID,
		slack.NewTextBlockObject(slack.PlainTextType, "リマインド時刻（任意）", false, false),
		slack.NewTextBlockObject(slack.PlainTextType, "⚠ 未対応機能：現在は値を受け取るだけで通知は飛びません", false, false),
		reminderPicker,
	)
	reminderBlock.Optional = true

	modal := slack.ModalViewRequest{
		Type:            slack.VTModal,
		CallbackID:      ModalCallbackID,
		Title:           slack.NewTextBlockObject(slack.PlainTextType, "Rollcall Check", false, false),
		Submit:          slack.NewTextBlockObject(slack.PlainTextType, "実行", false, false),
		Close:           slack.NewTextBlockObject(slack.PlainTextType, "キャンセル", false, false),
		PrivateMetadata: string(meta),
		Blocks: slack.Blocks{
			BlockSet: []slack.Block{postModeBlock, reminderBlock},
		},
	}

	if _, err := h.client.OpenView(callback.TriggerID, modal); err != nil {
		log.Printf("failed to open modal: %v", err)
	}
}

// ParseModalSubmission モーダル送信からメタデータ・forceNewフラグ・リマインド時刻を取り出す
// reminderAt は未指定時 0 を返す（Unix timestamp）
func ParseModalSubmission(callback slack.InteractionCallback) (ModalMetadata, bool, int64, error) {
	var meta ModalMetadata
	if err := json.Unmarshal([]byte(callback.View.PrivateMetadata), &meta); err != nil {
		return meta, false, 0, err
	}
	forceNew := false
	var reminderAt int64
	if callback.View.State != nil {
		if block, ok := callback.View.State.Values[PostModeBlockID]; ok {
			if action, ok := block[PostModeActionID]; ok {
				forceNew = action.SelectedOption.Value == "new"
			}
		}
		if block, ok := callback.View.State.Values[ReminderBlockID]; ok {
			if action, ok := block[ReminderActionID]; ok {
				reminderAt = action.SelectedDateTime
			}
		}
	}
	return meta, forceNew, reminderAt, nil
}

// handleModalSubmission モーダル送信を処理して集計を実行する
func (h *ShortcutHandler) handleModalSubmission(callback slack.InteractionCallback) {
	meta, forceNew, reminderAt, err := ParseModalSubmission(callback)
	if err != nil {
		log.Printf("failed to parse modal metadata: %v", err)
		return
	}

	log.Printf("modal submission: channel=%s ts=%s by=%s forceNew=%v reminderAt=%d",
		meta.ChannelID, meta.MessageTS, meta.UserID, forceNew, reminderAt)
	h.cmdHandler.RunCheck(meta.ChannelID, meta.MessageTS, meta.UserID, nil, forceNew)
}
