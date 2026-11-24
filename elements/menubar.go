package elements

import (
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/style"
	"github.com/FlameInTheDark/go-gpui/text"
)

// MenuBar renders the window's application menu (w.AppMenu(), installed
// with gpui.Window.SetAppMenu) as an in-app horizontal bar with dropdowns
// — the Windows/Linux counterpart of upstream's native macOS menubar
// (the upstream editor renders the same structure in its title bar).
//
// Interaction matches platform conventions: a click opens a dropdown and
// keeps it open until dismissed (click-outside, Escape, or item
// activation); hovering sibling titles while open switches between them;
// ArrowLeft/ArrowRight move between menus, ArrowUp/ArrowDown move the
// selection, Enter activates, Escape closes. Items dispatch typed actions
// through the window's action pipeline (or their handler).
type MenuBar struct {
	st *style.StyleRefinement
}

// NewMenuBar creates the bar.
func NewMenuBar() *MenuBar {
	return &MenuBar{st: style.NewStyleRefinement()}
}

// StyleRef returns the refinement (bar-level styling: background,
// height...).
func (m *MenuBar) StyleRef() *style.StyleRefinement { return m.st }

// ElementID implements Element.
func (m *MenuBar) ElementID() gpui.ElementId { return gpui.NameElementId("menu-bar") }

// menuBarState is the bar's persistent interaction state.
type menuBarState struct {
	// open is the index of the open menu (-1 closed).
	open int
	// hoverBar is the hovered title index (-1 none); while a menu is
	// open, hovering a sibling title switches to it (platform behavior).
	hoverBar int
	// hoverItem is the selected item index inside the open menu.
	hoverItem int
	// origin is the open title's lower-left anchor (dropdown places
	// under it).
	origin geometry.Point[geometry.Pixels]
	// anchorWidth is the open title's width (dropdown min alignment).
	anchorWidth float32
}

// RequestLayout implements Element.
func (m *MenuBar) RequestLayout(w *gpui.Window) {
	st := style.DefaultStyle()
	st.Refine(m.st)
	st.Size.Height = geometry.LengthFromDefinite(geometry.PixelsDefinite(geometry.Px(remOf(w) * 2.25)))
	w.RequestLayout(st, nil)
}

