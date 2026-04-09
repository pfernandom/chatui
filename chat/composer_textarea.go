package chat

import (
	"strings"
	"time"
	"unicode/utf8"

	tui "github.com/pfernandom/go-tui"
)

// keyPatternMatches mirrors go-tui dispatchEntry.matchesKey for [ComposerTextArea.HandleEvent].
func keyPatternMatches(p tui.KeyPattern, ke tui.KeyEvent) bool {
	if p.AnyKey {
		return true
	}
	if p.ExcludeMods != 0 && ke.Mod&p.ExcludeMods != 0 {
		return false
	}
	if p.Mod != 0 && ke.Mod != p.Mod {
		return false
	}
	if p.AnyRune && ke.Key == tui.KeyRune {
		return true
	}
	if p.Rune != 0 && ke.Rune == p.Rune && ke.Key == tui.KeyRune {
		return true
	}
	if p.Key != 0 && ke.Key == p.Key {
		return true
	}
	return false
}

// ComposerTextArea is a multi-line composer input with wrapping and cursor handling.
// It matches go-tui's TextArea behavior with fixes for maxHeight row emission and
// border-aware wrap width, without requiring consumers to vendor go-tui.
//
// It implements tui.Component, tui.KeyListener, tui.WatcherProvider, tui.Focusable, and tui.AppBinder.
type ComposerTextArea struct {
	width            int
	maxHeight        int
	border           tui.BorderStyle
	textStyle        tui.Style
	placeholder      string
	placeholderStyle tui.Style
	cursorRune       rune
	focusColor       *tui.Color
	borderGradient   *tui.Gradient
	focusGradient    *tui.Gradient
	autoFocus        bool
	submitKey        tui.Key
	onSubmit         func(string)

	text      *tui.State[string]
	cursorPos *tui.State[int]
	blink     *tui.State[bool]
	focused   *tui.State[bool]
}

var (
	_ tui.Component       = (*ComposerTextArea)(nil)
	_ tui.KeyListener     = (*ComposerTextArea)(nil)
	_ tui.WatcherProvider = (*ComposerTextArea)(nil)
	_ tui.Focusable       = (*ComposerTextArea)(nil)
	_ tui.AppBinder       = (*ComposerTextArea)(nil)
)

// BindApp binds internal state to the app.
func (t *ComposerTextArea) BindApp(app *tui.App) {
	t.text.BindApp(app)
	t.cursorPos.BindApp(app)
	t.blink.BindApp(app)
	t.focused.BindApp(app)
}

