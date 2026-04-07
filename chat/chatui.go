package chat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	tui "github.com/grindlemire/go-tui"
)

const (
	defaultPlaceholder     = "Type a message. Enter sends. Ctrl+J adds a newline."
	defaultCompactHeight   = 6
	defaultMultilineHeight = 9
)

type Stream interface {
	io.WriteCloser
	WriteStyled(text string, style tui.Style) (int, error)
	WriteGradient(text string, g tui.Gradient, base ...tui.Style) (int, error)
	WriteElement(v tui.Viewable)
}

type Request struct {
	Context     context.Context
	Input       string
	Stream      Stream
	TerminalApp *tui.App
	Shell       *App

	id uint64
}

func (r *Request) SetStatus(text string) {
	if r == nil || r.Shell == nil {
		return
	}
	r.Shell.setRequestStatus(r.id, r.Context, text)
}

type ResponseHandler func(*Request) error
type MessageRenderer func(input string) string
type ErrorRenderer func(err error) string
type OverlayView func(*App) *tui.Element

type Config struct {
	Title            string
	Placeholder      string
	CompactHeight    int
	MultilineHeight  int
	DefaultMultiline bool
	TextAreaOptions  []tui.TextAreaOption

	HandleResponse    ResponseHandler
	RenderUserMessage MessageRenderer
	RenderError       ErrorRenderer

	Instructions string

	SettingsView OverlayView
	HelpView     OverlayView
}

type App struct {
	config Config

	app          *tui.App
	showSettings *tui.State[bool]
	showHelp     *tui.State[bool]
	multiline    *tui.State[bool]
	streaming    *tui.State[bool]
	textarea     *tui.TextArea

	mu             sync.Mutex
	activeCancel   context.CancelFunc
	activeReqID    uint64
	nextReqID      uint64
	statusOverride string

	instructions string
	queueUpdate  func(func())
}

func New(config Config) *App {
	cfg := normalizeConfig(config)

	a := &App{
		config:       cfg,
		showSettings: tui.NewState(false),
		showHelp:     tui.NewState(false),
		multiline:    tui.NewState(cfg.DefaultMultiline),
		streaming:    tui.NewState(false),
		instructions: cfg.Instructions,
	}
	a.queueUpdate = func(fn func()) {
		if fn != nil {
			fn()
		}
	}

	opts := []tui.TextAreaOption{
		tui.WithTextAreaWidth(60),
		tui.WithTextAreaMaxHeight(4),
		tui.WithTextAreaBorder(tui.BorderRounded),
		tui.WithTextAreaPlaceholder(cfg.Placeholder),
		tui.WithTextAreaAutoFocus(true),
	}
	opts = append(opts, cfg.TextAreaOptions...)
	opts = append(opts, tui.WithTextAreaOnSubmit(a.send))
	a.textarea = tui.NewTextArea(opts...)

	return a
}

func (a *App) Start(opts ...tui.AppOption) (*tui.App, error) {
	opts = append(opts, tui.WithInlineHeight(9))
	opts = append(opts, tui.WithRootComponent(a))
	return tui.NewApp(opts...)
}

func (a *App) Component() tui.Component {
	return a
}

func (a *App) BindApp(app *tui.App) {
	a.app = app
	a.queueUpdate = app.QueueUpdate
	a.showSettings.BindApp(app)
	a.showHelp.BindApp(app)
	a.multiline.BindApp(app)
	a.streaming.BindApp(app)
	a.textarea.BindApp(app)
}

func (a *App) KeyMap() tui.KeyMap {
	if a.showSettings.Get() || a.showHelp.Get() {
		return tui.KeyMap{
			tui.OnStop(tui.KeyEscape, func(ke tui.KeyEvent) { a.closeOverlay() }),
			tui.OnStop(tui.KeyCtrlC, func(ke tui.KeyEvent) { ke.App().Stop() }),
		}
	}

	km := a.textarea.KeyMap()
	km = append(km,
		tui.OnStop(tui.KeyTab, func(ke tui.KeyEvent) { a.toggleMultiline() }),
		tui.OnStop(tui.KeyEscape, func(ke tui.KeyEvent) { ke.App().Stop() }),
		tui.OnStop(tui.KeyCtrlC, func(ke tui.KeyEvent) { ke.App().Stop() }),
	)
	if a.config.SettingsView != nil {
		km = append(km, tui.OnStop(tui.KeyCtrlS, func(ke tui.KeyEvent) { a.openSettings() }))
	}
	if a.config.HelpView != nil {
		km = append(km, tui.OnStop(tui.KeyF1, func(ke tui.KeyEvent) { a.openHelp() }))
	}
	return km
}