// Prepaint implements Element.
func (m *MenuBar) Prepaint(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {}

// remOf resolves the rem size with a floor.
func remOf(w *gpui.Window) float32 {
	rem := w.RemSize().AsF32()
	if rem <= 0 {
		rem = 16
	}
	return rem
}

// menuBarFontRuns builds a single-run font slice for the window base
// font.
func menuBarFontRuns(w *gpui.Window, str string) []text.FontRun {
	id, _ := w.TextSystem().FontID(w.TextStyle().ToFont())
	return []text.FontRun{{Len: len(str), FontID: id}}
}

// Paint implements Element.
func (m *MenuBar) Paint(w *gpui.Window) {
	colors := w.DefaultColors()
	menus := w.AppMenu()
	if len(menus) == 0 {
		return
	}
	rem := remOf(w)

	var state menuBarState
	gpui.UseElementState(w, func() menuBarState {
		return menuBarState{open: -1, hoverBar: -1, hoverItem: -1}
	}, func(s *menuBarState) {
		state = *s
	})

	bounds := w.ElementBounds(m)
	gid := w.CurrentElementID()

	// Bar background.
	st := style.DefaultStyle()
	st.Refine(m.st)
	if st.Background == nil {
		bg := gc.SolidBackground(opaque(colors.Container.Hsla(), 0.55))
		st.Background = &bg
	}
	w.PaintQuad(st, bounds)

	fontSize := style.DefaultStyle().Text.FontSize.ToPixels(w.RemSize())
	itemPad := rem * 0.75
	x := bounds.Origin.X.AsF32() + rem*0.5
	centerY := bounds.Origin.Y.AsF32() + bounds.Size.Height.AsF32()/2

	for i, menu := range menus {
		l := w.TextSystem().LayoutLine(menu.Name, fontSize, menuBarFontRuns(w, menu.Name))
		titleW := l.Width.AsF32() + itemPad*2
		titleBounds := geometry.Bounds[geometry.Pixels]{
			Origin: geometry.PointPixels(x, bounds.Origin.Y.AsF32()),
			Size:   geometry.SizePixels(titleW, bounds.Size.Height.AsF32()),
		}
		if state.open == i || state.hoverBar == i {
			hl := colors.Selected.Hsla()
			hl.A = 0.6
			hlst := style.DefaultStyle()
			hb := gc.SolidBackground(hl)
			hlst.Background = &hb
			w.PaintQuad(hlst, titleBounds)
		}
		fg := colors.Text.Hsla()
		paintMenuLine(w, l, geometry.PointPixels(
			x+itemPad,
			centerY-(l.Ascent.AsF32()+l.Descent.AsF32())/2+l.Ascent.AsF32()), fg)

		idx := i
		tb := titleBounds
		hb := w.InsertHitbox(tb, gpui.HitboxNormal)
		w.OnMouseEvent("move", platform.ButtonLeft, hb, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			gpui.UpdateElementStateAt(w, gid, func() menuBarState { return menuBarState{open: -1, hoverBar: -1, hoverItem: -1} }, func(s *menuBarState) {
				s.hoverBar = idx
				// While a menu is open, hovering a sibling title switches
				// to it (platform menu behavior).
				if s.open >= 0 && s.open != idx {
					s.open = idx
					s.hoverItem = -1
					s.origin = geometry.PointPixels(tb.Origin.X.AsF32(), tb.Origin.Y.AsF32()+tb.Size.Height.AsF32())
					s.anchorWidth = tb.Size.Width.AsF32()
				}
			})
			w.Refresh()
		})
		w.OnMouseEvent("exit", platform.ButtonLeft, hb, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			gpui.UpdateElementStateAt(w, gid, func() menuBarState { return menuBarState{open: -1, hoverBar: -1, hoverItem: -1} }, func(s *menuBarState) {
				if s.hoverBar == idx {
					s.hoverBar = -1
				}
			})
			w.Refresh()
		})
		w.OnMouseEvent("down", platform.ButtonLeft, hb, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			gpui.UpdateElementStateAt(w, gid, func() menuBarState { return menuBarState{open: -1, hoverBar: -1, hoverItem: -1} }, func(s *menuBarState) {
				if s.open == idx {
					s.open = -1 // toggle closed
					s.hoverItem = -1
				} else {
					s.open = idx
					s.hoverItem = -1
					s.origin = geometry.PointPixels(tb.Origin.X.AsF32(), tb.Origin.Y.AsF32()+tb.Size.Height.AsF32())
					s.anchorWidth = tb.Size.Width.AsF32()
				}
			})
			w.Refresh()
		})
		x += titleW
	}

	// The open dropdown paints as a deferred overlay (above everything).
	if state.open >= 0 && state.open < len(menus) {
		w.DeferDraw(newMenuDropdown(menus[state.open], gid, state), geometry.Point[geometry.Pixels]{}, 1000)
	}
}

// opaque raises a color's alpha floor.
func opaque(c gc.Hsla, a float32) gc.Hsla {
	if c.A < a {
		c.A = a
	}
	return c
}

// dropdownTextWidth measures one string in the menu font (0 when the text
// system is unavailable).
func dropdownTextWidth(w *gpui.Window, s string, fontSize geometry.Pixels) float32 {
	l := w.TextSystem().LayoutLine(s, fontSize, menuBarFontRuns(w, s))
	return l.Width.AsF32()
}

// dropdownFitText shortens s with an ellipsis until it fits maxW (returned
// unchanged when it already fits or the text system is absent).
func dropdownFitText(w *gpui.Window, s string, fontSize geometry.Pixels, maxW float32) string {
	if maxW <= 0 {
		return s
	}
	for s != "" && dropdownTextWidth(w, s, fontSize) > maxW {
		runes := []rune(s)
		if len(runes) <= 4 {
			return "…"
		}
		s = string(runes[:len(runes)-2]) + "…"
	}
	return s
}

