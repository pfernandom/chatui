package tui

import (
	"strings"
	"testing"
)

func TestTextArea_Render_MaxHeightLimitsChildren(t *testing.T) {
	ta := NewTextArea(
		WithTextAreaWidth(20),
		WithTextAreaMaxHeight(3),
		WithTextAreaBorder(BorderRounded),
	)
	// Inner width is 18; many soft-wrapped lines of 'x'.
	ta.SetText(strings.Repeat("x", 200))

	root := ta.Render(nil)
	if got := len(root.Children()); got != 3 {
		t.Fatalf("rendered line children: got %d want 3 (extra rows must not be emitted past maxHeight)", got)
	}
}

func TestTextArea_wrapLineLimit_WithBorder(t *testing.T) {
	ta := NewTextArea(
		WithTextAreaWidth(20),
		WithTextAreaBorder(BorderRounded),
	)
	if got := ta.wrapLineLimit(); got != 18 {
		t.Fatalf("wrapLineLimit with border: got %d want 18", got)
	}
}
