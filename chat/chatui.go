package chat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	tui "github.com/pfernandom/go-tui"
)

const (
	defaultPlaceholder = "Type a message. Enter sends. Ctrl+J adds a newline."
	// Default min (compact) and max (multiline) strip heights; actual height is computed from content.
	defaultCompactHeight   = 10
	defaultMultilineHeight = 20
	// TextArea shows at most this many wrapped rows; higher avoids clipping long pasted lines.
	defaultTextAreaMaxHeight = 12
	// Default status strings (must stay in sync with statusText).
	statusStreaming = "Streaming response..."
	statusReady     = "Ready. Submit a message to stream a response above the widget."
	// Used before BindApp when terminal size is unknown (e.g. tests).
	fallbackTextAreaWidth = 60
	minTextAreaWidth      = 16
	// Subtracted from terminal width when TextAreaWidth is 0 (auto).
	termWidthGutter = 6
	// chromePaddingRows is vertical cells used by root WithPadding(1) (top + bottom).
	chromePaddingRows = 2
	// initialTermWidth/initialTermHeight are used before BindApp for WithInlineHeight sizing.
	initialTermWidth  = 80
	initialTermHeight = 24
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

	// FromSlash is true when this request was produced from the slash-command path
	// (fallthrough or streaming from a non-fallthrough SlashResponse). When false, Slash is zero.
	FromSlash bool
	// Slash holds the parsed command when FromSlash is true.
	Slash SlashCommand

	id uint64
}

func (r *Request) SetStatus(text string) {
	if r == nil || r.Shell == nil {
		return
	}
	r.Shell.setRequestStatus(r.id, r.Context, text)
}

// ResponseHandler runs in its own goroutine per request. Use [Request.Stream] and
// [App.PrintAboveln]/[App.QueuePrintAboveln] for terminal output; they are safe from
// background goroutines (unlike calling [tui.App.PrintAboveln] on TerminalApp directly).
type ResponseHandler func(*Request) error
type MessageRenderer func(input string) string
type ErrorRenderer func(err error) string
type OverlayView func(*App) *tui.Element

// SlashCommand describes input that begins with "/" after trimming.
type SlashCommand struct {
	Raw  string
	Name string
	Args string
}

// SlashResponse controls echo and streaming for slash commands.
type SlashResponse struct {
	Command  SlashCommand
	Response string
	Handled  bool
}

// NewResponse returns a SlashResponse with Handled false and Response set to content.
// RenderUserMessage and Request.Input both use Response for the streaming path.
func (sc SlashCommand) NewResponse(content string) SlashResponse {
	return SlashResponse{
		Command:  sc,
		Response: content,
		Handled:  false,
	}
}

// Forward returns a SlashResponse that passes through the raw slash line as Response,
// with Handled false (same echo and Input as a fallthrough).
func (sc SlashCommand) Forward() SlashResponse {
	return SlashResponse{
		Command:  sc,
		Response: sc.Raw,
		Handled:  false,
	}
}

// Handled returns a SlashResponse with Handled true and Command set to sc.
// The shell clears the input and does not echo or call HandleResponse.
func (sc SlashCommand) Handled() SlashResponse {
	return SlashResponse{
		Command: sc,
		Handled: true,
	}
}

// IsFallthrough reports whether the handler returned the zero value, meaning: fall through
// to normal submit (echo trimmed line, stream with Input == trimmed).
func (r SlashResponse) IsFallthrough() bool {
	return r == SlashResponse{}
}

// ParseSlashCommand reports whether trimmed begins with "/". Name is the first segment;
// Args is the remainder after the first run of spaces. For "/" alone, Name and Args are empty.
func ParseSlashCommand(trimmed string) (SlashCommand, bool) {
	if !strings.HasPrefix(trimmed, "/") {
		return SlashCommand{}, false
	}
	raw := trimmed
	body := strings.TrimSpace(trimmed[1:])
	if body == "" {
		return SlashCommand{Raw: raw}, true
	}
	i := strings.IndexByte(body, ' ')
	if i < 0 {
		return SlashCommand{Raw: raw, Name: body}, true
	}
	name := strings.TrimSpace(body[:i])
	args := strings.TrimSpace(body[i+1:])
	return SlashCommand{Raw: raw, Name: name, Args: args}, true
}

