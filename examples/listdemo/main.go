// Command listdemo is a 1000-row virtualized list built with go-gpui's
// UniformList element: a header row, alternating row colors, row selection
// on click, wheel scrolling, a ScrollHandle-driven goto-row control with
// autoscroll, and accessibility annotations on the rows. Only the visible
// rows are laid out, hit-tested and painted.
//
// By default it opens an interactive native window; with --headless it
// renders PNGs and verifies virtualization, selection, wheel scrolling and
// overscroll clamping through the input pipeline:
//
//	go run ./examples/listdemo
//	go run ./examples/listdemo --headless --out /tmp/frames
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/FlameInTheDark/go-gpui/app"
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/elements"
	"github.com/FlameInTheDark/go-gpui/examples/internal/demo"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/headless"
)

var (
	listBg    = gc.RGB(0x1e1e2e).Hsla()
	rowAlt    = gc.RGB(0x24243a).Hsla()
	selBg     = gc.RGB(0x313244).Hsla()
	selAccent = gc.RGB(0x89b4fa).Hsla()
	textDim   = gc.RGB(0x6c7086).Hsla()
	textMain  = gc.RGB(0xcdd6f4).Hsla()
	green     = gc.RGB(0xa6e3a1).Hsla()
	peach     = gc.RGB(0xfab387).Hsla()
	surface   = gc.RGB(0x181825).Hsla()
	headerBg  = gc.RGB(0x11111b).Hsla()
)

// listState tracks the selection and the goto target.
type listState struct {
	selected int
}

// scroll tracks the list's scroll offset externally (TrackScroll +
// Autoscroll: the goto buttons drive it, user wheel input cancels the
// follow).
var scroll *elements.ScrollHandle

// The list element is hoisted so the verification code can query it.
var list *elements.UniformList

func main() {
	a := demo.Start[listState](demo.Config{
		Name:  "listdemo",
		Title: "go-gpui virtualized list",
		Size:  geometry.SizePixels(480, 520),
	}, func(w *gpui.Window, cx *app.Context[listState]) listState {
		return listState{selected: -1}
	})
	cx := a.App.App
	px := func(v float32) geometry.Length { return geometry.LengthFromPixels(geometry.Px(v)) }

	a.SetRoot(func(w *gpui.Window, c *app.App) gpui.Element {
		state := a.Handle.View.Read(cx)

		header := elements.NewDiv().Flex().ItemsCenter().Px_3().H(px(30)).BgColor(headerBg).
			Child(elements.NewText("Virtualized list - 1000 rows").WithTextColor(textDim)).
			Child(elements.NewDiv().FlexGrow(1)).
			Child(elements.NewText("only visible rows are laid out").WithTextColor(textDim))

		list = elements.NewUniformList(1000, func(ix int, w *gpui.Window) gpui.Element {
			rowBg := listBg
			if ix%2 == 1 {
				rowBg = rowAlt
			}
			selected := ix == state.selected
			if selected {
				rowBg = selBg
			}
			row := elements.NewDiv().Flex().ItemsCenter().Px_3().H(px(28)).BgColor(rowBg).
				ID(elements.ButtonID(fmt.Sprintf("row-%d", ix))).
				A11y(gpui.AccessibilityListItem, fmt.Sprintf("Row %d", ix)).
				OnClick(func(ev gpui.ClickEvent, w *gpui.Window) {
					a.Handle.View.Update(cx, func(s *listState, c *app.Context[listState]) any {
						s.selected = ix
						return nil
					})
					w.Draw()
				})
			if selected {
				row = row.BorderL_2().BorderColor(selAccent)
			}
			bullet := green
			if ix%3 == 1 {
				bullet = peach
			} else if ix%3 == 2 {
				bullet = selAccent
			}
			return row.
				Child(elements.NewDiv().W(px(8)).H(px(8)).Rounded_Full().BgColor(bullet).Mr_3()).
				Child(elements.NewText(fmt.Sprintf("Row %d", ix)).WithTextColor(textMain)).
				Child(elements.NewDiv().FlexGrow(1)).
				Child(elements.NewText(fmt.Sprintf("3x%d = %d", ix, 3*ix)).WithTextColor(textDim))
		})

		selText := " none"
		selColor := textDim
		if state.selected >= 0 {
			selText = fmt.Sprintf(" Row %d", state.selected)
			selColor = selAccent
		}
		footer := elements.NewDiv().Flex().ItemsCenter().Px_3().H(px(28)).BgColor(surface).
			Child(elements.NewText("Selection:").WithTextColor(textDim)).
			Child(elements.NewText(selText).WithTextColor(selColor))

		if scroll == nil {
			scroll = elements.NewScrollHandle()
		}
		gotoButton := func(label string, target func(h *elements.ScrollHandle)) *elements.Div {
			return elements.NewDiv().
				ID(elements.ButtonID("goto-"+label)).
				A11y(gpui.AccessibilityButton, "go to "+label).
				Px_3().Rounded_Md().BgColor(selBg).
				Child(elements.NewText(label).WithTextColor(selAccent)).
				OnClick(func(ev gpui.ClickEvent, w *gpui.Window) {
					target(scroll)
					w.Refresh()
				})
		}
		controls := elements.NewDiv().Flex().ItemsCenter().Gap_2().
			Child(gotoButton("Top", func(h *elements.ScrollHandle) { h.ScrollToTop() })).
			Child(gotoButton("Row 500", func(h *elements.ScrollHandle) { h.SetTop(geometry.Px(500 * 28)) })).
			Child(gotoButton("Bottom", func(h *elements.ScrollHandle) { h.ScrollToBottom() })).
			Child(gotoButton("Follow", func(h *elements.ScrollHandle) { h.Autoscroll() }))

		return elements.NewDiv().Flex().FlexCol().FlexGrow(1).BgColor(listBg).
			Child(header).
			Child(elements.NewDiv().Flex1().Child(list)).
			Child(controls).
			Child(footer)
	})

	if a.Headless == nil {
		a.Loop() // blocks until the window is closed
		return
	}
	verifyHeadless(a, cx)
}

