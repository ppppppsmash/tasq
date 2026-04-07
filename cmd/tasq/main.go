package main

import (
	"fmt"
	"log"
	"os"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
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

	go handleEvents(client)

	log.Println("tasq starting...")
	return client.Run()
}

func handleEvents(client *socketmode.Client) {
	for evt := range client.Events {
		switch evt.Type {
		case socketmode.EventTypeConnecting:
			log.Println("connecting to Slack...")
		case socketmode.EventTypeConnected:
			log.Println("connected to Slack")
		case socketmode.EventTypeConnectionError:
			log.Println("connection error")
		default:
			log.Printf("event: %s\n", evt.Type)
		}
	}
}
