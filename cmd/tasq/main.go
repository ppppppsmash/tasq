package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/kurosawa-dev/tasq/internal/handler"
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

	appToken := os.Getenv("SLACK_APP_TOKEN")
	botToken := os.Getenv("SLACK_BOT_TOKEN")

	if appToken == "" || botToken == "" {
		return fmt.Errorf("SLACK_APP_TOKEN and SLACK_BOT_TOKEN must be set")
	}

	api := slack.New(botToken,
		slack.OptionAppLevelToken(appToken),
		slack.OptionLog(log.New(os.Stdout, "slack: ", log.LstdFlags)),
	)

	client := socketmode.New(api,
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.LstdFlags)),
	)

	cmdHandler := handler.NewCommandHandler(client)
	reactionHandler := handler.NewReactionHandler(client, cmdHandler)

	go handleEvents(client, cmdHandler, reactionHandler)

	log.Println("tasq starting...")
	return client.Run()
}

func handleEvents(client *socketmode.Client, cmdHandler *handler.CommandHandler, reactionHandler *handler.ReactionHandler) {
	for evt := range client.Events {
		switch evt.Type {
		case socketmode.EventTypeSlashCommand:
			cmd, ok := evt.Data.(slack.SlashCommand)
			if !ok {
				continue
			}
			go cmdHandler.Handle(evt, cmd)

		case socketmode.EventTypeEventsAPI:
			eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				continue
			}
			client.Ack(*evt.Request)

			if inner, ok := handler.ExtractReactionEvent(eventsAPIEvent); ok {
				go reactionHandler.Handle(evt, inner)
			}

		case socketmode.EventTypeConnecting:
			log.Println("connecting to Slack...")
		case socketmode.EventTypeConnected:
			log.Println("connected to Slack")
		case socketmode.EventTypeConnectionError:
			log.Println("connection error")
		}
	}
}