// paintMenuLine paints a shaped line at the pen origin.
func paintMenuLine(w *gpui.Window, l *text.LineLayout, origin geometry.Point[geometry.Pixels], color gc.Hsla) {
	for _, run := range l.Runs {
		for _, g := range run.Glyphs {
			w.PaintGlyph(
				geometry.PointPixels(origin.X.AsF32()+g.Position.X.AsF32(), origin.Y.AsF32()+g.Position.Y.AsF32()),
				run.FontID, g.ID, l.FontSize, color)
		}
	}
}

// menuDropdown is the open dropdown panel (deferred draw above the tree).
type menuDropdown struct {
	menu  gpui.Menu
	gid   gpui.GlobalElementId
	state menuBarState
}

// newMenuDropdown builds the panel.
func newMenuDropdown(menu gpui.Menu, gid gpui.GlobalElementId, state menuBarState) *menuDropdown {
	return &menuDropdown{menu: menu, gid: gid, state: state}
}

// ElementID implements Element.
func (d *menuDropdown) ElementID() gpui.ElementId { return gpui.NameElementId("menu-dropdown") }

// RequestLayout implements Element.
func (d *menuDropdown) RequestLayout(w *gpui.Window) {
	w.RequestLayout(style.DefaultStyle(), nil)
}

// Prepaint implements Element. Deferred draws never prepaint in this
// port (the deferred list is rebuilt during paint), so all listener and
// hitbox registration happens in Paint like the editor popovers.
func (d *menuDropdown) Prepaint(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {}

// registerDropdown installs the dismiss hitbox and keyboard navigation
// (upstream key context "Menu").
func (d *menuDropdown) register(w *gpui.Window) {
	gid := d.gid
	// The dismiss hitbox covers the whole window below the bar: any press
	// outside the panel closes the menu.
	content := w.LogicalContentSize()
	dismiss := w.InsertHitbox(geometry.Bounds[geometry.Pixels]{
		Origin: geometry.Point[geometry.Pixels]{},
		Size:   content,
	}, gpui.HitboxBlockMouse)
	w.OnMouseEvent("down", platform.ButtonLeft, dismiss, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
		if phase != gpui.BubblePhase {
			return
		}
		gpui.UpdateElementStateAt(w, d.gid, func() menuBarState { return menuBarState{open: -1, hoverBar: -1, hoverItem: -1} }, func(s *menuBarState) {
			s.open = -1
			s.hoverItem = -1
		})
		w.Refresh()
	})
	// Keyboard: arrows / enter / escape (upstream key context "Menu").
	w.OnKeyEvent("down", "Menu", func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
		e, ok := ev.(platform.KeyDownEvent)
		if !ok || phase != gpui.BubblePhase {
			return
		}
		if e.Keystroke.Modifiers.Modified() {
			return
		}
		menus := w.AppMenu()
		// Listeners run between frames where the element path is empty:
		// read the bar's state through its captured id.
		var st menuBarState
		gpui.UpdateElementStateAt(w, gid, func() menuBarState { return menuBarState{open: -1, hoverBar: -1, hoverItem: -1} }, func(s *menuBarState) { st = *s })
		if st.open < 0 || st.open >= len(menus) {
			return
		}
		items := menus[st.open].Items
		nextEnabled := func(from int, forward bool) int {
			if forward {
				for i := from + 1; i < len(items); i++ {
					if !items[i].Separator && !items[i].Disabled {
						return i
					}
				}
				return -1
			}
			for i := from - 1; i >= 0; i-- {
				if !items[i].Separator && !items[i].Disabled {
					return i
				}
			}
			return -1
		}
		switch e.Keystroke.Key {
		case "escape":
			gpui.UpdateElementStateAt(w, gid, func() menuBarState { return menuBarState{open: -1, hoverBar: -1, hoverItem: -1} }, func(s *menuBarState) {
				s.open = -1
				s.hoverItem = -1
			})
			w.StopPropagation()
			w.Refresh()
		case "down":
			if next := nextEnabled(st.hoverItem, true); next >= 0 {
				gpui.UpdateElementStateAt(w, gid, func() menuBarState { return menuBarState{open: -1, hoverBar: -1, hoverItem: -1} }, func(s *menuBarState) { s.hoverItem = next })
				w.Refresh()
			}
			w.StopPropagation()
		case "up":
			if next := nextEnabled(st.hoverItem, false); next >= 0 {
				gpui.UpdateElementStateAt(w, gid, func() menuBarState { return menuBarState{open: -1, hoverBar: -1, hoverItem: -1} }, func(s *menuBarState) { s.hoverItem = next })
				w.Refresh()
			}
			w.StopPropagation()
		case "left", "right":
			if len(menus) < 2 {
				return
			}
			dir := 1
			if e.Keystroke.Key == "left" {
				dir = -1
			}
			next := (st.open + dir + len(menus)) % len(menus)
			gpui.UpdateElementStateAt(w, gid, func() menuBarState { return menuBarState{open: -1, hoverBar: -1, hoverItem: -1} }, func(s *menuBarState) {
				s.open = next
				s.hoverItem = -1
			})
			w.StopPropagation()
			w.Refresh()
		case "enter":
			if st.hoverItem >= 0 && st.hoverItem < len(items) {
				item := items[st.hoverItem]
				gpui.UpdateElementStateAt(w, gid, func() menuBarState { return menuBarState{open: -1, hoverBar: -1, hoverItem: -1} }, func(s *menuBarState) {
					s.open = -1
					s.hoverItem = -1
				})
				w.StopPropagation()
				w.Refresh()
				w.DispatchMenuItem(item)
			}
		}
	})
}

