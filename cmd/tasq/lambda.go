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

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/kurosawa-dev/rollcall/internal/handler"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

type lambdaHandler struct {
	signingSecret   string
	cmdHandler      *handler.CommandHandler
	reactionHandler *handler.ReactionHandler
	shortcutHandler *handler.ShortcutHandler
}

func runLambda(api *slack.Client) error {
	signingSecret := os.Getenv("SLACK_SIGNING_SECRET")
	if signingSecret == "" {
		return fmt.Errorf("SLACK_SIGNING_SECRET must be set for HTTP mode")
	}

	cmdHandler := handler.NewCommandHandler(api)
	reactionHandler := handler.NewReactionHandler(api, cmdHandler)
	shortcutHandler := handler.NewShortcutHandler(cmdHandler)

	h := &lambdaHandler{
		signingSecret:   signingSecret,
		cmdHandler:      cmdHandler,
		reactionHandler: reactionHandler,
		shortcutHandler: shortcutHandler,
	}

	log.Println("rollcall starting in HTTP (Lambda) mode...")
	lambda.Start(h.handleRequest)
	return nil
}

func (h *lambdaHandler) handleRequest(_ context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	body := req.Body
	if req.IsBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			log.Printf("failed to decode base64 body: %v", err)
			return events.APIGatewayV2HTTPResponse{StatusCode: 400, Body: "bad request"}, nil
		}
		body = string(decoded)
	}

	log.Printf("request: headers=%v body=%s", req.Headers, body)

	timestamp := req.Headers["x-slack-request-timestamp"]
	signature := req.Headers["x-slack-signature"]

	if !verifySlackSignature(h.signingSecret, timestamp, body, signature) {
		log.Printf("signature verification failed: ts=%s sig=%s", timestamp, signature)
		return events.APIGatewayV2HTTPResponse{StatusCode: 401, Body: "invalid signature"}, nil
	}

	contentType := req.Headers["content-type"]

	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		return h.handleFormRequest(body)
	}

	return h.handleJSONRequest(body)
}

func (h *lambdaHandler) handleFormRequest(body string) (events.APIGatewayV2HTTPResponse, error) {
	values, err := url.ParseQuery(body)
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 400, Body: "bad request"}, nil
	}

	// Interactive component (shortcut)
	if payload := values.Get("payload"); payload != "" {
		var callback slack.InteractionCallback
		if err := json.Unmarshal([]byte(payload), &callback); err != nil {
			log.Printf("failed to parse interaction payload: %v", err)
			return events.APIGatewayV2HTTPResponse{StatusCode: 400, Body: "bad payload"}, nil
		}
		h.shortcutHandler.Handle(callback)
		return events.APIGatewayV2HTTPResponse{StatusCode: 200}, nil
	}

	// Slash command
	if values.Get("command") != "" {
		cmd := parseSlashCommand(values)
		h.cmdHandler.Handle(cmd)
		return events.APIGatewayV2HTTPResponse{StatusCode: 200}, nil
	}

	return events.APIGatewayV2HTTPResponse{StatusCode: 400, Body: "unknown form request"}, nil
}

func (h *lambdaHandler) handleJSONRequest(body string) (events.APIGatewayV2HTTPResponse, error) {
	// URL verification challenge
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

	// Events API callback
	if envelope.Type == "event_callback" {
		evt, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
		if err != nil {
			log.Printf("failed to parse event: %v", err)
			return events.APIGatewayV2HTTPResponse{StatusCode: 400, Body: "bad event"}, nil
		}

		if inner, ok := handler.ExtractReactionEvent(evt); ok {
			h.reactionHandler.Handle(inner)
		}

		return events.APIGatewayV2HTTPResponse{StatusCode: 200}, nil
	}

	return events.APIGatewayV2HTTPResponse{StatusCode: 200}, nil
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