func (a *App) Watchers() []tui.Watcher {
	return a.textarea.Watchers()
}

func (a *App) Render(app *tui.App) *tui.Element {
	if a.showSettings.Get() {
		return a.renderOverlay(a.config.SettingsView, "Settings")
	}
	if a.showHelp.Get() {
		return a.renderOverlay(a.config.HelpView, "Help")
	}

	app.SetInlineHeight(a.inlineHeight())

	root := tui.New(
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Column),
		tui.WithPadding(1),
		tui.WithHeightPercent(100),
	)

	if a.config.Title != "" {
		root.AddChild(tui.New(
			tui.WithText(a.config.Title),
			tui.WithTextStyle(tui.NewStyle().Bold().Foreground(tui.Cyan)),
		))
	}
	root.AddChild(tui.New(
		tui.WithText(a.instructionsText()),
		tui.WithTextStyle(tui.NewStyle().Dim()),
	))
	root.AddChild(tui.New(
		tui.WithText(a.metaText()),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Green)),
	))
	root.AddChild(a.textarea.Render(app))
	root.AddChild(tui.New(
		tui.WithText(a.statusText()),
		tui.WithTextStyle(tui.NewStyle().Dim()),
	))

	return root
}

func (a *App) send(text string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || a.app == nil {
		return
	}

	a.textarea.Clear()
	a.app.PrintAboveln("%s", a.config.RenderUserMessage(trimmed))
	a.startResponse(trimmed)
}

func (a *App) startResponse(input string) {
	reqCtx, cancel := context.WithCancel(context.Background())
	stream := newRequestStream(reqCtx, a.app.StreamAbove())

	reqID, prevCancel := a.registerRequest(cancel)
	if prevCancel != nil {
		prevCancel()
	}
	a.streaming.Set(true)

	req := &Request{
		Context:     reqCtx,
		Input:       input,
		Stream:      stream,
		TerminalApp: a.app,
		Shell:       a,
		id:          reqID,
	}

	go func(id uint64) {
		err := a.config.HandleResponse(req)
		_ = stream.Close()
		a.finishResponse(id, err)
	}(reqID)
}

func (a *App) finishResponse(id uint64, err error) {
	if a.app == nil {
		return
	}

	a.app.QueueUpdate(func() {
		if !a.completeRequest(id) {
			return
		}

		a.streaming.Set(false)
		if err != nil && !errors.Is(err, context.Canceled) {
			a.app.PrintAboveln("%s", a.config.RenderError(err))
		}
	})
}

func (a *App) registerRequest(cancel context.CancelFunc) (uint64, context.CancelFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()

	prev := a.activeCancel
	a.nextReqID++
	a.activeReqID = a.nextReqID
	a.activeCancel = cancel
	a.statusOverride = ""

	return a.activeReqID, prev
}

func (a *App) completeRequest(id uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if id != a.activeReqID {
		return false
	}

	a.activeCancel = nil
	a.statusOverride = ""
	return true
}

func (a *App) setRequestStatus(id uint64, ctx context.Context, text string) {
	if a.queueUpdate == nil {
		return
	}

	a.queueUpdate(func() {
		if ctx.Err() != nil {
			return
		}

		a.mu.Lock()
		defer a.mu.Unlock()
		if id != a.activeReqID {
			return
		}

		a.statusOverride = text
	})
}

func (a *App) toggleMultiline() {
	a.multiline.Set(!a.multiline.Get())
}

func (a *App) openSettings() {
	if a.app == nil || a.config.SettingsView == nil || a.showSettings.Get() {
		return
	}
	a.showHelp.Set(false)
	a.showSettings.Set(true)
	_ = a.app.EnterAlternateScreen()
}

func (a *App) openHelp() {
	if a.app == nil || a.config.HelpView == nil || a.showHelp.Get() {
		return
	}
	a.showSettings.Set(false)
	a.showHelp.Set(true)
	_ = a.app.EnterAlternateScreen()
}

func (a *App) closeOverlay() {
	if a.app == nil {
		return
	}
	_ = a.app.ExitAlternateScreen()
	a.showSettings.Set(false)
	a.showHelp.Set(false)
}

