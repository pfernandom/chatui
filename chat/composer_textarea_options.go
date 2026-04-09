package chat

import tui "github.com/grindlemire/go-tui"

// ComposerTextareaOption configures a [ComposerTextArea].
type ComposerTextareaOption func(*ComposerTextArea)

// ComposerWidth sets the composer width in terminal cells.
func ComposerWidth(cells int) ComposerTextareaOption {
	return func(t *ComposerTextArea) {
		t.width = cells
	}
}

// ComposerMaxHeight sets the maximum visible wrapped rows (0 = unlimited).
func ComposerMaxHeight(rows int) ComposerTextareaOption {
	return func(t *ComposerTextArea) {
		t.maxHeight = rows
	}
}

// ComposerBorder sets the border style.
func ComposerBorder(b tui.BorderStyle) ComposerTextareaOption {
	return func(t *ComposerTextArea) {
		t.border = b
	}
}

// ComposerTextStyle sets the text style.
func ComposerTextStyle(s tui.Style) ComposerTextareaOption {
	return func(t *ComposerTextArea) {
		t.textStyle = s
	}
}

// ComposerPlaceholder sets placeholder text when empty and unfocused.
func ComposerPlaceholder(text string) ComposerTextareaOption {
	return func(t *ComposerTextArea) {
		t.placeholder = text
	}
}

// ComposerPlaceholderStyle sets placeholder style (default: dim).
func ComposerPlaceholderStyle(s tui.Style) ComposerTextareaOption {
	return func(t *ComposerTextArea) {
		t.placeholderStyle = s
	}
}

// ComposerCursor sets the cursor rune (default block cursor).
func ComposerCursor(r rune) ComposerTextareaOption {
	return func(t *ComposerTextArea) {
		t.cursorRune = r
	}
}

// ComposerFocusColor sets the border foreground when focused.
func ComposerFocusColor(c tui.Color) ComposerTextareaOption {
	return func(t *ComposerTextArea) {
		t.focusColor = &c
	}
}

// ComposerBorderGradient sets the border gradient when unfocused.
func ComposerBorderGradient(g tui.Gradient) ComposerTextareaOption {
	return func(t *ComposerTextArea) {
		t.borderGradient = &g
	}
}

// ComposerFocusGradient sets the border gradient when focused (overrides focus color).
func ComposerFocusGradient(g tui.Gradient) ComposerTextareaOption {
	return func(t *ComposerTextArea) {
		t.focusGradient = &g
	}
}

// ComposerSubmitKey sets the key that submits (default Enter; Ctrl+J inserts newline).
func ComposerSubmitKey(k tui.Key) ComposerTextareaOption {
	return func(t *ComposerTextArea) {
		t.submitKey = k
	}
}

// ComposerValue binds text to an external [tui.State] (two-way).
func ComposerValue(state *tui.State[string]) ComposerTextareaOption {
	return func(t *ComposerTextArea) {
		t.text = state
		t.cursorPos = tui.NewState(len([]rune(state.Get())))
	}
}

// ComposerAutoFocus sets whether the field requests focus on first mount.
func ComposerAutoFocus(auto bool) ComposerTextareaOption {
	return func(t *ComposerTextArea) {
		t.autoFocus = auto
	}
}

// ComposerOnSubmit sets the callback when the submit key is pressed.
func ComposerOnSubmit(fn func(string)) ComposerTextareaOption {
	return func(t *ComposerTextArea) {
		t.onSubmit = fn
	}
}
