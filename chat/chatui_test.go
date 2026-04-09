package chat

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	tui "github.com/pfernandom/go-tui"
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
		in       string
		wantOK   bool
		wantName string
		wantArgs string
		wantRaw  string
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

func TestSlashResponseHelpers(t *testing.T) {
	sc := SlashCommand{Raw: "/foo bar", Name: "foo", Args: "bar"}
	nr := sc.NewResponse("transformed")
	if nr.Handled || nr.Response != "transformed" || nr.Command.Name != "foo" {
		t.Fatalf("NewResponse: %+v", nr)
	}
	fw := sc.Forward()
	if fw.Handled || fw.Response != "/foo bar" || fw.Command.Raw != "/foo bar" {
		t.Fatalf("Forward: %+v", fw)
	}
	hd := sc.Handled()
	if !hd.Handled || hd.Command.Name != "foo" {
		t.Fatalf("Handled: %+v", hd)
	}
	var zero SlashResponse
	if !zero.IsFallthrough() || !(SlashResponse{}).IsFallthrough() {
		t.Fatal("zero SlashResponse should be fallthrough")
	}
	if (SlashResponse{Handled: true}).IsFallthrough() {
		t.Fatal("non-zero should not be fallthrough")
	}
}

func TestDispatchSlashCommand(t *testing.T) {
	shell := New(Config{
		SlashCommands: map[string]SlashCommandConfig{
			"local": &ExecuteCommand{
				Execute: func(_ *App, _ SlashCommand) error { return nil },
			},
		},
	})

	resp, _, path, err := shell.dispatchSlashCommand("/local")
	if err != nil || !path || resp.IsFallthrough() || !resp.Handled {
		t.Fatalf("dispatch /local: resp=%+v path=%v err=%v", resp, path, err)
	}

	resp, _, path, err = shell.dispatchSlashCommand("/LOCAL")
	if err != nil || !path || resp.IsFallthrough() || !resp.Handled {
		t.Fatalf("dispatch /LOCAL (case): resp=%+v path=%v err=%v", resp, path, err)
	}

	resp, _, path, err = shell.dispatchSlashCommand("/other")
	if err != nil || !path || !resp.IsFallthrough() {
		t.Fatalf("dispatch /other: resp=%+v path=%v err=%v", resp, path, err)
	}

	resp, _, path, err = shell.dispatchSlashCommand("plain")
	if err != nil || path || !resp.IsFallthrough() {
		t.Fatalf("non-slash: resp=%+v path=%v", resp, path)
	}

	errShell := New(Config{
		SlashCommands: map[string]SlashCommandConfig{
			"x": &TransformCommand{
				Transform: func(SlashCommand) (string, error) {
					return "", errors.New("slash err")
				},
			},
		},
	})
	resp, _, path, err = errShell.dispatchSlashCommand("/x")
	if err == nil || !path {
		t.Fatalf("dispatch error: err=%v path=%v", err, path)
	}
	_ = resp

	none := New(Config{})
	resp, _, path, err = none.dispatchSlashCommand("/x")
	if err != nil || !path || !resp.IsFallthrough() {
		t.Fatalf("without SlashCommands: resp=%+v path=%v err=%v", resp, path, err)
	}
}

func TestSlashCommandsNamesMergedAndSorted(t *testing.T) {
	shell := New(Config{
		SlashCommandNames: []string{"zebra"},
		SlashCommands: map[string]SlashCommandConfig{
			"apple":  &ExecuteCommand{Execute: func(*App, SlashCommand) error { return nil }},
			"Banana": &ExecuteCommand{Execute: func(*App, SlashCommand) error { return nil }},
		},
	})
	want := []string{"apple", "banana", "zebra"}
	if !slices.Equal(shell.config.SlashCommandNames, want) {
		t.Fatalf("SlashCommandNames = %v, want %v", shell.config.SlashCommandNames, want)
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

func TestApp_PrintAbovelnBeforeBindNoPanic(t *testing.T) {
	shell := New(Config{HandleResponse: func(*Request) error { return nil }})
	shell.PrintAboveln("%s", "a")
	shell.QueuePrintAboveln("%s", "b")
}

func TestDefaultRenderUserMessageDivider(t *testing.T) {
	got := defaultRenderUserMessage("hi")
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), got)
	}
	if lines[0] != "You: hi" {
		t.Fatalf("line0 = %q", lines[0])
	}
	if len(lines[1]) < userMessageDividerMinWidth {
		t.Fatalf("divider too short: %q", lines[1])
	}
	for _, r := range lines[1] {
		if r != '─' {
			t.Fatalf("divider should be light horizontal rules, got %q", lines[1])
		}
	}
}

func TestCountWrappedLines(t *testing.T) {
	tests := []struct {
		s     string
		limit int
		want  int
	}{
		{"", 10, 0},
		{"hello", 10, 1},
		{"abcdefghij", 10, 1},
		{"abcdefghijk", 10, 2},
		{"a\nb", 10, 2},
	}
	for _, tt := range tests {
		if got := countWrappedLines(tt.s, tt.limit); got != tt.want {
			t.Errorf("countWrappedLines(%q, %d) = %d, want %d", tt.s, tt.limit, got, tt.want)
		}
	}
}

func TestComputeInlineHeightForTerminal_clampMinMax(t *testing.T) {
	longInstr := strings.Repeat("word ", 400)
	shell := New(normalizeConfig(Config{
		Instructions:     longInstr,
		CompactHeight:    5,
		MultilineHeight:  12,
		DefaultMultiline: true,
		HandleResponse:   func(*Request) error { return nil },
	}))
	// Narrow terminal => heavy wrapping; natural height >> 12; max clamp applies.
	h := shell.computeInlineHeightForTerminal(32, 100)
	if h != 12 {
		t.Fatalf("expected max clamp 12, got %d", h)
	}

	shellSmall := New(normalizeConfig(Config{
		Instructions:     "x",
		CompactHeight:    25,
		MultilineHeight:  40,
		DefaultMultiline: true,
		HandleResponse:   func(*Request) error { return nil },
	}))
	hSmall := shellSmall.computeInlineHeightForTerminal(120, 100)
	if hSmall < 25 {
		t.Fatalf("expected min clamp 25, got %d", hSmall)
	}
}

func TestComputeInlineHeightForTerminal_compactModeMaxIsCompactHeight(t *testing.T) {
	shell := New(normalizeConfig(Config{
		Instructions:     "x",
		CompactHeight:    9,
		MultilineHeight:  30,
		DefaultMultiline: false,
		HandleResponse:   func(*Request) error { return nil },
	}))
	h := shell.computeInlineHeightForTerminal(200, 100)
	if h > 9 {
		t.Fatalf("compact mode max should be CompactHeight, got %d", h)
	}
}

func TestComputeInlineHeightForTerminal_capsToTerminalHeight(t *testing.T) {
	shell := New(normalizeConfig(Config{
		Instructions:     "x",
		CompactHeight:    4,
		MultilineHeight:  80,
		DefaultMultiline: true,
		HandleResponse:   func(*Request) error { return nil },
	}))
	h := shell.computeInlineHeightForTerminal(200, 14)
	if h > 14 {
		t.Fatalf("expected cap at terminal height 14, got %d", h)
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
