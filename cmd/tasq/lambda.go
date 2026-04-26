package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/kurosawa-dev/rollcall/internal/handler"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

// asyncTask is a payload for self-invocation to process events asynchronously.
type asyncTask struct {
	Async     bool   `json:"async"`
	Type      string `json:"type"`       // "command", "shortcut", "reaction", "mention", "modal_submission"
	ChannelID string `json:"channel_id"`
	MessageTS string `json:"message_ts"`
	UserID    string `json:"user_id"`
	Text      string `json:"text,omitempty"`
	ForceNew  bool   `json:"force_new,omitempty"`
}

type lambdaHandler struct {
	signingSecret  string
	lambdaClient   *awslambda.Client
	cmdHandler     *handler.CommandHandler
	// reactionHandler *handler.ReactionHandler
	shortcutHandler *handler.ShortcutHandler
	mentionHandler  *handler.MentionHandler
}

func runLambda(api *slack.Client) error {
	signingSecret := os.Getenv("SLACK_SIGNING_SECRET")
	if signingSecret == "" {
		return fmt.Errorf("SLACK_SIGNING_SECRET must be set for HTTP mode")
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	cmdHandler := handler.NewCommandHandler(api)
	// reactionHandler := handler.NewReactionHandler(api, cmdHandler)
	shortcutHandler := handler.NewShortcutHandler(api, cmdHandler)
	mentionHandler := handler.NewMentionHandler(cmdHandler)

	h := &lambdaHandler{
		signingSecret:  signingSecret,
		lambdaClient:   awslambda.NewFromConfig(cfg),
		cmdHandler:     cmdHandler,
		// reactionHandler: reactionHandler,
		shortcutHandler: shortcutHandler,
		mentionHandler:  mentionHandler,
	}

	log.Println("rollcall starting in HTTP (Lambda) mode...")
	lambda.Start(h.handleRequest)
	return nil
}

func (h *lambdaHandler) handleRequest(ctx context.Context, raw json.RawMessage) (events.APIGatewayV2HTTPResponse, error) {
	// Check if this is an async self-invocation
	var task asyncTask
	if err := json.Unmarshal(raw, &task); err == nil && task.Async {
		h.processAsync(task)
		return events.APIGatewayV2HTTPResponse{StatusCode: 200}, nil
	}

	// Otherwise, it's a Slack request via API Gateway
	var req events.APIGatewayV2HTTPRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		log.Printf("failed to parse API Gateway request: %v", err)
		return events.APIGatewayV2HTTPResponse{StatusCode: 400, Body: "bad request"}, nil
	}

	body := req.Body
	if req.IsBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			log.Printf("failed to decode base64 body: %v", err)
			return events.APIGatewayV2HTTPResponse{StatusCode: 400, Body: "bad request"}, nil
		}
		body = string(decoded)
	}

	timestamp := req.Headers["x-slack-request-timestamp"]
	signature := req.Headers["x-slack-signature"]

	if !verifySlackSignature(h.signingSecret, timestamp, body, signature) {
		log.Printf("signature verification failed: ts=%s sig=%s", timestamp, signature)
		return events.APIGatewayV2HTTPResponse{StatusCode: 401, Body: "invalid signature"}, nil
	}

	contentType := req.Headers["content-type"]

	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		return h.handleFormRequest(ctx, body)
	}

	return h.handleJSONRequest(ctx, body)
}

func (h *lambdaHandler) handleFormRequest(ctx context.Context, body string) (events.APIGatewayV2HTTPResponse, error) {
	values, err := url.ParseQuery(body)
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 400, Body: "bad request"}, nil
	}

	// Interactive component (shortcut / modal submission)
	if payload := values.Get("payload"); payload != "" {
		var callback slack.InteractionCallback
		if err := json.Unmarshal([]byte(payload), &callback); err != nil {
			log.Printf("failed to parse interaction payload: %v", err)
			return events.APIGatewayV2HTTPResponse{StatusCode: 400, Body: "bad payload"}, nil
		}

		switch callback.Type {
		case slack.InteractionTypeMessageAction:
			if callback.CallbackID == handler.ShortcutCallbackID {
				h.shortcutHandler.OpenModal(callback)
			}
		case slack.InteractionTypeViewSubmission:
			if callback.View.CallbackID == handler.ModalCallbackID {
				meta, forceNew, reminderAt, err := handler.ParseModalSubmission(callback)
				if err != nil {
					log.Printf("failed to parse modal submission: %v", err)
					return events.APIGatewayV2HTTPResponse{StatusCode: 200}, nil
				}
				log.Printf("modal submission received: reminderAt=%d (reminder feature not yet active)", reminderAt)
				h.invokeAsync(ctx, asyncTask{
					Async:     true,
					Type:      "modal_submission",
					ChannelID: meta.ChannelID,
					MessageTS: meta.MessageTS,
					UserID:    meta.UserID,
					ForceNew:  forceNew,
				})
			}
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 200}, nil
	}

	// Slash command
	if values.Get("command") != "" {
		cmd := parseSlashCommand(values)
		h.invokeAsync(ctx, asyncTask{
			Async:     true,
			Type:      "command",
			ChannelID: cmd.ChannelID,
			UserID:    cmd.UserID,
			Text:      cmd.Text,
		})
		return events.APIGatewayV2HTTPResponse{StatusCode: 200}, nil
	}

	return events.APIGatewayV2HTTPResponse{StatusCode: 400, Body: "unknown form request"}, nil
}

