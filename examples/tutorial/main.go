// Command tutorial is the companion app for docs/tutorial.md: a small
// task list built directly on go-gpui's public API, with no helper
// harness. Every concept the tutorial introduces - the application and
// window lifecycle, the state entity and its render method, flexbox
// layout, styling, interactivity, keyboard focus, a text input built
// from key events, and the modal prompt - appears here once, in the
// order the document introduces it.
//
// Run it:
//
//	go run ./examples/tutorial
//
// Interactions:
//
//   - type into the input, press Enter to add a task
//   - click a task to toggle it done, click the x to remove it
//   - Tab / Shift+Tab moves keyboard focus between input and list
//   - when the list has focus: Up/Down select, Space toggles, Delete removes
//   - "Clear done" empties the finished tasks after a confirm prompt
//   - closing the window with unfinished tasks asks for confirmation
package main

import (
	"fmt"
	"os"
	"strings"

	ui "github.com/FlameInTheDark/go-gpui"
	"github.com/FlameInTheDark/go-gpui/app"
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/elements"
	"github.com/FlameInTheDark/go-gpui/font"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/auto"
	"github.com/FlameInTheDark/go-gpui/style"
	"github.com/FlameInTheDark/go-gpui/text"
)

// ---------------------------------------------------------------------------
// Theme: plain color values. A real application would centralize these in
// its own ui package (the editor example does); a tutorial keeps them
// where you can see them.
// ---------------------------------------------------------------------------

var (
	windowBg = gc.RGB(0x1e1e2e).Hsla()
	surface  = gc.RGB(0x313244).Hsla()
	accent   = gc.RGB(0x89b4fa).Hsla()
	hover    = gc.RGB(0x45475a).Hsla()
	red      = gc.RGB(0xf38ba8).Hsla()
	textC    = gc.RGB(0xcdd6f4).Hsla()
	dim      = gc.RGB(0x6c7086).Hsla()
)

// The UI font: any family name the system resolves; the text stack falls
// back per rune when glyphs are missing.
var uiFont = font.Font{Family: "Monospace"}

// ---------------------------------------------------------------------------
// State. The view state is a plain Go struct; go-gpui wraps it in a
// generation-tagged entity. Mutate it on the UI goroutine via its context.
// ---------------------------------------------------------------------------

type task struct {
	title string
	done  bool
}

type taskList struct {
	tasks []task
	// draft is the input's pending text (typed but not yet added).
	draft string
	// selected is the list's keyboard cursor (-1 = none).
	selected int
	// focus handles: one for the input, one for the list. Handles are
	// created once per window and compared by identity.
	inputFocus gpui.FocusHandle
	listFocus  gpui.FocusHandle
	dispatch   func(func())
}

// newTaskList returns the constructor that go-gpui calls when the window opens.
func newTaskList(p platform.Platform) func(*gpui.Window, *app.Context[taskList]) taskList {
	return func(*gpui.Window, *app.Context[taskList]) taskList {
		return taskList{
			tasks: []task{
				{title: "Read the tutorial", done: true},
				{title: "Build an app"},
			},
			selected:   1,
			inputFocus: gpui.NewFocusHandle(),
			listFocus:  gpui.NewFocusHandle(),
			dispatch:   p.DispatchOnMainThread,
		}
	}
}

// ---------------------------------------------------------------------------
// Render. Called whenever the view is invalidated: rebuild the element
// tree from the current state; the framework diffs nothing, it just
// paints the new tree.
// ---------------------------------------------------------------------------

func (s *taskList) render(w *gpui.Window, cx *app.Context[taskList]) gpui.Element {
	// Capture during render, then call from an event handler to mutate and notify.
	update := cx.Updater()

	header := elements.NewDiv().
		Flex().ItemsCenter().JustifyBetween().
		Px_4().Py_2().BorderB_1().BorderColor(surface).
		Child(label("go-gpui tutorial", textC)).
		Child(label(fmt.Sprintf("%d left", s.remaining()), dim))

	input := s.renderInput(w, update)
	body := s.renderList(w, update)

	return elements.NewDiv().
		Flex().FlexCol().FlexGrow(1).
		BgColor(windowBg).Font(uiFont).
		Child(header).
		Child(input).
		Child(body).
		Child(s.renderFooter(update))
}

// label is a convenience for a monochrome text element.
func label(value string, color gc.Hsla) *elements.Text {
	return elements.NewText(value).WithTextColor(color)
}