type Config struct {
	Title       string
	Placeholder string
	// CompactHeight is the minimum height (terminal rows) of the inline strip and the
	// maximum when multiline mode is off (compact layout).
	CompactHeight int
	// MultilineHeight is the maximum height of the inline strip when multiline mode is on.
	// The shell grows between CompactHeight and MultilineHeight based on composer + chrome.
	MultilineHeight  int
	DefaultMultiline bool
	// ComposerTextareaOptions are applied after built-in sizing, border, and placeholder.
	ComposerTextareaOptions []ComposerTextareaOption
	// TextAreaWidth is the composer width in terminal cells. If 0, width is set at BindApp
	// from the terminal size (minus a small gutter) so the field fits narrow windows.
	TextAreaWidth int
	// TextAreaMaxHeight is the maximum number of visible wrapped rows ([ComposerTextArea]).
	// If 0, defaultTextAreaMaxHeight is used. Override with ComposerTextareaOptions if needed.
	TextAreaMaxHeight int

	HandleResponse ResponseHandler
	// RenderUserMessage formats text printed above the widget before a response (often "You: …").
	// The string may include newlines; the default appends a horizontal rule under the echo.
	RenderUserMessage MessageRenderer
	RenderError       ErrorRenderer

	Instructions string

	SettingsView OverlayView
	HelpView     OverlayView

	// SlashCommands maps lowercase command names (e.g. "help") to implementations.
	// When set, registered names are merged into SlashCommandNames for autocomplete.
	SlashCommands map[string]SlashCommandConfig

	// SlashCommandNames lists available slash names without a leading "/". When non-empty,
	// "/" triggers autocomplete hints and Tab completes the active (last) line when it looks
	// like a single slash token (see package docs).
	SlashCommandNames []string
}

type App struct {
	config Config

	app          *tui.App
	showSettings *tui.State[bool]
	showHelp     *tui.State[bool]
	multiline    *tui.State[bool]
	streaming    *tui.State[bool]
	textarea     *ComposerTextArea

	mu             sync.Mutex
	activeCancel   context.CancelFunc
	activeReqID    uint64
	nextReqID      uint64
	statusOverride string

	instructions string
	queueUpdate  func(func())

	slashTabCycle      int
	slashLastTabPrefix string

	textareaSizedWidth int // last width passed to NewTextArea; -1 = not yet synced from terminal

	// streamInlineMaxHeight is the largest inline height seen during the active stream; while
	// streaming, effectiveInlineHeight never returns below this (clamped to terminal height).
	// Prevents SetInlineHeight from shrinking mid-stream when layout inputs change (meta line,
	// slash hint, SetStatus text, etc.), which corrupts StreamAbove output on narrow TTYs.
	streamInlineMaxHeight int
}

func New(config Config) *App {
	cfg := normalizeConfig(config)
	for name := range cfg.SlashCommands {
		cfg.SlashCommandNames = append(cfg.SlashCommandNames, strings.ToLower(strings.TrimSpace(name)))
	}
	cfg.SlashCommandNames = normalizeSlashNames(cfg.SlashCommandNames)

	a := &App{
		config:             cfg,
		showSettings:       tui.NewState(false),
		showHelp:           tui.NewState(false),
		multiline:          tui.NewState(cfg.DefaultMultiline),
		streaming:          tui.NewState(false),
		instructions:       cfg.Instructions,
		textareaSizedWidth: -1,
	}
	a.queueUpdate = func(fn func()) {
		if fn != nil {
			fn()
		}
	}

	a.textarea = NewComposerTextarea(a.textAreaOptions(fallbackTextAreaWidth)...)

	return a
}

