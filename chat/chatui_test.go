package chat

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	tui "github.com/grindlemire/go-tui"
)

func TestNormalizeConfigDefaults(t *testing.T) {
	cfg := normalizeConfig(Config{})

	if cfg.Title != "" {
		t.Fatalf("Title = %q, want empty string", cfg.Title)
	}
	if cfg.Placeholder != defaultPlaceholder {
		t.Fatalf("Placeholder = %q, want %q", cfg.Placeholder, defaultPlaceholder)
	}
	if cfg.CompactHeight != defaultCompactHeight {
		t.Fatalf("CompactHeight = %d, want %d", cfg.CompactHeight, defaultCompactHeight)
	}
	if cfg.MultilineHeight != defaultMultilineHeight {
		t.Fatalf("MultilineHeight = %d, want %d", cfg.MultilineHeight, defaultMultilineHeight)
	}
	if cfg.HandleResponse == nil {
		t.Fatal("HandleResponse should be defaulted")
	}
	if cfg.RenderUserMessage == nil {
		t.Fatal("RenderUserMessage should be defaulted")
	}
	if cfg.RenderError == nil {
		t.Fatal("RenderError should be defaulted")
	}
	if cfg.SlashCommandNames != nil {
		t.Fatalf("SlashCommandNames = %v, want nil", cfg.SlashCommandNames)
	}
}

func TestNormalizeConfigKeepsMultilineHeightAtLeastCompact(t *testing.T) {
	cfg := normalizeConfig(Config{
		CompactHeight:   12,
		MultilineHeight: 3,
	})

	if cfg.MultilineHeight < cfg.CompactHeight {
		t.Fatalf("MultilineHeight = %d, CompactHeight = %d", cfg.MultilineHeight, cfg.CompactHeight)
	}
}

func TestNewPreservesExplicitTitleAndAllowsEmptyTitle(t *testing.T) {
	empty := New(Config{})
	if empty.config.Title != "" {
		t.Fatalf("empty title = %q, want empty string", empty.config.Title)
	}

	named := New(Config{Title: "Demo"})
	if named.config.Title != "Demo" {
		t.Fatalf("named title = %q, want %q", named.config.Title, "Demo")
	}
}

func TestDefaultHandleResponseWritesFallbackMessage(t *testing.T) {
	cfg := normalizeConfig(Config{})
	stream := &recordingStream{}
	req := &Request{
		Context: context.Background(),
		Input:   "hello",
		Stream:  stream,
	}

	if err := cfg.HandleResponse(req); err != nil {
		t.Fatalf("HandleResponse returned error: %v", err)
	}
	if !strings.Contains(stream.buf.String(), "No response handler configured.") {
		t.Fatalf("fallback handler output = %q", stream.buf.String())
	}
}

func TestRequestStreamRejectsCanceledWrites(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rs := newRequestStream(ctx, &recordingStream{})

	if _, err := rs.Write([]byte("hello")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Write error = %v, want context.Canceled", err)
	}
	if _, err := rs.WriteStyled("hello", tui.NewStyle()); !errors.Is(err, context.Canceled) {
		t.Fatalf("WriteStyled error = %v, want context.Canceled", err)
	}
	if _, err := rs.WriteGradient("hello", tui.NewGradient(tui.Red, tui.Blue)); !errors.Is(err, context.Canceled) {
		t.Fatalf("WriteGradient error = %v, want context.Canceled", err)
	}
}

func TestRequestStreamCloseIsIdempotent(t *testing.T) {
	inner := &recordingStream{}
	rs := newRequestStream(context.Background(), inner)

	if err := rs.Close(); err != nil {
		t.Fatalf("first Close error: %v", err)
	}
	if err := rs.Close(); err != nil {
		t.Fatalf("second Close error: %v", err)
	}
	if _, err := rs.Write([]byte("hello")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Write after Close error = %v, want io.ErrClosedPipe", err)
	}
	if !inner.closed {
		t.Fatal("inner stream should be closed")
	}
}