// renderInput is a single-line text input built from primitives: a focus
// handle, a key handler and a caret. go-gpui ships layout, painting and
// events; widgets like this are composed, not built in.
func (s *taskList) renderInput(w *gpui.Window, update func(func(*taskList))) gpui.Element {
	placeholder := "Add a task and press Enter..."

	value := label(placeholder, dim)
	if s.draft != "" {
		value = label(s.draft, textC)
	}

	caretColor := dim
	if w.ContainsFocused(s.inputFocus) {
		caretColor = accent
	}

	return elements.NewDiv().
		ID(elements.ButtonID("task-input")).
		Flex().ItemsCenter().Gap_2().
		Mx_4().Mt_4().Px_4().Py_2().
		BgColor(surface).Rounded_Lg().CursorText().
		TrackFocus(s.inputFocus). // clicking the box focuses the handle
		OnKeyDown(func(ev platform.KeyDownEvent, w *gpui.Window) {
			ks := ev.Keystroke
			if ks.Modifiers.Control || ks.Modifiers.Alt || ks.Modifiers.Platform {
				return // leave shortcuts to the platform/window
			}
			switch ks.Key {
			case "backspace":
				update(func(state *taskList) {
					if state.draft != "" {
						// chop one rune (UTF-8 aware)
						r := []rune(state.draft)
						state.draft = string(r[:len(r)-1])
					}
				})
			case "enter", "return":
				title := strings.TrimSpace(s.draft)
				if title != "" {
					update(func(state *taskList) {
						state.tasks = append(state.tasks, task{title: title})
						state.draft = ""
						state.selected = len(state.tasks) - 1
						w.Focus(state.listFocus)
					})
				}
			case "tab":
				update(func(state *taskList) { w.Focus(state.listFocus) })
			case "escape":
				update(func(state *taskList) { state.draft = "" })
			default:
				// A printable character: KeyChar carries the typed rune.
				if ch := ks.KeyChar; ch != "" && len([]rune(ch)) == 1 {
					update(func(state *taskList) { state.draft += ch })
				}
			}
		}).
		Child(value).
		Child(elements.NewDiv(). // the caret
						W(ui.Px(1)).
						H(ui.Px(14)).
						BgColor(caretColor))
}

// renderList paints the tasks: a click toggles done, the x removes, and
// the selected row is highlighted. The keyboard handling lives on the
// list container (focused through Tab).
func (s *taskList) renderList(w *gpui.Window, update func(func(*taskList))) gpui.Element {
	if len(s.tasks) == 0 {
		return elements.NewDiv().
			Flex().FlexCol().Flex1().ItemsCenter().JustifyCenter().
			Child(label("Nothing to do (yet)", dim))
	}

	rows := elements.NewDiv().Flex().FlexCol().Gap_2().P_4().FlexGrow(1)
	for i, t := range s.tasks {
		rows = rows.Child(s.renderRow(i, t, w, update))
	}

	return elements.NewDiv().
		ID(elements.ButtonID("task-list")).
		Flex().FlexCol().FlexGrow(1).OverflowHidden().
		TrackFocus(s.listFocus).
		OnKeyDown(func(ev platform.KeyDownEvent, w *gpui.Window) {
			ks := ev.Keystroke
			if ks.Modifiers.Control || ks.Modifiers.Alt {
				return
			}
			update(func(state *taskList) {
				switch ks.Key {
				case "up":
					if state.selected > 0 {
						state.selected--
					}
				case "down":
					if state.selected < len(state.tasks)-1 {
						state.selected++
					}
				case "space":
					if state.selected >= 0 && state.selected < len(state.tasks) {
						state.tasks[state.selected].done = !state.tasks[state.selected].done
					}
				case "delete", "backspace":
					if state.selected >= 0 && state.selected < len(state.tasks) {
						state.tasks = append(state.tasks[:state.selected], state.tasks[state.selected+1:]...)
						if state.selected >= len(state.tasks) {
							state.selected = len(state.tasks) - 1
						}
					}
				case "tab":
					if ks.Modifiers.Shift {
						w.Focus(state.inputFocus)
					}
				}
			})
		}).
		Child(rows)
}

// renderRow renders one task row.
func (s *taskList) renderRow(ix int, t task, w *gpui.Window, update func(func(*taskList))) gpui.Element {
	titleColor := textC
	strike := (*style.StrikethroughStyle)(nil)
	if t.done {
		titleColor = dim
		strike = &style.StrikethroughStyle{Thickness: 1}
	}

	bg := surface
	if ix == s.selected && w.ContainsFocused(s.listFocus) {
		bg = hover
	}

	return elements.NewDiv().
		ID(elements.ButtonID(fmt.Sprintf("task-row-%d", ix))).
		Flex().ItemsCenter().JustifyBetween().
		Px_4().Py_2().Rounded_Lg().BgColor(bg).
		Hover(func(r *style.StyleRefinement) { r.BgColor(hover) }).
		OnClick(func(ev gpui.ClickEvent, w *gpui.Window) {
			update(func(state *taskList) {
				if ix < len(state.tasks) {
					state.tasks[ix].done = !state.tasks[ix].done
				}
			})
		}).
		Child(elements.NewText(t.title).
			WithRuns([]text.TextRun{{Len: len(t.title), Font: uiFont, Color: titleColor, Strikethrough: strike}})).
		Child(elements.NewDiv().
			ID(elements.ButtonID(fmt.Sprintf("task-del-%d", ix))).
			Px_2().CursorPointer().
			Child(label("x", red)).
			OnClick(func(ev gpui.ClickEvent, w *gpui.Window) {
				w.StopPropagation() // don't toggle the parent row after deleting it
				update(func(state *taskList) {
					state.tasks = append(state.tasks[:ix], state.tasks[ix+1:]...)
					if state.selected >= len(state.tasks) {
						state.selected = len(state.tasks) - 1
					}
				})
			}))
}

