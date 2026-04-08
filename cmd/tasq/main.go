package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/kurosawa-dev/rollcall/internal/handler"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()

	botToken := os.Getenv("SLACK_BOT_TOKEN")
	if botToken == "" {
		return fmt.Errorf("SLACK_BOT_TOKEN must be set")
	}

	mode := os.Getenv("APP_MODE")

	if mode == "http" {
		api := slack.New(botToken)
		return runLambda(api)
	}

	// SocketMode (default)
	appToken := os.Getenv("SLACK_APP_TOKEN")
	if appToken == "" {
		return fmt.Errorf("SLACK_APP_TOKEN must be set for socket mode")
	}

	api := slack.New(botToken,
		slack.OptionAppLevelToken(appToken),
		slack.OptionLog(log.New(os.Stdout, "slack: ", log.LstdFlags)),
	)

	client := socketmode.New(api,
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.LstdFlags)),
	)

	cmdHandler := handler.NewCommandHandler(api)
	reactionHandler := handler.NewReactionHandler(api, cmdHandler)
	shortcutHandler := handler.NewShortcutHandler(cmdHandler)

	go handleEvents(client, cmdHandler, reactionHandler, shortcutHandler)

	log.Println("rollcall starting...")
	return client.Run()
}

func handleEvents(client *socketmode.Client, cmdHandler *handler.CommandHandler, reactionHandler *handler.ReactionHandler, shortcutHandler *handler.ShortcutHandler) {
	for evt := range client.Events {
		switch evt.Type {
		case socketmode.EventTypeSlashCommand:
			cmd, ok := evt.Data.(slack.SlashCommand)
			if !ok {
				continue
			}
			client.Ack(*evt.Request)
			go cmdHandler.Handle(cmd)

		case socketmode.EventTypeEventsAPI:
			eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				continue
			}
			client.Ack(*evt.Request)

			if inner, ok := handler.ExtractReactionEvent(eventsAPIEvent); ok {
				go reactionHandler.Handle(inner)
			}

		case socketmode.EventTypeInteractive:
			callback, ok := evt.Data.(slack.InteractionCallback)
			if !ok {
				continue
			}
			client.Ack(*evt.Request)
			go shortcutHandler.Handle(callback)

		case socketmode.EventTypeConnecting:
			log.Println("connecting to Slack...")
		case socketmode.EventTypeConnected:
			log.Println("connected to Slack")
		case socketmode.EventTypeConnectionError:
			log.Println("connection error")
		}
	}
}