func TestRequestSetStatusUpdatesActiveStatus(t *testing.T) {
	shell := New(Config{})
	shell.queueUpdate = func(fn func()) { fn() }

	ctx := context.Background()
	req := &Request{
		Context: ctx,
		Shell:   shell,
		id:      7,
	}

	shell.activeReqID = 7
	req.SetStatus("Preparing reply...")

	if got := shell.statusText(); got != "Preparing reply..." {
		t.Fatalf("statusText() = %q, want %q", got, "Preparing reply...")
	}
}

func TestRequestSetStatusIgnoredForCanceledRequest(t *testing.T) {
	shell := New(Config{})
	shell.queueUpdate = func(fn func()) { fn() }
	shell.activeReqID = 9

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := &Request{
		Context: ctx,
		Shell:   shell,
		id:      9,
	}

	req.SetStatus("should not apply")

	if got := shell.statusText(); got != "Ready. Submit a message to stream a response above the widget." {
		t.Fatalf("statusText() = %q", got)
	}
}

func TestRequestSetStatusIgnoredForSupersededRequest(t *testing.T) {
	shell := New(Config{})
	shell.queueUpdate = func(fn func()) { fn() }
	shell.activeReqID = 11

	req := &Request{
		Context: context.Background(),
		Shell:   shell,
		id:      10,
	}

	req.SetStatus("old request")

	if got := shell.statusText(); got != "Ready. Submit a message to stream a response above the widget." {
		t.Fatalf("statusText() = %q", got)
	}
}

func TestNormalizeSlashNamesDedupesAndSorts(t *testing.T) {
	cfg := normalizeConfig(Config{
		SlashCommandNames: []string{"Foo", " foo ", "bar", "Foo", ""},
	})
	want := []string{"bar", "foo"}
	if !slices.Equal(cfg.SlashCommandNames, want) {
		t.Fatalf("SlashCommandNames = %v, want %v", cfg.SlashCommandNames, want)
	}
}

func TestParseSlashCommand(t *testing.T) {
	tests := []struct {
		in         string
		wantOK     bool
		wantName   string
		wantArgs   string
		wantRaw    string
	}{
		{"hello", false, "", "", ""},
		{"/", true, "", "", "/"},
		{"/foo", true, "foo", "", "/foo"},
		{"/FOO bar", true, "FOO", "bar", "/FOO bar"},
		{"/a  b  c", true, "a", "b  c", "/a  b  c"},
	}
	for _, tt := range tests {
		sc, ok := ParseSlashCommand(tt.in)
		if ok != tt.wantOK {
			t.Fatalf("ParseSlashCommand(%q) ok = %v, want %v", tt.in, ok, tt.wantOK)
		}
		if !tt.wantOK {
			continue
		}
		if sc.Name != tt.wantName || sc.Args != tt.wantArgs || sc.Raw != tt.wantRaw {
			t.Fatalf("ParseSlashCommand(%q) = %+v, want Name=%q Args=%q Raw=%q", tt.in, sc, tt.wantName, tt.wantArgs, tt.wantRaw)
		}
	}
}

func TestActiveLineLastAndReplaceActiveLine(t *testing.T) {
	if got := activeLineLast(""); got != "" {
		t.Fatalf("activeLineLast(\"\") = %q", got)
	}
	if got := activeLineLast("a\n/b"); got != "/b" {
		t.Fatalf("activeLineLast = %q, want /b", got)
	}
	if got := replaceActiveLine("x\n/y", "/z"); got != "x\n/z" {
		t.Fatalf("replaceActiveLine = %q", got)
	}
	if got := replaceActiveLine("/only", "/x"); got != "/x" {
		t.Fatalf("replaceActiveLine single = %q", got)
	}
}

func TestTrySlashTabComplete(t *testing.T) {
	shell := New(Config{SlashCommandNames: []string{"hello", "help"}})
	shell.textarea.SetText("/h")
	if !shell.trySlashTabComplete() {
		t.Fatal("expected Tab completion")
	}
	if got := shell.textarea.Text(); got != "/hel" {
		t.Fatalf("Text() = %q, want /hel", got)
	}
	if !shell.trySlashTabComplete() {
		t.Fatal("expected second Tab cycle")
	}
	if got := shell.textarea.Text(); got != "/hello" {
		t.Fatalf("Text() = %q, want /hello", got)
	}
}