func (h *lambdaHandler) handleJSONRequest(ctx context.Context, body string) (events.APIGatewayV2HTTPResponse, error) {
	var envelope struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 400, Body: "bad json"}, nil
	}

	if envelope.Type == "url_verification" {
		return events.APIGatewayV2HTTPResponse{
			StatusCode: 200,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       fmt.Sprintf(`{"challenge":"%s"}`, envelope.Challenge),
		}, nil
	}

	if envelope.Type == "event_callback" {
		evt, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
		if err != nil {
			log.Printf("failed to parse event: %v", err)
			return events.APIGatewayV2HTTPResponse{StatusCode: 400, Body: "bad event"}, nil
		}

		// Reaction trigger disabled
		// if inner, ok := handler.ExtractReactionEvent(evt); ok && inner.Reaction == handler.TriggerReaction {
		// 	h.invokeAsync(ctx, asyncTask{
		// 		Async:     true,
		// 		Type:      "reaction",
		// 		ChannelID: inner.Item.Channel,
		// 		MessageTS: inner.Item.Timestamp,
		// 		UserID:    inner.User,
		// 	})
		// }
		if inner, ok := handler.ExtractMentionEvent(evt); ok && inner.ThreadTimeStamp == "" {
			h.invokeAsync(ctx, asyncTask{
				Async:     true,
				Type:      "mention",
				ChannelID: inner.Channel,
				MessageTS: inner.TimeStamp,
				UserID:    inner.User,
			})
		}

		return events.APIGatewayV2HTTPResponse{StatusCode: 200}, nil
	}

	return events.APIGatewayV2HTTPResponse{StatusCode: 200}, nil
}

func (h *lambdaHandler) invokeAsync(ctx context.Context, task asyncTask) {
	payload, err := json.Marshal(task)
	if err != nil {
		log.Printf("failed to marshal async task: %v", err)
		return
	}

	funcName := os.Getenv("AWS_LAMBDA_FUNCTION_NAME")
	_, err = h.lambdaClient.Invoke(ctx, &awslambda.InvokeInput{
		FunctionName:   &funcName,
		InvocationType: types.InvocationTypeEvent,
		Payload:        payload,
	})
	if err != nil {
		log.Printf("failed to invoke async: %v", err)
	}
}

func (h *lambdaHandler) processAsync(task asyncTask) {
	log.Printf("async processing: type=%s channel=%s ts=%s user=%s forceNew=%v", task.Type, task.ChannelID, task.MessageTS, task.UserID, task.ForceNew)

	switch task.Type {
	case "modal_submission":
		h.cmdHandler.RunCheck(task.ChannelID, task.MessageTS, task.UserID, nil, task.ForceNew)
	case "shortcut", "reaction", "mention":
		h.cmdHandler.RunCheck(task.ChannelID, task.MessageTS, task.UserID, nil, false)
	case "command":
		h.cmdHandler.Handle(slack.SlashCommand{
			ChannelID: task.ChannelID,
			UserID:    task.UserID,
			Text:      task.Text,
		})
	}
}

func parseSlashCommand(values url.Values) slack.SlashCommand {
	return slack.SlashCommand{
		Token:       values.Get("token"),
		TeamID:      values.Get("team_id"),
		ChannelID:   values.Get("channel_id"),
		ChannelName: values.Get("channel_name"),
		UserID:      values.Get("user_id"),
		UserName:    values.Get("user_name"),
		Command:     values.Get("command"),
		Text:        values.Get("text"),
		ResponseURL: values.Get("response_url"),
		TriggerID:   values.Get("trigger_id"),
	}
}

func verifySlackSignature(signingSecret, timestamp, body, expectedSig string) bool {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if math.Abs(float64(time.Now().Unix()-ts)) > 300 {
		return false
	}

	sigBasestring := "v0:" + timestamp + ":" + body
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte(sigBasestring))
	computed := "v0=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(computed), []byte(expectedSig))
}