func (a *App) inlineHeight() int {
	if a.multiline.Get() {
		return a.config.MultilineHeight
	}
	return a.config.CompactHeight
}

func (a *App) instructionsText() string {
	if a.instructions != "" {
		return a.instructions
	}
	parts := []string{"Enter sends above the widget.", "Tab toggles compact/multiline."}
	if a.config.SettingsView != nil {
		parts = append(parts, "Ctrl+S opens settings.")
	}
	if a.config.HelpView != nil {
		parts = append(parts, "F1 opens help.")
	}
	return strings.Join(parts, " ")
}

func (a *App) metaText() string {
	return fmt.Sprintf("Mode: %s | Ctrl+C or Esc exits", a.modeLabel())
}

func (a *App) modeLabel() string {
	if a.multiline.Get() {
		return "multiline"
	}
	return "compact"
}

func (a *App) statusText() string {
	a.mu.Lock()
	status := a.statusOverride
	a.mu.Unlock()
	if status != "" {
		return status
	}
	if a.streaming.Get() {
		return "Streaming response..."
	}
	return "Ready. Submit a message to stream a response above the widget."
}

func (a *App) renderOverlay(view OverlayView, fallbackTitle string) *tui.Element {
	if view != nil {
		if el := view(a); el != nil {
			return el
		}
	}

	root := tui.New(
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Column),
		tui.WithPadding(1),
		tui.WithHeightPercent(100),
		tui.WithBorder(tui.BorderRounded),
		tui.WithBorderStyle(tui.NewStyle().Foreground(tui.Yellow)),
	)
	root.AddChild(tui.New(
		tui.WithText(fallbackTitle),
		tui.WithTextStyle(tui.NewStyle().Bold().Foreground(tui.Yellow)),
	))
	root.AddChild(tui.New(
		tui.WithText("Press Escape to return to inline mode."),
		tui.WithTextStyle(tui.NewStyle().Dim()),
	))
	return root
}

func normalizeConfig(config Config) Config {
	cfg := config
	if cfg.Placeholder == "" {
		cfg.Placeholder = defaultPlaceholder
	}
	if cfg.CompactHeight < 1 {
		cfg.CompactHeight = defaultCompactHeight
	}
	if cfg.MultilineHeight < cfg.CompactHeight {
		cfg.MultilineHeight = defaultMultilineHeight
		if cfg.MultilineHeight < cfg.CompactHeight {
			cfg.MultilineHeight = cfg.CompactHeight
		}
	}
	if cfg.HandleResponse == nil {
		cfg.HandleResponse = func(req *Request) error {
			_, err := req.Stream.Write([]byte("No response handler configured.\n"))
			return err
		}
	}
	if cfg.RenderUserMessage == nil {
		cfg.RenderUserMessage = func(input string) string {
			return fmt.Sprintf("You: %s", input)
		}
	}
	if cfg.RenderError == nil {
		cfg.RenderError = func(err error) string {
			return fmt.Sprintf("Error: %v", err)
		}
	}
	return cfg
}

type requestStream struct {
	ctx    context.Context
	writer Stream

	mu     sync.Mutex
	closed bool
}

func newRequestStream(ctx context.Context, writer Stream) *requestStream {
	return &requestStream{ctx: ctx, writer: writer}
}

func (s *requestStream) Write(p []byte) (int, error) {
	if err := s.checkWritable(); err != nil {
		return 0, err
	}
	return s.writer.Write(p)
}

func (s *requestStream) WriteStyled(text string, style tui.Style) (int, error) {
	if err := s.checkWritable(); err != nil {
		return 0, err
	}
	return s.writer.WriteStyled(text, style)
}

func (s *requestStream) WriteGradient(text string, g tui.Gradient, base ...tui.Style) (int, error) {
	if err := s.checkWritable(); err != nil {
		return 0, err
	}
	return s.writer.WriteGradient(text, g, base...)
}

func (s *requestStream) WriteElement(v tui.Viewable) {
	if s.checkWritable() != nil {
		return
	}
	s.writer.WriteElement(v)
}

func (s *requestStream) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	return s.writer.Close()
}

func (s *requestStream) checkWritable() error {
	if err := s.ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return io.ErrClosedPipe
	}
	return nil
}

var (
	_ tui.Component       = (*App)(nil)
	_ tui.KeyListener     = (*App)(nil)
	_ tui.WatcherProvider = (*App)(nil)
	_ tui.AppBinder       = (*App)(nil)
	_ Stream              = (*requestStream)(nil)
)