// verifyHeadless drives the list through the input pipeline.
func verifyHeadless(a *demo.App[listState], cx *app.App) {
	hw := a.Headless.LastWindow()
	out := func(name string) string { return filepath.Join(a.OutDir, name) }

	// Frame 1: top of the list; rows 0..N visible, few hitboxes (virtualized).
	first, last := list.VisibleRange()
	if first != 0 || last > 20 {
		demo.Fatal("listdemo", fmt.Errorf("initial visible range = [%d,%d), want small window starting at 0", first, last))
	}
	// Visible rows (~19) + the goto control buttons stay far below 1000.
	if n := len(a.Handle.Window.RenderedHitboxes()); n > 30 {
		demo.Fatal("listdemo", fmt.Errorf("%d hitboxes for a 1000-row list: virtualization broken", n))
	}
	if err := savePNG(hw, out("list_top.png")); err != nil {
		demo.Fatal("listdemo", err)
	}

	// Click row 5 (header is 30px, rows are 28px tall).
	click(hw, 200, 30+5*28+14)
	a.Handle.Window.Draw()
	if state := a.Handle.View.Read(cx); state.selected != 5 {
		demo.Fatal("listdemo", fmt.Errorf("click selected row %d, want 5", state.selected))
	}
	if err := savePNG(hw, out("list_selected.png")); err != nil {
		demo.Fatal("listdemo", err)
	}

	// Scroll down 10 wheel ticks x 3 lines x 20px/line = 600px (negative = down).
	for i := 0; i < 10; i++ {
		hw.DispatchInput(platform.ScrollEvent{
			Position: geometry.PointPixels(200, 200),
			Delta:    platform.ScrollDeltaLines(0, -3),
		})
	}
	a.Handle.Window.Draw()
	first, last = list.VisibleRange()
	if first < 15 || first > 25 {
		demo.Fatal("listdemo", fmt.Errorf("after 600px of scrolling, first visible row = %d, want ~21", first))
	}
	if err := savePNG(hw, out("list_scrolled.png")); err != nil {
		demo.Fatal("listdemo", err)
	}

	// Click a row in the scrolled view. After 600px of scrolling with 28px
	// rows, row `first` is partially clipped at the top; the next row sits at
	// y = 30 + 28 (row height) + 14 (half) = 72.
	click(hw, 200, 72)
	a.Handle.Window.Draw()
	if state := a.Handle.View.Read(cx); state.selected != first+1 {
		demo.Fatal("listdemo", fmt.Errorf("clicked row at y=72 selected %d, want %d", state.selected, first+1))
	}

	// Overscroll far past the end (500 ticks x 60px = 30000px > scrollMax
	// 27658px): must clamp to the last screenful.
	for i := 0; i < 500; i++ {
		hw.DispatchInput(platform.ScrollEvent{
			Position: geometry.PointPixels(200, 200),
			Delta:    platform.ScrollDeltaLines(0, -3),
		})
	}
	a.Handle.Window.Draw()
	first, last = list.VisibleRange()
	// The 520px-tall window shows ~17 rows, so the last screenful starts
	// around row 983; anything past the end must clamp to it.
	if last != 1000 || first < 975 {
		demo.Fatal("listdemo", fmt.Errorf("overscroll clamp: visible range = [%d,%d), want end at 1000", first, last))
	}
	if err := savePNG(hw, out("list_end.png")); err != nil {
		demo.Fatal("listdemo", err)
	}

	demo.Printf("listdemo", "virtualization, selection, wheel scroll and clamping verified")
}

func click(hw *headless.Window, x, y float32) {
	pos := geometry.PointPixels(x, y)
	hw.DispatchInput(platform.MouseDownEvent{Button: platform.ButtonLeft, Position: pos})
	hw.DispatchInput(platform.MouseUpEvent{Button: platform.ButtonLeft, Position: pos})
}

func savePNG(hw *headless.Window, path string) error {
	pngBytes, err := hw.PNG()
	if err != nil {
		return err
	}
	return os.WriteFile(path, pngBytes, 0o644)
}