// renderFooter has the clear-done button and the confirm prompt.
func (s *taskList) renderFooter(update func(func(*taskList))) gpui.Element {
	clear := elements.NewDiv().
		Mx_4().Mb_4().Px_4().Py_2().Rounded_Lg().
		BgColor(accent).CursorPointer().
		Hover(func(r *style.StyleRefinement) { r.BgColor(hover) }).
		Child(label("Clear done", windowBg)).
		OnClick(func(ev gpui.ClickEvent, w *gpui.Window) {
			if s.remaining() == len(s.tasks) {
				return // nothing marked done: no prompt
			}
			// The modal prompt system: a response arrives after the user
			// answers (possibly several frames later), so wait on the
			// completion channel from a goroutine.
			resp := w.Prompt(gpui.PromptLevelWarning, "Clear finished tasks?",
				fmt.Sprintf("%d finished tasks will be removed.", len(s.tasks)-s.remaining()), "",
				[]gpui.PromptAction{{Label: "Clear"}, {Label: "Cancel"}})
			if resp == nil {
				return // identical prompt already open
			}
			dispatch := s.dispatch
			go func() {
				<-resp.Changed() // fires when answered or superseded
				if idx, ok := resp.Index(); ok && idx == 0 {
					dispatch(func() {
						update(func(state *taskList) {
							kept := state.tasks[:0]
							for _, t := range state.tasks {
								if !t.done {
									kept = append(kept, t)
								}
							}
							state.tasks = kept
							if state.selected >= len(state.tasks) {
								state.selected = len(state.tasks) - 1
							}
						})
					})
				}
			}()
		})

	return elements.NewDiv().
		Flex().FlexCol().
		Child(elements.NewDiv().Flex().JustifyEnd().Child(clear)).
		Child(elements.NewDiv().
			Flex().Px_4().Py_2().BorderT_1().BorderColor(surface).
			Child(label("Tab switches focus - Space toggles - Del removes", dim)))
}

// remaining counts unfinished tasks.
func (s *taskList) remaining() int {
	n := 0
	for _, t := range s.tasks {
		if !t.done {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// main explicitly selects a backend and customizes the window close callback.
// ---------------------------------------------------------------------------

func main() {
	// 1. The platform backend: win32 on Windows, X11 or Wayland on Linux.
	plat, err := auto.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "tutorial:", err)
		os.Exit(1)
	}

	// 2. The application owns entities and effect scheduling.
	appl := gpui.NewApplication(plat)

	// 3. Open the window; the constructor builds the root view's state.
	// shouldClose is wired after the window exists (the veto needs the
	// window's prompt API).
	var shouldClose func() bool
	bounds := platform.WindowedBounds(geometry.BoundsPixels(0, 0, 420, 520))
	handle, err := gpui.OpenViewWith(appl,
		platform.WindowOptions{Title: "go-gpui tutorial", WindowBounds: &bounds},
		newTaskList(plat), (*taskList).render,
		func(callbacks *platform.WindowCallbacks) {
			// Veto closing while tasks are unfinished (the modal prompt
			// drives the decision).
			callbacks.OnShouldClose = func() bool {
				if shouldClose == nil {
					return true
				}
				return shouldClose()
			}
		})
	if err != nil {
		fmt.Fprintln(os.Stderr, "tutorial:", err)
		os.Exit(1)
	}
	shouldClose = func() bool {
		s := handle.View.Read(appl.App)
		if s.remaining() == 0 {
			return true
		}
		resp := handle.Window.Prompt(gpui.PromptLevelWarning, "Quit?",
			"There are unfinished tasks.", "", []gpui.PromptAction{{Label: "Quit"}, {Label: "Cancel"}})
		if resp == nil {
			return false
		}
		go func() {
			<-resp.Changed()
			if idx, ok := resp.Index(); ok && idx == 0 {
				plat.DispatchOnMainThread(plat.Quit)
			}
		}()
		return false
	}

	// 4. Run the platform event loop until the window closes.
	appl.Run()
}