// textAreaOptions builds composer options for a given content width in cells.
func (a *App) textAreaOptions(width int) []ComposerTextareaOption {
	maxH := a.config.TextAreaMaxHeight
	if maxH <= 0 { // defensive; normalizeConfig usually sets a positive default
		maxH = defaultTextAreaMaxHeight
	}
	opts := []ComposerTextareaOption{
		ComposerWidth(width),
		ComposerMaxHeight(maxH),
		ComposerBorder(tui.BorderRounded),
		ComposerPlaceholder(a.config.Placeholder),
		ComposerAutoFocus(true),
	}
	opts = append(opts, a.config.ComposerTextareaOptions...)
	opts = append(opts, ComposerOnSubmit(a.send))
	return opts
}

// effectiveTextAreaWidth returns the composer width: explicit config, or terminal-based auto.
func (a *App) effectiveTextAreaWidth(termWidth int) int {
	if a.config.TextAreaWidth > 0 {
		return a.config.TextAreaWidth
	}
	if termWidth <= 0 {
		return fallbackTextAreaWidth
	}
	w := termWidth - termWidthGutter
	if w < minTextAreaWidth {
		w = minTextAreaWidth
	}
	return w
}

func (a *App) Start(opts ...tui.AppOption) (*tui.App, error) {
	base := []tui.AppOption{
		tui.WithInlineHeight(a.initialInlineHeight()),
		tui.WithRootComponent(a),
	}
	opts = append(base, opts...)
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

	tw, _ := app.Terminal().Size()
	wantW := a.effectiveTextAreaWidth(tw)
	if wantW != a.textareaSizedWidth {
		preserved := ""
		if a.textarea != nil {
			preserved = a.textarea.Text()
		}
		a.textarea = NewComposerTextarea(a.textAreaOptions(wantW)...)
		if preserved != "" {
			a.textarea.SetText(preserved)
		}
		a.textareaSizedWidth = wantW
	}

	a.textarea.BindApp(app)
}

// PrintAboveln appends a formatted line to the history region above the inline composer.
// Updates are queued on the go-tui main loop, so it is safe to call from any goroutine
// (including [ResponseHandler]). No-op if BindApp has not run yet.
func (a *App) PrintAboveln(format string, args ...any) {
	a.QueuePrintAboveln(format, args...)
}

// QueuePrintAboveln is equivalent to [App.PrintAboveln]; both marshal work onto the
// app's main loop. Kept for API compatibility with go-tui naming.
func (a *App) QueuePrintAboveln(format string, args ...any) {
	if a == nil || a.app == nil {
		return
	}
	a.app.QueuePrintAboveln(format, args...)
}

// Terminal returns the underlying go-tui Terminal (e.g. for Clear). Returns nil before the shell is bound.
func (a *App) Terminal() tui.Terminal {
	if a == nil || a.app == nil {
		return nil
	}
	return a.app.Terminal()
}

func (a *App) Close() {
	if a.app == nil {
		return
	}
	a.app.Stop()
}

// IsFocused reports whether the composer is focused. It allows focus-gated key bindings
// merged into App.KeyMap to align with the embedded TextArea.
func (a *App) IsFocused() bool {
	return a != nil && a.textarea != nil && a.textarea.IsFocused()
}

