package gpui_test

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/app"
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/elements"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/keymap"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/style"
)

// TestPromptRendersAndAnswers verifies the prompt lifecycle: Prompt opens a
// modal dialog that paints above the root (scrim + card quads), digit keys
// answer it, and the answer lands on the response and DismissPrompt.
func TestPromptRendersAndAnswers(t *testing.T) {
	w := openInteractivityWindow(t, 400, 300, func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv().ID(elements.ButtonID("root"))
	})
	quadsBefore := len(w.RenderedScene().Quads)

	resp := w.Prompt(gpui.PromptLevelWarning, "Delete file?", "This cannot be undone.", "",
		[]gpui.PromptAction{{Label: "Delete"}, {Label: "Cancel"}})
	if resp == nil {
		t.Fatal("Prompt returned nil")
	}
	if w.PromptCount() != 1 {
		t.Fatalf("prompt count = %d, want 1", w.PromptCount())
	}
	w.Draw()
	scene := w.RenderedScene()
	// The dialog adds at least the scrim, the card, the badge and two
	// buttons above the root's quads.
	if got := len(scene.Quads) - quadsBefore; got < 5 {
		t.Fatalf("prompt painted %d extra quads, want >= 5", got)
	}

	// Digit key "2" answers Cancel (index 1).
	w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Key: "2", KeyChar: "2"}})
	if idx, ok := resp.Index(); !ok || idx != 1 {
		t.Fatalf("answer = (%v, %v), want (1, true)", idx, ok)
	}
	if w.PromptCount() != 0 {
		t.Fatalf("prompt still open after answer: %d", w.PromptCount())
	}
	// The prompt layer is gone after a redraw.
	w.Draw()
	if w.PromptCount() != 0 {
		t.Fatalf("prompt count after redraw = %d, want 0", w.PromptCount())
	}
}

// TestPromptEscapeAndEnter verifies the keyboard conventions: Escape picks
// the last (cancel) action, Enter the first.
func TestPromptEscapeAndEnter(t *testing.T) {
	w := openInteractivityWindow(t, 400, 300, func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv()
	})
	resp := w.Prompt(gpui.PromptLevelInfo, "Title", "Message", "",
		[]gpui.PromptAction{{Label: "OK"}, {Label: "Cancel"}})
	w.Draw()
	w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Key: "escape"}})
	if idx, ok := resp.Index(); !ok || idx != 1 {
		t.Fatalf("escape answer = (%v, %v), want (1, true)", idx, ok)
	}

	resp2 := w.Prompt(gpui.PromptLevelInfo, "Title", "Message", "",
		[]gpui.PromptAction{{Label: "OK"}, {Label: "Cancel"}})
	w.Draw()
	w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Key: "enter"}})
	if idx, ok := resp2.Index(); !ok || idx != 0 {
		t.Fatalf("enter answer = (%v, %v), want (0, true)", idx, ok)
	}
}

// TestPromptDeduplicatesIdenticalTopPrompt verifies the upstream
// Option::None behavior: re-asking the same prompt on the top layer
// returns nil instead of stacking a duplicate.
func TestPromptDeduplicatesIdenticalTopPrompt(t *testing.T) {
	w := openInteractivityWindow(t, 400, 300, func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv()
	})
	actions := []gpui.PromptAction{{Label: "Yes"}, {Label: "No"}}
	if r := w.Prompt(gpui.PromptLevelInfo, "Same", "Same", "", actions); r == nil {
		t.Fatal("first prompt must open")
	}
	if r := w.Prompt(gpui.PromptLevelInfo, "Same", "Same", "", actions); r != nil {
		t.Fatal("identical top prompt must deduplicate (nil)")
	}
	if w.PromptCount() != 1 {
		t.Fatalf("prompt count = %d, want 1", w.PromptCount())
	}
	// A different prompt stacks on top and supersedes the old response.
	first := w.PromptAnswer(0)
	if r := w.Prompt(gpui.PromptLevelInfo, "Other", "Same", "", actions); r == nil {
		t.Fatal("different prompt must open")
	}
	select {
	case <-first.Changed():
	default:
		t.Fatal("old response must be superseded")
	}
}

