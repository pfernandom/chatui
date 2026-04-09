package main

import (
	"fmt"
	"os"
	"time"

	"github.com/pfernandom/chatui/chat"

	tui "github.com/pfernandom/go-tui"
)

var botGradient = tui.NewGradient(tui.BrightCyan, tui.BrightMagenta)

func main() {
	shell := chat.New(chat.Config{
		Title:            "Inline Chat Demo",
		DefaultMultiline: true,
		HandleResponse:   handleResponse,
		SettingsView:     settingsView,
		HelpView:         helpView,
	})

	app, err := tui.NewApp(
		tui.WithInlineHeight(9),
		tui.WithRootComponent(shell),
	)
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

	for _, r := range reply {
		if _, err := req.Stream.WriteGradient(string(r), botGradient); err != nil {
			return err
		}
		time.Sleep(14 * time.Millisecond)
	}

	return nil
}

func settingsView(_ *chat.App) *tui.Element {
	root := tui.New(
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Column),
		tui.WithPadding(1),
		tui.WithHeightPercent(100),
		tui.WithBorder(tui.BorderRounded),
		tui.WithBorderStyle(tui.NewStyle().Foreground(tui.Yellow)),
	)
	root.AddChild(tui.New(
		tui.WithText("Settings Overlay"),
		tui.WithTextStyle(tui.NewStyle().Bold().Foreground(tui.Yellow)),
	))
	root.AddChild(tui.New(
		tui.WithText("This screen uses EnterAlternateScreen(), so it can take over the full terminal temporarily."),
	))
	root.AddChild(tui.New(
		tui.WithText("Press Escape to return to inline mode. The scrollback above the widget will still be there."),
		tui.WithTextStyle(tui.NewStyle().Dim()),
	))
	return root
}

func helpView(_ *chat.App) *tui.Element {
	root := tui.New(
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Column),
		tui.WithPadding(1),
		tui.WithHeightPercent(100),
		tui.WithBorder(tui.BorderRounded),
		tui.WithBorderStyle(tui.NewStyle().Foreground(tui.Cyan)),
	)
	root.AddChild(tui.New(
		tui.WithText("Help"),
		tui.WithTextStyle(tui.NewStyle().Bold().Foreground(tui.Cyan)),
	))
	root.AddChild(tui.New(
		tui.WithText("Enter submits a prompt, Tab toggles compact mode, Ctrl+S opens settings, and F1 opens this help screen."),
	))
	root.AddChild(tui.New(
		tui.WithText("The assistant stream is produced by the demo's custom HandleResponse callback."),
		tui.WithTextStyle(tui.NewStyle().Dim()),
	))
	return root
}
