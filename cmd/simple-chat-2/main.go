package main

import (
	"fmt"
	"os"
	"time"

	"github.com/pfernandom/chatui/chat"

	tui "github.com/pfernandom/go-tui"
)

var botGradient = tui.NewGradient(tui.BrightCyan, tui.BrightMagenta)

var skills = map[string]string{
	"discover": "Discover the world",
	"learn":    "Learn about the world",
	"create":   "Create something new",
	"share":    "Share something with the world",
	"help":     "Help: available commands are: help, clear, exit",
}

func main() {
	slashCommands := map[string]chat.SlashCommandConfig{
		"help": &chat.TransformCommand{
			Transform: func(sc chat.SlashCommand) (string, error) {
				return "Help: available commands are: help, clear", nil
			},
		},
		"clear": &chat.ExecuteCommand{
			Execute: func(app *chat.App, sc chat.SlashCommand) error {
				if term := app.Terminal(); term != nil {
					term.Clear()
				}
				return nil
			},
		},
		"exit": &chat.ExecuteCommand{
			Execute: func(app *chat.App, sc chat.SlashCommand) error {
				app.Close()
				return nil
			},
		},
	}

	for name, skill := range skills {
		slashCommands[name] = &chat.TransformCommand{
			Transform: func(sc chat.SlashCommand) (string, error) {
				return skill, nil
			},
		}
	}
	shell := chat.New(chat.Config{
		Instructions:     "Enter a message to send to the bot.",
		DefaultMultiline: true,
		HandleResponse:   handleResponse,
		SlashCommands:    slashCommands,
	})

	app, err := shell.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer app.Close()

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func handleResponse(req *chat.Request) error {
	reply := fmt.Sprintf(
		"Bot: streaming this reply with StreamAbove(). You sent %q. The composer remains pinned below while this text is written into the history region above it.\n",
		req.Input,
	)
	req.SetStatus("Streaming reply above the input field...")

	gradient := botGradient
	if req.FromSlash {
		gradient = tui.NewGradient(tui.BrightRed, tui.BrightYellow)
	}

	for _, r := range reply {
		if _, err := req.Stream.WriteGradient(string(r), gradient); err != nil {
			return err
		}
		time.Sleep(14 * time.Millisecond)
	}
	req.SetStatus("Reply streamed")

	return nil
}