// Paint implements Element.
func (d *menuDropdown) Paint(w *gpui.Window) {
	// Deferred elements register listeners from paint (this port paints
	// them without a prepaint pass).
	d.register(w)
	colors := w.DefaultColors()
	rem := remOf(w)
	origin := d.state.origin

	rowH := rem * 1.7
	pad := rem * 0.35
	fontSize := style.DefaultStyle().Text.FontSize.ToPixels(w.RemSize())

	// Width: wide enough for the widest label + keystroke row (menus size
	// to their content), at least rem*13, capped by the window. Labels
	// that still cannot fit ellipsize instead of overflowing the panel.
	rowInner := rem*0.65*2 + rem*0.25
	width := rem * 13
	for _, it := range d.menu.Items {
		if it.Separator {
			continue
		}
		need := rowInner + dropdownTextWidth(w, it.Label, fontSize)
		if it.Keystroke != "" {
			need += dropdownTextWidth(w, it.Keystroke, fontSize)
		}
		if need > width {
			width = need
		}
	}
	content := w.LogicalContentSize()
	if width > content.Width.AsF32() {
		width = content.Width.AsF32()
	}
	var height float32
	for _, it := range d.menu.Items {
		if it.Separator {
			height += rem * 0.5
		} else {
			height += rowH
		}
	}
	panel := geometry.Bounds[geometry.Pixels]{
		Origin: geometry.PointPixels(origin.X.AsF32(), origin.Y.AsF32()+rem*0.2),
		Size:   geometry.SizePixels(width, height+pad*2),
	}
	if panel.Origin.X.AsF32()+width > content.Width.AsF32() {
		panel.Origin.X = geometry.Px(content.Width.AsF32() - width)
	}
	if panel.Origin.X.AsF32() < 0 {
		panel.Origin.X = 0
	}
	if panel.Origin.Y.AsF32()+panel.Size.Height.AsF32() > content.Height.AsF32() {
		panel.Origin.Y = geometry.Px(content.Height.AsF32() - panel.Size.Height.AsF32())
	}
	if panel.Origin.Y.AsF32() < 0 {
		panel.Origin.Y = 0
	}

	// Panel background + border.
	st := style.DefaultStyle()
	b := gc.SolidBackground(colors.Container.Hsla())
	st.Background = &b
	border := colors.Border.Hsla()
	border.A = 0.7
	st.BorderColor = &border
	st.BorderWidths = geometry.EdgesAbsolute{
		Top: geometry.PixelsLength(geometry.Px(1)), Right: geometry.PixelsLength(geometry.Px(1)),
		Bottom: geometry.PixelsLength(geometry.Px(1)), Left: geometry.PixelsLength(geometry.Px(1)),
	}
	st.CornerRadii = geometry.CornersAbsolute{
		TopLeft: geometry.PixelsLength(geometry.Px(rem * 0.35)), TopRight: geometry.PixelsLength(geometry.Px(rem * 0.35)),
		BottomRight: geometry.PixelsLength(geometry.Px(rem * 0.35)), BottomLeft: geometry.PixelsLength(geometry.Px(rem * 0.35)),
	}
	w.PaintQuad(st, panel)
	w.InsertHitbox(panel, gpui.HitboxBlockMouse)

	y := panel.Origin.Y.AsF32() + pad
	for i, it := range d.menu.Items {
		if it.Separator {
			sep := geometry.Bounds[geometry.Pixels]{
				Origin: geometry.PointPixels(panel.Origin.X.AsF32()+rem*0.6, y+rem*0.15),
				Size:   geometry.SizePixels(width-rem*1.2, 1),
			}
			sst := style.DefaultStyle()
			sepC := colors.Separator.Hsla()
			sepC.A = 0.5
			sb := gc.SolidBackground(sepC)
			sst.Background = &sb
			w.PaintQuad(sst, sep)
			y += rem * 0.5
			continue
		}
		row := geometry.Bounds[geometry.Pixels]{
			Origin: geometry.PointPixels(panel.Origin.X.AsF32()+rem*0.25, y),
			Size:   geometry.SizePixels(width-rem*0.5, rowH),
		}
		if d.state.hoverItem == i {
			hst := style.DefaultStyle()
			hl := colors.Selected.Hsla()
			hl.A = 0.75
			hb := gc.SolidBackground(hl)
			hst.Background = &hb
			w.PaintQuad(hst, row)
		}
		fg := colors.Text.Hsla()
		if it.Disabled {
			fg = colors.Disabled.Hsla()
		}
		// The label shrinks to the room left by the keystroke hint
		// (upstream ellipsize_middle when a menu is narrower than a
		// row's content).
		label := it.Label
		maxLabelW := row.Size.Width.AsF32() - rowInner
		if it.Keystroke != "" {
			maxLabelW -= dropdownTextWidth(w, it.Keystroke, fontSize)
		}
		label = dropdownFitText(w, label, fontSize, maxLabelW)
		lbl := w.TextSystem().LayoutLine(label, fontSize, menuBarFontRuns(w, label))
		paintMenuLine(w, lbl, geometry.PointPixels(
			row.Origin.X.AsF32()+rem*0.65,
			row.Origin.Y.AsF32()+(rowH-(lbl.Ascent.AsF32()+lbl.Descent.AsF32()))/2+lbl.Ascent.AsF32()), fg)
		if it.Keystroke != "" {
			ks := w.TextSystem().LayoutLine(it.Keystroke, fontSize, menuBarFontRuns(w, it.Keystroke))
			dim := fg
			dim.A = 0.55
			paintMenuLine(w, ks, geometry.PointPixels(
				row.Origin.X.AsF32()+width-rem*0.65-ks.Width.AsF32()-rem*0.25,
				row.Origin.Y.AsF32()+(rowH-(ks.Ascent.AsF32()+ks.Descent.AsF32()))/2+ks.Ascent.AsF32()), dim)
		}
		item := it
		hb := w.InsertHitbox(row, gpui.HitboxNormal)
		w.OnMouseEvent("move", platform.ButtonLeft, hb, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			gpui.UpdateElementStateAt(w, d.gid, func() menuBarState { return menuBarState{open: -1, hoverBar: -1, hoverItem: -1} }, func(s *menuBarState) { s.hoverItem = i })
			w.Refresh()
		})
		w.OnMouseEvent("up", platform.ButtonLeft, hb, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase || item.Disabled {
				return
			}
			gpui.UpdateElementStateAt(w, d.gid, func() menuBarState { return menuBarState{open: -1, hoverBar: -1, hoverItem: -1} }, func(s *menuBarState) {
				s.open = -1
				s.hoverItem = -1
			})
			w.Refresh()
			w.DispatchMenuItem(item)
		})
		y += rowH
	}
}