// NewComposerTextarea creates a new composer text area.
func NewComposerTextarea(opts ...ComposerTextareaOption) *ComposerTextArea {
	t := &ComposerTextArea{
		width:            40,
		maxHeight:        0,
		border:           tui.BorderNone,
		textStyle:        tui.Style{},
		placeholder:      "",
		placeholderStyle: tui.Style{}.Dim(),
		cursorRune:       '▌',
		submitKey:        tui.KeyEnter,

		text:      tui.NewState(""),
		cursorPos: tui.NewState(0),
		blink:     tui.NewState(true),
		focused:   tui.NewState(false),
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Text returns the current text content.
func (t *ComposerTextArea) Text() string {
	return t.text.Get()
}

// SetText sets the text and moves the cursor to the end.
func (t *ComposerTextArea) SetText(s string) {
	t.text.Set(s)
	t.cursorPos.Set(utf8.RuneCountInString(s))
}

// Clear clears the composer.
func (t *ComposerTextArea) Clear() {
	t.text.Set("")
	t.cursorPos.Set(0)
}

// Height returns the total rendered height including border.
func (t *ComposerTextArea) Height() int {
	lines := t.wrapText()
	height := len(lines)
	if height < 1 {
		height = 1
	}
	if t.maxHeight > 0 && height > t.maxHeight {
		height = t.maxHeight
	}
	if t.border != tui.BorderNone {
		height += 2
	}
	return height
}

// Render returns the element tree for the composer.
func (t *ComposerTextArea) Render(app *tui.App) *tui.Element {
	lines := t.wrapText()
	height := len(lines)
	if height < 1 {
		height = 1
	}
	if t.maxHeight > 0 && height > t.maxHeight {
		height = t.maxHeight
	}

	totalHeight := height
	if t.border != tui.BorderNone {
		totalHeight += 2
	}

	opts := []tui.Option{
		tui.WithDirection(tui.Column),
		tui.WithHeight(totalHeight),
		tui.WithFocusable(true),
		tui.WithAutoFocus(t.autoFocus),
	}
	if t.width > 0 {
		opts = append(opts, tui.WithWidth(t.width))
	}
	if t.border != tui.BorderNone {
		opts = append(opts, tui.WithBorder(t.border))
		if t.focused.Get() {
			if t.focusGradient != nil {
				opts = append(opts, tui.WithBorderGradient(*t.focusGradient))
			} else if t.focusColor != nil {
				opts = append(opts, tui.WithBorderStyle(tui.NewStyle().Foreground(*t.focusColor)))
			}
		} else if t.borderGradient != nil {
			opts = append(opts, tui.WithBorderGradient(*t.borderGradient))
		}
	}
	root := tui.New(opts...)

	root.SetOnFocus(func(e *tui.Element) {
		t.Focus()
	})
	root.SetOnBlur(func(e *tui.Element) {
		t.Blur()
	})

	if t.text.Get() == "" && t.placeholder != "" && !t.focused.Get() {
		root.AddChild(tui.New(tui.WithText(t.placeholder), tui.WithTextStyle(t.placeholderStyle)))
	} else {
		displayLines := len(lines)
		if t.maxHeight > 0 && displayLines > t.maxHeight {
			displayLines = t.maxHeight
		}
		for i := 0; i < displayLines; i++ {
			root.AddChild(tui.New(tui.WithText(t.lineWithCursor(i)), tui.WithTextStyle(t.textStyle)))
		}
	}

	return root
}

// IsFocusable reports whether the composer can receive focus.
func (t *ComposerTextArea) IsFocusable() bool {
	return true
}

// IsTabStop reports whether the composer participates in Tab navigation.
func (t *ComposerTextArea) IsTabStop() bool {
	return true
}

// Focus marks the composer as focused.
func (t *ComposerTextArea) Focus() {
	if t.focused.Get() {
		return
	}
	t.focused.Set(true)
	t.blink.Set(true)
}

// Blur marks the composer as unfocused.
func (t *ComposerTextArea) Blur() {
	if !t.focused.Get() {
		return
	}
	t.focused.Set(false)
}

// IsFocused reports whether the composer has focus.
func (t *ComposerTextArea) IsFocused() bool {
	return t.focused.Get()
}

// HandleEvent processes keyboard events (mirrors go-tui TextArea).
func (t *ComposerTextArea) HandleEvent(e tui.Event) bool {
	ke, ok := e.(tui.KeyEvent)
	if !ok {
		return false
	}

	for _, binding := range t.KeyMap() {
		if keyPatternMatches(binding.Pattern, ke) {
			binding.Handler(ke)
			return binding.Stop
		}
	}
	return false
}

// KeyMap returns key bindings for the composer.
func (t *ComposerTextArea) KeyMap() tui.KeyMap {
	km := tui.KeyMap{
		tui.OnFocused(tui.AnyRune, t.insertChar),
		tui.OnFocused(tui.KeyBackspace, t.backspace),
		tui.OnFocused(tui.KeyDelete, t.delete),
		tui.OnFocused(tui.KeyLeft, t.moveLeft),
		tui.OnFocused(tui.KeyRight, t.moveRight),
		tui.OnFocused(tui.KeyUp, t.moveUp),
		tui.OnFocused(tui.KeyDown, t.moveDown),
		tui.OnFocused(tui.KeyHome, t.moveHome),
		tui.OnFocused(tui.KeyEnd, t.moveEnd),
	}

	if t.submitKey == tui.KeyEnter {
		km = append(km,
			tui.OnFocused(tui.Rune('j').Ctrl(), t.insertNewline),
			tui.OnFocused(tui.KeyEnter, t.submit),
		)
	} else {
		km = append(km,
			tui.OnFocused(tui.KeyEnter, t.insertNewline),
			tui.OnFocused(t.submitKey, t.submit),
		)
	}

	km = append(km,
		tui.OnFocused(tui.KeyEscape, func(ke tui.KeyEvent) {
			if app := ke.App(); app != nil {
				app.BlurFocused()
			}
		}),
	)

	return km
}

// Watchers returns cursor blink watchers.
func (t *ComposerTextArea) Watchers() []tui.Watcher {
	return []tui.Watcher{
		tui.OnTimer(500*time.Millisecond, func() {
			if t.focused.Get() {
				t.blink.Set(!t.blink.Get())
			}
		}),
	}
}

func (t *ComposerTextArea) insertChar(ke tui.KeyEvent) {
	runes := []rune(t.text.Get())
	pos := t.clampCursorPos()
	newRunes := append(runes[:pos], append([]rune{ke.Rune}, runes[pos:]...)...)
	t.text.Set(string(newRunes))
	t.cursorPos.Set(pos + 1)
	t.blink.Set(true)
}

func (t *ComposerTextArea) insertNewline(ke tui.KeyEvent) {
	runes := []rune(t.text.Get())
	pos := t.clampCursorPos()
	newRunes := append(runes[:pos], append([]rune{'\n'}, runes[pos:]...)...)
	t.text.Set(string(newRunes))
	t.cursorPos.Set(pos + 1)
	t.blink.Set(true)
}

func (t *ComposerTextArea) backspace(ke tui.KeyEvent) {
	runes := []rune(t.text.Get())
	pos := t.clampCursorPos()
	if pos > 0 {
		newRunes := append(runes[:pos-1], runes[pos:]...)
		t.text.Set(string(newRunes))
		t.cursorPos.Set(pos - 1)
	}
}

func (t *ComposerTextArea) delete(ke tui.KeyEvent) {
	runes := []rune(t.text.Get())
	pos := t.clampCursorPos()
	if pos < len(runes) {
		newRunes := append(runes[:pos], runes[pos+1:]...)
		t.text.Set(string(newRunes))
	}
}

func (t *ComposerTextArea) moveLeft(ke tui.KeyEvent) {
	pos := t.cursorPos.Get()
	if pos > 0 {
		t.cursorPos.Set(pos - 1)
		t.blink.Set(true)
	}
}

func (t *ComposerTextArea) moveRight(ke tui.KeyEvent) {
	pos := t.cursorPos.Get()
	if pos < utf8.RuneCountInString(t.text.Get()) {
		t.cursorPos.Set(pos + 1)
		t.blink.Set(true)
	}
}

func (t *ComposerTextArea) moveUp(ke tui.KeyEvent) {
	lines := t.wrapText()
	row, col := t.cursorRowCol(lines)
	if row > 0 {
		prevLine := lines[row-1]
		prevLen := utf8.RuneCountInString(prevLine)
		if col > prevLen {
			col = prevLen
		}
		t.cursorPos.Set(t.posFromRowCol(lines, row-1, col))
		t.blink.Set(true)
	}
}

func (t *ComposerTextArea) moveDown(ke tui.KeyEvent) {
	lines := t.wrapText()
	row, col := t.cursorRowCol(lines)
	if row < len(lines)-1 {
		nextLine := lines[row+1]
		nextLen := utf8.RuneCountInString(nextLine)
		if col > nextLen {
			col = nextLen
		}
		t.cursorPos.Set(t.posFromRowCol(lines, row+1, col))
		t.blink.Set(true)
	}
}

func (t *ComposerTextArea) moveHome(ke tui.KeyEvent) {
	lines := t.wrapText()
	row, _ := t.cursorRowCol(lines)
	t.cursorPos.Set(t.posFromRowCol(lines, row, 0))
	t.blink.Set(true)
}

func (t *ComposerTextArea) moveEnd(ke tui.KeyEvent) {
	lines := t.wrapText()
	row, _ := t.cursorRowCol(lines)
	t.cursorPos.Set(t.posFromRowCol(lines, row, utf8.RuneCountInString(lines[row])))
	t.blink.Set(true)
}

func (t *ComposerTextArea) submit(ke tui.KeyEvent) {
	if t.onSubmit != nil {
		t.onSubmit(t.text.Get())
	}
}

func (t *ComposerTextArea) wrapLineLimit() int {
	if t.width <= 0 {
		return 0
	}
	if t.border == tui.BorderNone {
		return t.width
	}
	if t.width <= 2 {
		return 1
	}
	return t.width - 2
}

func (t *ComposerTextArea) wrapText() []string {
	text := t.text.Get()
	if text == "" {
		return []string{""}
	}

	var lines []string

	paragraphs := strings.Split(text, "\n")

	for _, para := range paragraphs {
		if para == "" {
			lines = append(lines, "")
			continue
		}

		limit := t.wrapLineLimit()
		currentLine := make([]rune, 0)
		for _, r := range para {
			if limit > 0 && len(currentLine) >= limit {
				lines = append(lines, string(currentLine))
				currentLine = currentLine[:0]
			}
			currentLine = append(currentLine, r)
		}
		lines = append(lines, string(currentLine))
	}

	return lines
}

func (t *ComposerTextArea) cursorRowCol(lines []string) (row, col int) {
	text := t.text.Get()
	pos := t.clampCursorPos()
	textRunes := []rune(text)

	currentRow := 0
	currentCol := 0
	lineIdx := 0

	for i := 0; i < len(textRunes) && i < pos; i++ {
		if textRunes[i] == '\n' {
			currentRow++
			currentCol = 0
			lineIdx++
		} else {
			currentCol++
			if t.wrapLineLimit() > 0 && lineIdx < len(lines) && currentCol > utf8.RuneCountInString(lines[lineIdx]) {
				currentRow++
				currentCol = 1
				lineIdx++
			}
		}
	}

	return currentRow, currentCol
}

func (t *ComposerTextArea) posFromRowCol(lines []string, targetRow, targetCol int) int {
	text := t.text.Get()
	textRunes := []rune(text)

	currentRow := 0
	currentCol := 0
	lineIdx := 0

	for i := 0; i < len(textRunes); i++ {
		if currentRow == targetRow && currentCol == targetCol {
			return i
		}

		if textRunes[i] == '\n' {
			if currentRow == targetRow {
				return i
			}
			currentRow++
			currentCol = 0
			lineIdx++
		} else {
			currentCol++
			if t.wrapLineLimit() > 0 && lineIdx < len(lines) && currentCol > utf8.RuneCountInString(lines[lineIdx]) {
				if currentRow == targetRow {
					return i
				}
				currentRow++
				currentCol = 1
				lineIdx++
			}
		}
	}

	return len(textRunes)
}

func (t *ComposerTextArea) lineWithCursor(lineIdx int) string {
	lines := t.wrapText()
	if lineIdx >= len(lines) {
		return " "
	}

	row, col := t.cursorRowCol(lines)
	line := lines[lineIdx]

	if lineIdx == row && t.focused.Get() {
		cursor := string(t.cursorRune)
		if !t.blink.Get() {
			cursor = " "
		}
		runes := []rune(line)
		if col >= len(runes) {
			return line + cursor
		}
		withCursor := append(runes[:col], append([]rune{t.cursorRune}, runes[col:]...)...)
		if !t.blink.Get() {
			withCursor[col] = ' '
		}
		return string(withCursor)
	}

	if line == "" {
		return " "
	}
	return line
}

func (t *ComposerTextArea) clampCursorPos() int {
	pos := t.cursorPos.Get()
	if pos < 0 {
		return 0
	}
	max := utf8.RuneCountInString(t.text.Get())
	if pos > max {
		return max
	}
	return pos
}
