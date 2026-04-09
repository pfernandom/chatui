package chat

import (
	"strings"
	"testing"

	tui "github.com/pfernandom/go-tui"
)

func TestComposerTextarea_Render_MaxHeightLimitsChildren(t *testing.T) {
	ta := NewComposerTextarea(
		ComposerWidth(20),
		ComposerMaxHeight(3),
		ComposerBorder(tui.BorderRounded),
	)
	ta.SetText(strings.Repeat("x", 200))

	root := ta.Render(nil)
	if got := len(root.Children()); got != 3 {
		t.Fatalf("rendered line children: got %d want 3 (extra rows must not be emitted past maxHeight)", got)
	}
}

func TestComposerTextarea_wrapLineLimit_WithBorder(t *testing.T) {
	ta := NewComposerTextarea(
		ComposerWidth(20),
		ComposerBorder(tui.BorderRounded),
	)
	if got := ta.wrapLineLimit(); got != 18 {
		t.Fatalf("wrapLineLimit with border: got %d want 18", got)
	}
}