func TestSlashTabStep(t *testing.T) {
	t.Run("singleExpand", func(t *testing.T) {
		got, _ := slashTabStep("hel", []string{"hello"}, "hello", 0)
		if got != "/hello" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("singleSpaceWhenComplete", func(t *testing.T) {
		got, _ := slashTabStep("hello", []string{"hello"}, "hello", 0)
		if got != "/hello " {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("multiLCP", func(t *testing.T) {
		got, c := slashTabStep("h", []string{"hello", "help"}, "hel", 0)
		if got != "/hel" || c != 0 {
			t.Fatalf("got %q cycle %d", got, c)
		}
	})
	t.Run("multiCycle", func(t *testing.T) {
		got, c := slashTabStep("hel", []string{"hello", "help"}, "hel", 0)
		if got != "/hello" || c != 1 {
			t.Fatalf("got %q cycle %d", got, c)
		}
		got2, c2 := slashTabStep("hel", []string{"hello", "help"}, "hel", 1)
		if got2 != "/help" || c2 != 2 {
			t.Fatalf("got2 %q cycle %d", got2, c2)
		}
	})
}

func TestDispatchSlashCommand(t *testing.T) {
	shell := New(Config{
		SlashCommandNames: []string{"local"},
		SlashCommandHandler: func(_ *App, sc SlashCommand) (bool, error) {
			if sc.Name == "local" {
				return true, nil
			}
			return false, nil
		},
	})

	stop, err := shell.dispatchSlashCommand("/local")
	if !stop || err != nil {
		t.Fatalf("dispatch /local: stop=%v err=%v", stop, err)
	}

	stop, err = shell.dispatchSlashCommand("/other")
	if stop || err != nil {
		t.Fatalf("dispatch /other: stop=%v err=%v", stop, err)
	}

	stop, err = shell.dispatchSlashCommand("plain")
	if stop {
		t.Fatalf("non-slash: stop=%v", stop)
	}

	errShell := New(Config{
		SlashCommandHandler: func(_ *App, _ SlashCommand) (bool, error) {
			return false, errors.New("slash err")
		},
	})
	stop, err = errShell.dispatchSlashCommand("/x")
	if !stop || err == nil {
		t.Fatalf("dispatch error: stop=%v err=%v", stop, err)
	}

	none := New(Config{})
	stop, err = none.dispatchSlashCommand("/x")
	if stop || err != nil {
		t.Fatalf("without handler: stop=%v err=%v", stop, err)
	}
}

func TestCompleteRequestClearsCustomStatus(t *testing.T) {
	shell := New(Config{})
	shell.streaming.Set(true)
	shell.activeReqID = 12
	shell.statusOverride = "Streaming custom output..."

	if ok := shell.completeRequest(12); !ok {
		t.Fatal("completeRequest should succeed")
	}
	if got := shell.statusText(); got != "Streaming response..." {
		t.Fatalf("statusText() after completion = %q", got)
	}

	shell.streaming.Set(false)
	if got := shell.statusText(); got != "Ready. Submit a message to stream a response above the widget." {
		t.Fatalf("idle statusText() = %q", got)
	}
}

type recordingStream struct {
	buf    bytes.Buffer
	closed bool
}

func (r *recordingStream) Write(p []byte) (int, error) {
	return r.buf.Write(p)
}

func (r *recordingStream) Close() error {
	r.closed = true
	return nil
}

func (r *recordingStream) WriteStyled(text string, style tui.Style) (int, error) {
	return r.buf.WriteString(text)
}

func (r *recordingStream) WriteGradient(text string, g tui.Gradient, base ...tui.Style) (int, error) {
	return r.buf.WriteString(text)
}

func (r *recordingStream) WriteElement(v tui.Viewable) {}