// TestPromptButtonClick verifies buttons answer via mouse: down+up on a
// button rect commits that action.
func TestPromptButtonClick(t *testing.T) {
	w := openInteractivityWindow(t, 400, 300, func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv()
	})
	resp := w.Prompt(gpui.PromptLevelInfo, "Title", "Message", "",
		[]gpui.PromptAction{{Label: "Save"}, {Label: "Discard"}})
	w.Draw()

	// Locate a button hitbox from the rendered hitboxes: the prompt's
	// button hitboxes sit in the lower card area. Find the hitbox whose
	// bounds are inside the window center region and click its center.
	var buttonBounds geometry.Bounds[geometry.Pixels]
	found := false
	for _, hb := range w.RenderedHitboxes() {
		b := hb.Bounds
		cx := b.Origin.X.AsF32() + b.Size.Width.AsF32()/2
		cy := b.Origin.Y.AsF32() + b.Size.Height.AsF32()/2
		// Button rows live in the bottom half; skip the full-window scrim
		// and the card-sized blockers.
		if b.Size.Width.AsF32() > 20 && b.Size.Width.AsF32() < 200 &&
			b.Size.Height.AsF32() > 10 && cy > 150 && cx > 200 {
			buttonBounds = b
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no button hitbox found")
	}
	center := geometry.PointPixels(
		buttonBounds.Origin.X.AsF32()+buttonBounds.Size.Width.AsF32()/2,
		buttonBounds.Origin.Y.AsF32()+buttonBounds.Size.Height.AsF32()/2)
	w.HandleInput(platform.MouseDownEvent{Position: center, Button: platform.ButtonLeft, ClickCount: 1})
	w.HandleInput(platform.MouseUpEvent{Position: center, Button: platform.ButtonLeft, ClickCount: 1})
	if idx, ok := resp.Index(); !ok {
		t.Fatalf("button click did not answer (idx=%v ok=%v)", idx, ok)
	}
}

// TestPromptForInputCommits verifies the input prompt: typing edits the
// field, Enter commits the value, Escape cancels without committing.
func TestPromptForInputCommits(t *testing.T) {
	w := openInteractivityWindow(t, 400, 300, func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv()
	})
	resp := w.PromptForInput(gpui.PromptLevelInfo, "Project name", "Enter a name", "", "name...", "")
	if resp == nil {
		t.Fatal("PromptForInput returned nil")
	}
	w.Draw()

	typeKey := func(key, ch string) {
		w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Key: key, KeyChar: ch}})
	}
	typeKey("d", "d")
	typeKey("e", "e")
	typeKey("m", "m")
	typeKey("o", "o")
	// Backspace deletes the last rune.
	typeKey("backspace", "")
	typeKey("2", "2")
	typeKey("enter", "")
	v, ok := resp.Value()
	if !ok || v != "dem2" {
		t.Fatalf("input value = (%q, %v), want (\"dem2\", true)", v, ok)
	}
	if w.PromptCount() != 0 {
		t.Fatalf("prompt still open after enter")
	}

	// Escape path.
	resp2 := w.PromptForInput(gpui.PromptLevelInfo, "Title", "Msg", "", "ph", "preset")
	w.Draw()
	typeKey("escape", "")
	if _, ok := resp2.Value(); ok {
		t.Fatal("escape must not commit")
	}
	if w.PromptCount() != 0 {
		t.Fatal("prompt still open after escape")
	}
}

// TestPromptDismissPromptAPI verifies DismissPrompt and PromptAnswer layer
// indexing.
func TestPromptDismissPromptAPI(t *testing.T) {
	w := openInteractivityWindow(t, 400, 300, func(w *gpui.Window, cx *app.App) gpui.Element {
		return elements.NewDiv()
	})
	w.Prompt(gpui.PromptLevelInfo, "A", "a", "", []gpui.PromptAction{{Label: "OK"}})
	w.Draw()
	if w.PromptAnswer(0) == nil {
		t.Fatal("layer 0 must have an answer handle")
	}
	if w.DismissPrompt(0) == nil {
		t.Fatal("DismissPrompt(0) must return the response")
	}
	if w.PromptCount() != 0 {
		t.Fatal("prompt not closed")
	}
	if w.DismissPrompt(0) != nil {
		t.Fatal("dismissing an empty layer returns nil")
	}
}

// TestPromptStyling compiles the prompt's interaction with style defaults
// (a smoke check that the dialog renders without panicking when the window
// text style is customized).
func TestPromptStyling(t *testing.T) {
	w := openInteractivityWindow(t, 400, 300, func(w *gpui.Window, cx *app.App) gpui.Element {
		d := elements.NewDiv()
		d.StyleRef().TextColor(gc.RGB(0x888888).Hsla())
		return d
	})
	_ = style.DefaultStyle()
	w.Prompt(gpui.PromptLevelCritical, "Fatal", "Everything broke", "details here", nil)
	w.Draw()
	if w.PromptCount() != 1 {
		t.Fatalf("default-action prompt must open, count=%d", w.PromptCount())
	}
	w.HandleInput(platform.KeyDownEvent{Keystroke: keymap.Keystroke{Key: "enter"}})
	if w.PromptCount() != 0 {
		t.Fatal("prompt must close on enter")
	}
}