func (a *App) KeyMap() tui.KeyMap {
	if a.showSettings.Get() || a.showHelp.Get() {
		return tui.KeyMap{
			tui.OnStop(tui.KeyEscape, func(ke tui.KeyEvent) { a.closeOverlay() }),
			tui.OnStop(tui.KeyCtrlC, func(ke tui.KeyEvent) { ke.App().Stop() }),
		}
	}

	var km tui.KeyMap
	if len(a.config.SlashCommandNames) > 0 {
		km = append(km, tui.OnFocused(tui.Rune('/'), func(ke tui.KeyEvent) {
			_ = a.textarea.HandleEvent(tui.KeyEvent{Key: tui.KeyRune, Rune: '/'})
		}))
	}
	km = append(km, a.textarea.KeyMap()...)
	km = append(km,
		tui.OnStop(tui.KeyTab, func(ke tui.KeyEvent) {
			if len(a.config.SlashCommandNames) > 0 && a.trySlashTabComplete() {
				return
			}
			a.toggleMultiline()
		}),
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

	app.SetInlineHeight(a.effectiveInlineHeight(app))

	root := tui.New(
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Column),
		tui.WithPadding(1),
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
	if line := a.slashCommandsHintLine(); line != "" {
		root.AddChild(tui.New(
			tui.WithText(line),
			tui.WithTextStyle(tui.NewStyle().Dim()),
		))
	}
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

	resp, sc, slashPath, err := a.dispatchSlashCommand(trimmed)
	if err != nil {
		a.textarea.Clear()
		a.app.PrintAboveln("%s", a.config.RenderError(err))
		return
	}
	if slashPath {
		if resp.IsFallthrough() {
			a.textarea.Clear()
			a.app.PrintAboveln("%s", a.config.RenderUserMessage(trimmed))
			a.startResponse(trimmed, sc, true)
			return
		}
		if resp.Handled {
			a.textarea.Clear()
			return
		}
		a.textarea.Clear()
		a.app.PrintAboveln("%s", a.config.RenderUserMessage(resp.Response))
		a.startResponse(resp.Response, resp.Command, true)
		return
	}

	a.textarea.Clear()
	a.app.PrintAboveln("%s", a.config.RenderUserMessage(trimmed))
	a.startResponse(trimmed, SlashCommand{}, false)
}

// dispatchSlashCommand parses a slash line and optionally runs Config.SlashCommands[name].
// slashPath is true when the input parses as a slash command (starts with "/"); the caller
// then uses resp (fallthrough when zero) or handler output.
func (a *App) dispatchSlashCommand(trimmed string) (resp SlashResponse, sc SlashCommand, slashPath bool, err error) {
	sc, ok := ParseSlashCommand(trimmed)
	if !ok {
		return SlashResponse{}, SlashCommand{}, false, nil
	}
	slashPath = true
	if len(a.config.SlashCommands) == 0 {
		return SlashResponse{}, sc, slashPath, nil
	}
	key := strings.ToLower(sc.Name)
	handler, found := a.config.SlashCommands[key]
	if !found {
		return SlashResponse{}, sc, slashPath, nil
	}
	resp, err = handler.Handle(a, sc)
	return resp, sc, slashPath, err
}

func (a *App) startResponse(input string, slash SlashCommand, fromSlash bool) {
	reqCtx, cancel := context.WithCancel(context.Background())
	stream := newRequestStream(reqCtx, a.app.StreamAbove())

	reqID, prevCancel := a.registerRequest(cancel)
	if prevCancel != nil {
		prevCancel()
	}
	a.streaming.Set(true)
	a.streamInlineMaxHeight = 0

	req := &Request{
		Context:     reqCtx,
		Input:       input,
		Stream:      stream,
		TerminalApp: a.app,
		Shell:       a,
		FromSlash:   fromSlash,
		Slash:       slash,
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
		a.streamInlineMaxHeight = 0
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

func (a *App) trySlashTabComplete() bool {
	names := a.config.SlashCommandNames
	if len(names) == 0 {
		return false
	}
	full := a.textarea.Text()
	line := activeLineLast(full)
	if !strings.HasPrefix(line, "/") {
		return false
	}
	inner := line[1:]
	if strings.ContainsAny(inner, " \t") {
		return false
	}
	pref := inner
	matches := filterSlashPrefixMatches(names, pref)
	if len(matches) == 0 {
		return false
	}
	if pref != a.slashLastTabPrefix {
		a.slashTabCycle = 0
	}
	lcp := longestCommonPrefixStrings(matches)
	newLast, cycle := slashTabStep(pref, matches, lcp, a.slashTabCycle)
	if newLast == "" {
		return false
	}
	a.slashTabCycle = cycle
	a.slashLastTabPrefix = strings.TrimPrefix(newLast, "/")
	a.textarea.SetText(replaceActiveLine(full, newLast))
	return true
}

func activeLineLast(full string) string {
	if full == "" {
		return ""
	}
	i := strings.LastIndex(full, "\n")
	if i < 0 {
		return full
	}
	return full[i+1:]
}

func replaceActiveLine(full, newLast string) string {
	if !strings.Contains(full, "\n") {
		return newLast
	}
	i := strings.LastIndex(full, "\n")
	return full[:i+1] + newLast
}

func filterSlashPrefixMatches(names []string, pref string) []string {
	pl := strings.ToLower(pref)
	var out []string
	for _, n := range names {
		if strings.HasPrefix(strings.ToLower(n), pl) {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func longestCommonPrefixStrings(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	base := strs[0]
	for i := 0; i < len(base); i++ {
		c := base[i]
		for _, s := range strs[1:] {
			if i >= len(s) || s[i] != c {
				return base[:i]
			}
		}
	}
	return base
}

// slashTabStep returns the new last-line content (including leading "/") and updated cycle index.
func slashTabStep(pref string, matches []string, lcp string, cycle int) (newLast string, newCycle int) {
	if len(matches) == 1 {
		m := matches[0]
		if strings.EqualFold(m, pref) {
			return "/" + m + " ", 0
		}
		if strings.HasPrefix(strings.ToLower(m), strings.ToLower(pref)) {
			return "/" + m, 0
		}
		return "", 0
	}
	if !strings.EqualFold(pref, lcp) && strings.HasPrefix(strings.ToLower(lcp), strings.ToLower(pref)) {
		return "/" + lcp, 0
	}
	next := cycle % len(matches)
	pick := matches[next]
	return "/" + pick, cycle + 1
}

func (a *App) slashCommandsHintLine() string {
	if len(a.config.SlashCommandNames) == 0 {
		return ""
	}
	line := activeLineLast(a.textarea.Text())
	if !strings.HasPrefix(line, "/") {
		return ""
	}
	return "Slash: " + strings.Join(a.config.SlashCommandNames, ", ")
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

// inlineStripBounds returns min and max allowed inline strip heights in rows.
func (a *App) inlineStripBounds() (minH, maxH int) {
	minH = a.config.CompactHeight
	maxH = a.config.MultilineHeight
	if !a.multiline.Get() {
		maxH = a.config.CompactHeight
	}
	if minH < 1 {
		minH = 1
	}
	if maxH < minH {
		maxH = minH
	}
	return minH, maxH
}

// initialInlineHeight selects a strip height before the terminal is bound (Start/NewApp).
func (a *App) initialInlineHeight() int {
	return a.computeInlineHeightForTerminal(initialTermWidth, initialTermHeight)
}

// effectiveInlineHeight returns the strip height for the current layout and terminal.
func (a *App) effectiveInlineHeight(app *tui.App) int {
	if app == nil {
		return a.initialInlineHeight()
	}
	tw, th := app.Terminal().Size()
	h := a.computeInlineHeightForTerminal(tw, th)
	if !a.streaming.Get() {
		return h
	}
	if h > a.streamInlineMaxHeight {
		a.streamInlineMaxHeight = h
	}
	thCap := th
	if thCap > 0 && a.streamInlineMaxHeight > thCap {
		a.streamInlineMaxHeight = thCap
	}
	use := a.streamInlineMaxHeight
	if h < use {
		return use
	}
	return h
}

func (a *App) computeInlineHeightForTerminal(termWidth, termHeight int) int {
	minH, maxH := a.inlineStripBounds()
	inner := termWidth - 2 // root WithPadding(1) left+right
	if inner < 1 {
		inner = 1
	}

	rows := chromePaddingRows
	if a.config.Title != "" {
		rows += countWrappedLines(a.config.Title, inner)
	}
	rows += countWrappedLines(a.instructionsText(), inner)
	rows += metaRowsForLayout(inner)
	rows += a.slashCommandsHintRowsForLayout(inner)
	rows += a.statusRowsForLayout(inner)
	rows += a.textarea.Height()

	if rows < minH {
		rows = minH
	}
	if rows > maxH {
		rows = maxH
	}
	if termHeight > 0 && rows > termHeight {
		rows = termHeight
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

// countWrappedLines returns how many terminal rows text needs when wrapped at rune limit per line.
// Empty string contributes 0 rows. Paragraphs are split on '\n' and each wrapped like [ComposerTextArea.wrapText].
func countWrappedLines(s string, limit int) int {
	if s == "" {
		return 0
	}
	if limit < 1 {
		limit = 1
	}
	total := 0
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			total++
			continue
		}
		current := 0
		for range []rune(para) {
			if current >= limit {
				total++
				current = 0
			}
			current++
		}
		total++
	}
	return total
}

func (a *App) instructionsText() string {
	if a.instructions != "" {
		return a.instructions
	}
	parts := []string{"Enter sends above the widget."}
	if len(a.config.SlashCommandNames) > 0 {
		parts = append(parts, "Tab completes slash commands on a / line, or toggles compact/multiline otherwise.")
	} else {
		parts = append(parts, "Tab toggles compact/multiline.")
	}
	if a.config.SettingsView != nil {
		parts = append(parts, "Ctrl+S opens settings.")
	}
	if a.config.HelpView != nil {
		parts = append(parts, "F1 opens help.")
	}
	return strings.Join(parts, " ")
}

func metaLineForMode(mode string) string {
	return fmt.Sprintf("Mode: %s | Ctrl+C or Esc exits", mode)
}

func (a *App) metaText() string {
	return metaLineForMode(a.modeLabel())
}

// metaRowsForLayout returns wrapped row count for the mode line using the larger of compact vs
// multiline labels so inline height does not change when Tab toggles mode during streaming.
func metaRowsForLayout(inner int) int {
	c := countWrappedLines(metaLineForMode("compact"), inner)
	m := countWrappedLines(metaLineForMode("multiline"), inner)
	n := c
	if m > n {
		n = m
	}
	if n < 1 {
		n = 1
	}
	return n
}

// slashCommandsHintRowsForLayout reserves rows for the slash hint as if it were visible, so
// height does not jump when the user types "/" during an active stream.
func (a *App) slashCommandsHintRowsForLayout(inner int) int {
	if len(a.config.SlashCommandNames) == 0 {
		return 0
	}
	line := "Slash: " + strings.Join(a.config.SlashCommandNames, ", ")
	n := countWrappedLines(line, inner)
	if n < 1 {
		n = 1
	}
	return n
}

func (a *App) modeLabel() string {
	if a.multiline.Get() {
		return "multiline"
	}
	return "compact"
}

// statusRowsForLayout returns how many rows the status line may need, using the maximum of
// built-in and override strings. This must not depend on the current streaming/idle choice alone,
// or SetInlineHeight would change when switching between short "Streaming…" and long idle text,
// which retriggers go-tui inline resize and corrupts streamed output (e.g. in Docker).
func (a *App) statusRowsForLayout(inner int) int {
	a.mu.Lock()
	override := a.statusOverride
	a.mu.Unlock()

	n := countWrappedLines(statusStreaming, inner)
	if x := countWrappedLines(statusReady, inner); x > n {
		n = x
	}
	if override != "" {
		if x := countWrappedLines(override, inner); x > n {
			n = x
		}
	}
	if n < 1 {
		n = 1
	}
	return n
}

func (a *App) statusText() string {
	a.mu.Lock()
	status := a.statusOverride
	a.mu.Unlock()
	if status != "" {
		return status
	}
	if a.streaming.Get() {
		return statusStreaming
	}
	return statusReady
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

// userMessageDividerMinWidth is the minimum rune length of the rule under the user echo.
const userMessageDividerMinWidth = 12

// defaultRenderUserMessage prints the user line(s) and a horizontal rule below so the
// following streamed reply is visually separated from the echo.
func defaultRenderUserMessage(input string) string {
	msg := fmt.Sprintf("You: %s", input)
	maxw := userMessageDividerMinWidth
	for _, line := range strings.Split(msg, "\n") {
		if n := utf8.RuneCountInString(line); n > maxw {
			maxw = n
		}
	}
	div := strings.Repeat("─", maxw)
	return msg + "\n" + div
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
		cfg.RenderUserMessage = defaultRenderUserMessage
	}
	if cfg.RenderError == nil {
		cfg.RenderError = func(err error) string {
			return fmt.Sprintf("Error: %v", err)
		}
	}
	if cfg.TextAreaMaxHeight <= 0 {
		cfg.TextAreaMaxHeight = defaultTextAreaMaxHeight
	}
	cfg.SlashCommandNames = normalizeSlashNames(cfg.SlashCommandNames)
	return cfg
}

func normalizeSlashNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	sort.Strings(out)
	return out
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
