package main

import (
	"fmt"
	"os"
	"time"

	"github.com/pfernandom/chatui/chat"

	tui "github.com/grindlemire/go-tui"
)

var botGradient = tui.NewGradient(tui.BrightCyan, tui.BrightMagenta)

func main() {
	shell := chat.New(chat.Config{
		Instructions:        "Enter a message to send to the bot.",
		DefaultMultiline:    true,
		HandleResponse:      handleResponse,
		SlashCommandHandler: slashCommandHandler,
		SlashCommandNames:   []string{"help", "clear"},
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

func slashCommandHandler(app *chat.App, sc chat.SlashCommand) (chat.SlashResponse, error) {
	switch sc.Name {
	case "help":
		if len(sc.Args) == 0 {
			return sc.NewResponse("Help: available commands are: help, clear"), nil
		}
		return sc.NewResponse(fmt.Sprintf("Help: %s", sc.Args)), nil
	case "clear":
		if term := app.Terminal(); term != nil {
			term.Clear()
		}
		return sc.Handled(), nil
	}
	return sc.Forward(), nil
}
