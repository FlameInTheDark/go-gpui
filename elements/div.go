// Package elements provides go-gpui's concrete elements: Div (with the
// full interactivity surface), text, canvas, deferred/anchored overlays,
// animations, virtualized lists, images and SVGs.
package elements

import (
	"fmt"
	"time"

	"github.com/FlameInTheDark/go-gpui/app"
	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gestures"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/style"
)

// DragThreshold is the movement that distinguishes drag from click.
const DragThreshold = 2.0

// InteractiveState is the persistent per-element interaction state
// (upstream InteractiveElementState).
type interactiveState struct {
	focusHandle      gpui.FocusHandle
	clicked          bool
	hovered          bool
	hovering         bool
	pendingMouseDown *platform.MouseDownEvent
	scrollOffset     geometry.Point[geometry.Pixels]
	// ongoingScroll locks touch pans to their dominant axis while
	// restrict_scroll_to_axis is set (upstream div scroll state).
	ongoingScroll gestures.OngoingScroll

	// Tooltip state machine (upstream element_state.active_tooltip).
	tooltipPhase       int8 // tooltipNone / tooltipWaiting / tooltipVisible
	tooltipShowAtMS    int64
	tooltipPos         geometry.Point[geometry.Pixels]
	tooltipTimerActive bool
}

// Tooltip phases (upstream ActiveTooltip: None / WaitingForShow / Visible;
// the port collapses WaitingForHide into a direct hide).
const (
	tooltipNone    int8 = 0
	tooltipWaiting int8 = 1
	tooltipVisible int8 = 2
)

// DefaultTooltipShowDelay is the hover delay before a tooltip appears
// (upstream DEFAULT_TOOLTIP_SHOW_DELAY).
const DefaultTooltipShowDelay = 500

// Div is the container element with interactivity.
type Div struct {
	st       *style.StyleRefinement
	children []gpui.Element
	id       gpui.ElementId

	// conditional styles
	hoverStyle  *style.StyleRefinement
	activeStyle *style.StyleRefinement
	focusStyle  *style.StyleRefinement

	// listeners
	onClick     func(ev gpui.ClickEvent, w *gpui.Window)
	onMouseDown func(ev platform.MouseDownEvent, w *gpui.Window)
	// onMouseDownCapture is the capture-phase press handler: it runs
	// before every bubble listener (innermost first), so overlay controls
	// (dock resize grips, scrollbar tracks) layered over interactive
	// content receive the press even though the content underneath is
	// deeper in the element tree.
	onMouseDownCapture func(ev platform.MouseDownEvent, w *gpui.Window)
	onRightDown        func(ev platform.MouseDownEvent, w *gpui.Window)
	onMouseUp          func(ev platform.MouseUpEvent, w *gpui.Window)
	onMouseMove        func(ev platform.MouseMoveEvent, w *gpui.Window)
	onScroll           func(ev platform.ScrollEvent, w *gpui.Window)
	onKeyDown          func(ev platform.KeyDownEvent, w *gpui.Window)
	onDrag             func(ev gpui.DragEvent, w *gpui.Window)
	onHover            func(hovered bool, w *gpui.Window)
	// onMouseDownOut fires during the capture phase when a press lands
	// outside this element (upstream on_mouse_down_out).
	onMouseDownOut func(ev platform.MouseDownEvent, w *gpui.Window)
	// Gesture listeners (upstream gestures / interactive.rs).
	onPinch       func(ev platform.PinchEvent, w *gpui.Window)
	onLongPress   func(ev platform.LongPressEvent, w *gpui.Window)
	onTouchDrag   func(ev platform.TouchDragEvent, w *gpui.Window)
	onBackGesture func(ev platform.BackGestureEvent, w *gpui.Window)
	// onFileDrop receives external drag-and-drop file events (upstream
	// on_drop for FileDropEvent / platform FileDropEvent).
	onFileDrop func(ev platform.FileDropEvent, w *gpui.Window)

	// Drag and drop (upstream drag_listener / drop_listeners).
	dragStart     func(w *gpui.Window) *gpui.DragState
	onDragStarted func(d *gpui.DragState, w *gpui.Window)
	onDrop        func(payload any, w *gpui.Window)
	dropType      string
	dragOverStyle *style.StyleRefinement

	// Tooltip (upstream tooltip_builder + tooltip_show_delay).
	tooltipText        string
	tooltipBuilder     func(w *gpui.Window) gpui.Element
	tooltipShowDelayMS int64

	// interactivity flags
	focusable bool
	occluding bool
	// focusHandle is the explicitly tracked focus handle (TrackFocus);
	// zero means one is minted from element state.
	focusHandle gpui.FocusHandle
	// scrollHandle mirrors this div's scroll offset externally (TrackScroll;
	// upstream ScrollHandle::new + track_scroll).
	scrollHandle *ScrollHandle
	// a11y records this div in the accessibility tree (upstream
	// accesskit node for interactive elements).
	a11yRole   gpui.AccessibilityRole
	a11yLabel  string
	a11yValue  string
	keyContext string
	group      string

	// currentHitbox carries prepaint state to paint.
	currentHitbox gpui.Hitbox
	// scrollableHitbox marks the frame's hitbox as belonging to a scroll
	// container (upstream scroll_offset.is_some()).
	scrollableHitbox gpui.Hitbox
}

// NewDiv creates a div.
func NewDiv() *Div {
	return &Div{st: style.NewStyleRefinement()}
}

// StyleRef returns the refinement (for generated forwarders).
func (d *Div) StyleRef() *style.StyleRefinement { return d.st }

// ID sets the element id (enables stateful handlers).
func (d *Div) ID(id gpui.ElementId) *Div {
	d.id = id
	return d
}

// Child appends a child element.
func (d *Div) Child(e gpui.Element) *Div {
	d.children = append(d.children, e)
	return d
}

// Children appends children.
func (d *Div) Children(es ...gpui.Element) *Div {
	d.children = append(d.children, es...)
	return d
}

// Hover registers a hover style.
func (d *Div) Hover(f func(s *style.StyleRefinement)) *Div {
	r := style.NewStyleRefinement()
	f(r)
	d.hoverStyle = r
	return d
}

// Active registers an active (pressed) style.
func (d *Div) Active(f func(s *style.StyleRefinement)) *Div {
	r := style.NewStyleRefinement()
	f(r)
	d.activeStyle = r
	return d
}

// FocusStyle registers a focused style.
func (d *Div) FocusStyle(f func(s *style.StyleRefinement)) *Div {
	r := style.NewStyleRefinement()
	f(r)
	d.focusStyle = r
	return d
}

// OnClick registers a click handler (requires an ID).
func (d *Div) OnClick(f func(ev gpui.ClickEvent, w *gpui.Window)) *Div {
	d.onClick = f
	return d
}

// OnMouseDown registers a mouse-down handler (left button).
func (d *Div) OnMouseDown(f func(ev platform.MouseDownEvent, w *gpui.Window)) *Div {
	d.onMouseDown = f
	return d
}

// OnMouseDownCapture registers a capture-phase press handler (see
// onMouseDownCapture). Capture listeners run before bubble ones, so the
// overlay claims the gesture before the interactive content below it.
func (d *Div) OnMouseDownCapture(f func(ev platform.MouseDownEvent, w *gpui.Window)) *Div {
	d.onMouseDownCapture = f
	return d
}

// OnRightDown registers a right-button (secondary) press handler, the
// conventional trigger for context menus. The handler receives the press
// when it happens inside this element (upstream on_mouse_down with
// MouseButton::Right / context-menu behavior).
func (d *Div) OnRightDown(f func(ev platform.MouseDownEvent, w *gpui.Window)) *Div {
	d.onRightDown = f
	return d
}

// OnMouseUp registers a mouse-up handler.
func (d *Div) OnMouseUp(f func(ev platform.MouseUpEvent, w *gpui.Window)) *Div {
	d.onMouseUp = f
	return d
}

// OnMouseMove registers a move handler.
func (d *Div) OnMouseMove(f func(ev platform.MouseMoveEvent, w *gpui.Window)) *Div {
	d.onMouseMove = f
	return d
}

// OnScroll registers a scroll handler (enables overflow scroll defaults).
func (d *Div) OnScroll(f func(ev platform.ScrollEvent, w *gpui.Window)) *Div {
	d.onScroll = f
	return d
}

// OnPinch registers a trackpad-pinch handler (upstream
// StatefulInteractiveElement::on_pinch).
func (d *Div) OnPinch(f func(ev platform.PinchEvent, w *gpui.Window)) *Div {
	d.onPinch = f
	return d
}

// OnLongPress registers a long-press handler for touch input (upstream
// StatefulInteractiveElement::on_long_press). Claiming the Started phase
// (PreventDefault) routes the whole gesture to this element.
func (d *Div) OnLongPress(f func(ev platform.LongPressEvent, w *gpui.Window)) *Div {
	d.onLongPress = f
	return d
}

// OnTouchDrag registers a direct touch-drag handler (upstream
// StatefulInteractiveElement::on_touch_drag). Claiming the Started phase
// (PreventDefault) routes the whole gesture to this element.
func (d *Div) OnTouchDrag(f func(ev platform.TouchDragEvent, w *gpui.Window)) *Div {
	d.onTouchDrag = f
	return d
}

// OnBackGesture registers a back-navigation gesture handler, such as the
// iOS swipe from the leading screen edge (upstream
// StatefulInteractiveElement::on_back_gesture). The innermost element
// under the gesture's start position claims the whole gesture; every phase
// is delivered at the start position.
func (d *Div) OnBackGesture(f func(ev platform.BackGestureEvent, w *gpui.Window)) *Div {
	d.onBackGesture = f
	return d
}

// OnFileDrop registers an external drag-and-drop handler (upstream
// file-drop dispatch): the platform reports hovered file paths through
// Entered/Pending phases and the final list on Submit.
func (d *Div) OnFileDrop(f func(ev platform.FileDropEvent, w *gpui.Window)) *Div {
	d.onFileDrop = f
	return d
}

// OnKeyDown registers a key handler (requires focus).
func (d *Div) OnKeyDown(f func(ev platform.KeyDownEvent, w *gpui.Window)) *Div {
	d.onKeyDown = f
	return d
}

// OnDrag registers a drag handler.
func (d *Div) OnDrag(f func(ev gpui.DragEvent, w *gpui.Window)) *Div {
	d.onDrag = f
	return d
}

// OnHover registers a hover start/end callback (upstream
// StatefulInteractiveElement::on_hover): the bool is true when the hover
// begins and false when it ends. Transitions caused by layout changes under
// a stationary mouse also invoke the callback. Hover is suppressed while a
// drag is in flight and after keyboard input (upstream hover suppression).
func (d *Div) OnHover(f func(hovered bool, w *gpui.Window)) *Div {
	d.onHover = f
	return d
}

// OnMouseDownOut registers a handler that fires during the capture phase
// when a mouse-down of any button lands OUTSIDE this element (upstream
// InteractiveElement::on_mouse_down_out). Typical use: dismiss popovers.
func (d *Div) OnMouseDownOut(f func(ev platform.MouseDownEvent, w *gpui.Window)) *Div {
	d.onMouseDownOut = f
	return d
}

// Drag registers this element as a drag-and-drop source (upstream
// StatefulInteractiveElement::on_drag): once a left press moves beyond
// DragThreshold, start builds the DragState (typed Payload plus an optional
// drag View). The window keeps the state while the gesture lasts and paints
// the view under the cursor; drop targets receive the payload on release.
func (d *Div) Drag(start func(w *gpui.Window) *gpui.DragState) *Div {
	d.dragStart = start
	return d
}

// OnDragStarted runs when a drag actually leaves this element (the press
// moved past the threshold); d carries the payload and drag view.
func (d *Div) OnDragStarted(f func(d *gpui.DragState, w *gpui.Window)) *Div {
	d.onDragStarted = f
	return d
}

// OnDragStart registers a payload-only drag source (upstream on_drag with a
// bare value): once a left press moves past DragThreshold, f runs and its
// return value becomes the drag Payload. The feedback view under the cursor
// is a small chip showing the payload's string form (upstream renders the
// caller's view; this port substitutes the chip when no View is given).
// Overwrites an earlier Drag builder; the gesture plumbing (threshold,
// OnDrop, DragOverStyle) is shared.
func (d *Div) OnDragStart(f func(w *gpui.Window) any) *Div {
	d.dragStart = func(w *gpui.Window) *gpui.DragState {
		payload := f(w)
		st := &gpui.DragState{Payload: payload}
		if payload != nil {
			st.Content = fmt.Sprintf("%v", payload)
		}
		return st
	}
	return d
}

// OnDrop registers this element as a drop target accepting any payload
// (upstream on_drop with a TypeId match against the active drag). The
// handler runs on mouse-up while a compatible drag hovers the element; the
// drag is consumed before the call.
func (d *Div) OnDrop(f func(payload any, w *gpui.Window)) *Div {
	d.onDrop = f
	return d
}

// OnDropType is OnDrop restricted to payloads whose Type() matches typ
// (upstream drop listeners keyed by the dragged value's TypeId).
func (d *Div) OnDropType(typ string, f func(payload any, w *gpui.Window)) *Div {
	d.onDrop = f
	d.dropType = typ
	return d
}

// DragOverStyle registers a style applied while a compatible drag hovers
// this element (upstream drag_over styles): the check is the element's
// hit-test containing the cursor plus a matching drag payload type.
func (d *Div) DragOverStyle(f func(s *style.StyleRefinement)) *Div {
	r := style.NewStyleRefinement()
	f(r)
	d.dragOverStyle = r
	return d
}

// Tooltip shows a default text tooltip near the cursor after the show delay
// (upstream StatefulInteractiveElement::tooltip; the view is
// elements.NewTooltip). The tooltip hides when the mouse leaves the element.
func (d *Div) Tooltip(text string) *Div {
	d.tooltipText = text
	return d
}

// TooltipBuilder builds a custom tooltip view when shown (upstream
// tooltip(fn() AnyView)); it takes precedence over Tooltip.
func (d *Div) TooltipBuilder(f func(w *gpui.Window) gpui.Element) *Div {
	d.tooltipBuilder = f
	return d
}

// TooltipShowDelay overrides the hover delay before the tooltip shows, in
// milliseconds (upstream tooltip_show_delay; default DefaultTooltipShowDelay).
func (d *Div) TooltipShowDelay(ms int64) *Div {
	d.tooltipShowDelayMS = ms
	return d
}

// Cursor sets the mouse cursor shown while the pointer hovers this element
// (upstream Styled::mouse_cursor / div().cursor_style). The window resolves
// the hovered request each frame and applies it through the platform.
func (d *Div) Cursor(c style.MouseCursor) *Div {
	cs := c.ToCursorStyle()
	d.st.MouseCursor = &cs
	return d
}

// TrackFocus marks the div focusable with the given handle (upstream
// Stateful::track_focus): the handle is shared with the application, so
// w.Focus(handle) focuses this element and its key listeners fire.
func (d *Div) TrackFocus(handle gpui.FocusHandle) *Div {
	d.focusable = true
	if handle.IsValid() {
		d.focusHandle = handle
	}
	return d
}

// Occlude blocks mouse events for elements below.
func (d *Div) Occlude() *Div {
	d.occluding = true
	return d
}

// KeyContext sets the key dispatch context.
func (d *Div) KeyContext(ctx string) *Div {
	d.keyContext = ctx
	return d
}

// TrackScroll attaches a ScrollHandle to this scrollable div (upstream
// Interactivity::track_scroll): every prepaint mirrors the live offset,
// content size and viewport into the handle, and the handle's pending
// requests (SetTop / Autoscroll) apply before clamping.
func (d *Div) TrackScroll(h *ScrollHandle) *Div {
	d.scrollHandle = h
	return d
}

// A11y records this div in the accessibility tree with an explicit role
// and label (upstream accesskit node). Interactive divs without an
// explicit role default to button.
func (d *Div) A11y(role gpui.AccessibilityRole, label string) *Div {
	d.a11yRole = role
	d.a11yLabel = label
	return d
}

// A11yValue sets the accessibility value (text inputs).
func (d *Div) A11yValue(value string) *Div {
	d.a11yValue = value
	return d
}

// openA11y registers the div's accessibility node during paint and
// returns the closer; children painted in between nest under the node
// (upstream accesskit parenting).
func (d *Div) openA11y(w *gpui.Window, state *interactiveState) func() {
	role := d.a11yRole
	label := d.a11yLabel
	if role == gpui.AccessibilityUnknown {
		// Interactive divs default to button (upstream accesskit
		// role inference).
		if d.onClick == nil {
			return func() {} // nothing to record
		}
		role = gpui.AccessibilityButton
		if label == "" {
			label = d.tooltipText
		}
	}
	idx := len(w.NextA11yNodes())
	node := gpui.AccessibilityNode{
		Role:   role,
		Label:  label,
		Value:  d.a11yValue,
		Bounds: w.ElementBounds(d),
	}
	if state.focusHandle.IsValid() && w.Focused() == state.focusHandle {
		node.Focused = true
	}
	w.AppendA11yNode(node)
	w.PushA11yParent(idx)
	return func() {
		w.PopA11yParent(idx)
	}
}

// Group assigns an interaction group.
func (d *Div) Group(name string) *Div {
	d.group = name
	return d
}

// ElementID implements Element.
func (d *Div) ElementID() gpui.ElementId { return d.id }

// RequestLayout implements Element.
func (d *Div) RequestLayout(w *gpui.Window) {
	st := style.DefaultStyle()
	st.Refine(d.st)
	// Push this div's text refinement (font size, white-space, family…)
	// so descendant Text elements shape with it — upstream div text
	// styles apply to the whole subtree.
	if d.st != nil && d.st.Text != nil {
		w.WithTextStyle(d.st.Text, func() {
			w.RequestLayout(st, d.children)
		})
		return
	}
	w.RequestLayout(st, d.children)
}

// computeStyle folds the refinement chain (base -> hover -> focus -> active).
func (d *Div) computeStyle(w *gpui.Window, state *interactiveState, hitbox *gpui.Hitbox) style.Style {
	s := style.DefaultStyle()
	s.Refine(d.st)
	if hitbox != nil {
		if d.hoverStyle != nil && hitbox.IsHovered(w) {
			s.Refine(d.hoverStyle)
		}
	}
	if d.focusStyle != nil && state.focusHandle.IsValid() && state.focusHandle == w.Focused() {
		s.Refine(d.focusStyle)
	}
	if d.activeStyle != nil && state.clicked {
		s.Refine(d.activeStyle)
	}
	// Drag-over: applied while a compatible drag hovers the element (upstream
	// drag_over styles resolved during paint).
	if d.dragOverStyle != nil && hitbox != nil && hitbox.IsHovered(w) &&
		d.dropCompatible(w.ActiveDrag()) {
		s.Refine(d.dragOverStyle)
	}
	return s
}

// dropCompatible reports whether the active drag may target this element:
// the payload type matches dropType when one was registered (upstream
// TypeId equality between the drag and the drop listener).
func (d *Div) dropCompatible(drag *gpui.DragState) bool {
	return drag != nil && (d.dropType == "" || drag.Type() == d.dropType)
}

// Prepaint implements Element.
func (d *Div) Prepaint(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {
	d.prepaintInner(w, bounds)
}

func (d *Div) prepaintInner(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {
	var scrollOffset geometry.Point[geometry.Pixels]
	gpui.UseElementState(w, func() interactiveState {
		return interactiveState{}
	}, func(state *interactiveState) {
		if d.focusable {
			if d.focusHandle.IsValid() {
				state.focusHandle = d.focusHandle
			} else if !state.focusHandle.IsValid() {
				state.focusHandle = gpui.NewFocusHandle()
			}
		}

		// Children are laid out by the window walk (Div.RequestLayout built
		// the layout tree); this port stores no content size here.

		// Insert hitbox when interactive (upstream should_insert_hitbox:
		// scroll containers get a hitbox even without listeners). The
		// hitbox carries the element's stable id key so pointer captures
		// survive structural re-renders (see Hitbox.Key).
		behavior := gpui.HitboxNormal
		if d.occluding {
			behavior = gpui.HitboxBlockMouse
		}
		if d.needsHitbox() || d.isScrollContainer() {
			d.currentHitbox = w.InsertKeyedHitbox(bounds, behavior, d.id.HitboxKey())
		}

		// Clamp the scroll offset to the content extent (upstream
		// clamps scroll_offset to [-scroll_max, 0] in prepaint).
		if d.isScrollContainer() {
			scrollSize := w.ElementScrollSize(d)
			maxX := scrollSize.Width - bounds.Size.Width
			maxY := scrollSize.Height - bounds.Size.Height
			// ScrollHandle requests apply before clamping (upstream
			// request_scroll_to / autoscroll timing).
			if d.scrollHandle != nil {
				pending, follow, pendingLeft := d.scrollHandle.peek()
				if follow {
					state.scrollOffset.Y = -maxY
				} else if pending != nil {
					state.scrollOffset.Y = geometry.Px(-pending.AsF32())
				}
				if pendingLeft != nil {
					state.scrollOffset.X = geometry.Px(-pendingLeft.AsF32())
				}
			}
			if state.scrollOffset.Y.AsF32() < -maxY.AsF32() {
				state.scrollOffset.Y = -maxY
			}
			if state.scrollOffset.Y.AsF32() > 0 {
				state.scrollOffset.Y = 0
			}
			if state.scrollOffset.X.AsF32() < -maxX.AsF32() {
				state.scrollOffset.X = -maxX
			}
			if state.scrollOffset.X.AsF32() > 0 {
				state.scrollOffset.X = 0
			}
			// Mirror the final offset (post-request, post-clamp).
			if d.scrollHandle != nil {
				d.scrollHandle.sync(state.scrollOffset, scrollSize, bounds.Size)
			}
			d.scrollableHitbox = d.currentHitbox
		}
		scrollOffset = state.scrollOffset
	})
	// Prepainted children get their bounds from the walk in Window.Draw,
	// translated by this element's scroll offset (upstream
	// with_element_offset(scroll_offset)) and scoped by this element's key
	// context (upstream with_key_context). Overflow containers scope their
	// children's prepaint with the content mask too: hitboxes inherit the
	// mask at insert time (upstream Hitbox::content_mask), so children
	// clipped by a scroll container must not catch input outside the
	// visible viewport.
	w.WithElementOffset(scrollOffset, func() {
		w.WithKeyContext(d.keyContext, func() {
			d.withChildTextStyle(w, func() {
				childPrepaint := func() {
					for _, child := range d.children {
						if node, ok := w.NodeOf(child); ok {
							w.PrepaintElement(child, node)
						}
					}
				}
				st := style.DefaultStyle()
				st.Refine(d.st)
				if st.Overflow[0] != style.OverflowVisible || st.Overflow[1] != style.OverflowVisible {
					rem := w.RemSize()
					padding := st.Padding.Resolve(geometry.AbsoluteLength{}, geometry.AbsoluteLength{}, rem)
					childBounds := bounds
					childBounds.Origin.X += padding.Left
					childBounds.Origin.Y += padding.Top
					childBounds.Size.Width -= padding.Left + padding.Right
					childBounds.Size.Height -= padding.Top + padding.Bottom
					w.WithContentMask(geometry.IntersectBounds(w.ContentMask(), childBounds), childPrepaint)
					return
				}
				childPrepaint()
			})
		})
	})
}

// withChildTextStyle runs f with this div's text refinement active so
// descendant Text elements shape consistently across layout, prepaint and
// paint (upstream Interactivity::text_style refines the style for children).
func (d *Div) withChildTextStyle(w *gpui.Window, f func()) {
	if d.st != nil && d.st.Text != nil {
		w.WithTextStyle(d.st.Text, f)
		return
	}
	f()
}

func (d *Div) needsHitbox() bool {
	return d.onClick != nil || d.onMouseDown != nil || d.onMouseDownCapture != nil || d.onRightDown != nil ||
		d.onMouseUp != nil || d.onMouseMove != nil || d.onScroll != nil || d.onKeyDown != nil ||
		d.onDrag != nil || d.hoverStyle != nil || d.activeStyle != nil || d.occluding ||
		d.onHover != nil || d.onMouseDownOut != nil || d.dragStart != nil ||
		d.onDrop != nil || d.dragOverStyle != nil ||
		d.tooltipText != "" || d.tooltipBuilder != nil ||
		(d.st != nil && d.st.MouseCursor != nil)
}

// isScrollContainer reports whether the element's own style scrolls (upstream
// base_style.overflow == Scroll on either axis).
func (d *Div) isScrollContainer() bool {
	if d.st == nil || d.st.Overflow == nil {
		return false
	}
	isScroll := func(o *style.Overflow) bool {
		return o != nil && *o == style.OverflowScroll
	}
	return isScroll(d.st.Overflow.X) || isScroll(d.st.Overflow.Y)
}

// Paint implements Element.
func (d *Div) Paint(w *gpui.Window) {
	d.paintInner(w)
}

func (d *Div) paintInner(w *gpui.Window) {
	gpui.UseElementState(w, func() interactiveState {
		return interactiveState{}
	}, func(state *interactiveState) {
		hitbox := d.currentHitbox
		st := d.computeStyle(w, state, &hitbox)

		if st.Visibility == style.VisibilityHidden {
			return
		}

		rem := w.RemSize()

		// Accessibility node (upstream accesskit recording during
		// paint): opens BEFORE the children paint so child nodes
		// nest under it, closes after.
		closeA11y := d.openA11y(w, state)
		defer closeA11y()

		// Box paint (shadows -> background -> children -> inset shadows).
		bounds := w.ElementBounds(d)
		if st.Display != style.DisplayNone {
			w.PaintStyle(st, bounds, func() {
				// Padding + clip for children.
				padding := st.Padding.Resolve(geometry.AbsoluteLength{}, geometry.AbsoluteLength{}, rem)
				childBounds := bounds
				childBounds.Origin.X += padding.Left
				childBounds.Origin.Y += padding.Top
				childBounds.Size.Width -= padding.Left + padding.Right
				childBounds.Size.Height -= padding.Top + padding.Bottom
				mask := w.ContentMask()
				if st.Overflow[0] != style.OverflowVisible || st.Overflow[1] != style.OverflowVisible {
					mask = geometry.IntersectBounds(mask, childBounds)
				}
				w.WithContentMask(mask, func() {
					// Scroll offset from element state.
					w.WithElementOffset(geometry.Point[geometry.Pixels]{
						X: state.scrollOffset.X,
						Y: state.scrollOffset.Y,
					}, func() {
						d.paintChildren(w)
					})
				})
			})
		}

		// Listeners + tooltip state evaluation (paint phase).
		if hitbox.ID != 0 {
			d.paintTooltipState(w, state, hitbox)
			d.paintListeners(w, state, hitbox, st)
		}
	})
}

// NodeOf returns the layout node assigned to an element this frame.
func elementNodeOf(e gpui.Element) {}

func (d *Div) paintChildren(w *gpui.Window) {
	d.withChildTextStyle(w, func() {
		for _, child := range d.children {
			w.PaintElement(child)
		}
	})
}

func (d *Div) paintListeners(w *gpui.Window, state *interactiveState, hitbox gpui.Hitbox, st style.Style) {
	// Listeners run between frames where the element path is empty, so
	// every state mutation persists through the element's captured id
	// (upstream holds an Rc into the state instead). This keeps clicks and
	// drags working across redraws triggered by other input listeners.
	gid := w.CurrentElementID()
	persist := func(use func(s *interactiveState)) {
		gpui.UpdateElementStateAt(w, gid, func() interactiveState { return interactiveState{} }, use)
	}
	// Cursor style (upstream paint_mouse_listeners: hovered elements with a
	// mouse_cursor register a frame cursor request; the window applies the
	// topmost hovered request after the frame).
	if st.MouseCursor != nil && hitbox.IsHovered(w) {
		w.SetCursorStyle(*st.MouseCursor, hitbox)
	}
	// Hover transitions (upstream paint_hover_listener: move + exit listeners
	// update the persisted hover state and fire the callback on change).
	if d.onHover != nil {
		updateHover := func(isHovered bool) {
			changed := false
			persist(func(s *interactiveState) {
				if s.hovered != isHovered {
					s.hovered = isHovered
					changed = true
				}
			})
			if changed {
				d.onHover(isHovered, w)
			}
		}
		// Layout changes under a stationary mouse also transition (upstream
		// defers the check to after the frame; this port evaluates it at
		// paint, firing once per actual transition because the state
		// persists per element id).
		updateHover(hitbox.IsHovered(w) && !w.HasActiveDrag())
		w.OnMouseEvent("move", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase == gpui.BubblePhase {
				updateHover(hitbox.IsHovered(w) && !w.HasActiveDrag())
			}
		})
		// The pointer can leave without a final move; "exit" also matches
		// moves that left the hitbox (upstream clears on MouseExited).
		w.OnMouseEvent("exit", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase == gpui.BubblePhase {
				updateHover(false)
			}
		})
	}
	// Mouse down outside (upstream on_mouse_down_out: capture-phase handler
	// for presses of any button landing outside this element's hitbox).
	if d.onMouseDownOut != nil {
		w.OnMouseEvent("down-out", platform.ButtonLeft, hitbox, true, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase == gpui.CapturePhase {
				d.onMouseDownOut(ev.(platform.MouseDownEvent), w)
			}
		})
	}
	// Drag source: record the pending press and start the gesture once the
	// pointer moves past DragThreshold (upstream drag_listener handling in
	// paint_mouse_listeners).
	if d.dragStart != nil {
		w.OnMouseEvent("down", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			e := ev.(platform.MouseDownEvent)
			persist(func(s *interactiveState) {
				s.pendingMouseDown = &e
				s.clicked = false
			})
		})
		w.OnMouseEvent("move", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			e := ev.(platform.MouseMoveEvent)
			var drag *gpui.DragState
			var grab geometry.Point[geometry.Pixels]
			var origin geometry.Point[geometry.Pixels]
			persist(func(s *interactiveState) {
				if s.pendingMouseDown == nil || w.HasActiveDrag() {
					return
				}
				dx := e.Position.X.AsF32() - s.pendingMouseDown.Position.X.AsF32()
				dy := e.Position.Y.AsF32() - s.pendingMouseDown.Position.Y.AsF32()
				if dx*dx+dy*dy > DragThreshold*DragThreshold {
					origin = s.pendingMouseDown.Position
					grab = e.Position
					drag = d.dragStart(w)
					s.pendingMouseDown = nil
					s.clicked = false
				}
			})
			if drag != nil {
				drag.Origin = origin
				if drag.View == nil && drag.Content != "" {
					drag.View = d.defaultDragVisual(w, drag.Content)
				}
				if drag.View != nil {
					// The pointer offset inside the drag view (upstream
					// cursor_offset: event.position - hitbox.origin).
					b := w.ElementBounds(d)
					drag.CursorOffset = geometry.PointPixels(
						grab.X.AsF32()-b.Origin.X.AsF32(),
						grab.Y.AsF32()-b.Origin.Y.AsF32())
				}
				w.StartDrag(drag)
				if d.onDragStarted != nil {
					d.onDragStarted(drag, w)
				}
				w.StopPropagation()
			}
		})
	}
	// Drop target: deliver the payload on release over a compatible hover
	// (upstream drop_listeners run on MouseUp bubble when the hitbox is
	// hovered and the drag's value type matches).
	if d.onDrop != nil {
		w.OnMouseEvent("up", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			drag := w.ActiveDrag()
			if drag == nil || !d.dropCompatible(drag) || !hitbox.IsHovered(w) {
				return
			}
			w.TakeActiveDrag()
			d.onDrop(drag.Payload, w)
			w.StopPropagation()
		})
	}
	// Tooltip state (upstream register_tooltip_mouse_handlers: show after the
	// delay, cancel when the pointer leaves, clear on press/scroll).
	if d.tooltipText != "" || d.tooltipBuilder != nil {
		delay := d.tooltipShowDelayMS
		if delay <= 0 {
			delay = DefaultTooltipShowDelay
		}
		// Arm the wake-up timer so the tooltip appears while the mouse is
		// stationary. Hosts that drive frames themselves and install no
		// dispatcher (headless tests) get no timer goroutine; they draw
		// after the delay manually.
		if state.tooltipPhase == tooltipWaiting && !state.tooltipTimerActive && w.CanScheduleTimers() {
			state.tooltipTimerActive = true
			d := delay
			w.ScheduleAfter(time.Duration(d)*time.Millisecond, func(win *gpui.Window) {
				win.ScheduleFrame()
			})
		}
		w.OnMouseEvent("move", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			e := ev.(platform.MouseMoveEvent)
			// listenerMatches gated this listener on the hitbox being hovered.
			persist(func(s *interactiveState) {
				if s.tooltipPhase == tooltipNone {
					s.tooltipPhase = tooltipWaiting
					s.tooltipShowAtMS = w.NowMilliseconds() + delay
					s.tooltipPos = e.Position
					w.Refresh() // paint once to arm the stationary-pointer timer
				}
			})
		})
		// Leaving the element hides the tooltip with the small hysteresis the
		// hit-test provides (upstream hides when the hitbox stops hovering).
		w.OnMouseEvent("exit", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase == gpui.BubblePhase {
				persist(func(s *interactiveState) {
					if s.tooltipPhase != tooltipNone {
						s.tooltipPhase = tooltipNone
						w.Refresh()
					}
				})
			}
		})
		// Any press or scroll clears the tooltip (upstream clears unless the
		// tooltip itself is hovered; this port's tooltips are not hoverable).
		clearTooltip := func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			persist(func(s *interactiveState) {
				if s.tooltipPhase != tooltipNone {
					s.tooltipPhase = tooltipNone
					w.Refresh()
				}
			})
		}
		w.OnMouseEvent("any-down", platform.ButtonLeft, hitbox, false, clearTooltip)
		w.OnMouseEvent("scroll", platform.ButtonLeft, hitbox, false, clearTooltip)
	}
	// Capture-phase press (overlay grips and scrollbar tracks claim the
	// gesture before the interactive content underneath).
	if d.onMouseDownCapture != nil {
		w.OnMouseEvent("down", platform.ButtonLeft, hitbox, true, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase == gpui.CapturePhase {
				d.onMouseDownCapture(ev.(platform.MouseDownEvent), w)
			}
		})
	}
	// Mouse down: record pending click + focus.
	if d.onClick != nil || d.onMouseDown != nil {
		w.OnMouseEvent("down", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			e := ev.(platform.MouseDownEvent)
			if d.onMouseDown != nil {
				d.onMouseDown(e, w)
			}
			if d.onClick != nil {
				persist(func(s *interactiveState) {
					s.pendingMouseDown = &e
					s.clicked = true
				})
				w.StopPropagation()
			}
		})
	}
	if d.onMouseDown != nil {
		w.OnMouseEvent("any-down", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			e := ev.(platform.MouseDownEvent)
			// The left button is already delivered by the "down"
			// listener above; "any-down" only forwards the other
			// buttons so a press is never handled twice.
			if e.Button == platform.ButtonLeft {
				return
			}
			if d.onMouseDown != nil && d.onClick == nil {
				d.onMouseDown(e, w)
			}
		})
	}
	// Right-button press: context menus and other secondary actions.
	if d.onRightDown != nil {
		w.OnMouseEvent("down", platform.ButtonRight, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase == gpui.BubblePhase {
				d.onRightDown(ev.(platform.MouseDownEvent), w)
			}
		})
	}
	// Mouse up: complete the click.
	if d.onClick != nil {
		w.OnMouseEvent("up", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			e := ev.(platform.MouseUpEvent)
			if phase == gpui.CapturePhase {
				// Capture: verify the pending press is still inside. The
				// hitbox is from the frame that registered the listener,
				// so test geometry (stable) rather than per-frame ids.
				persist(func(s *interactiveState) {
					if s.pendingMouseDown != nil && !geometry.BoundsContains(hitbox.Bounds, w.MousePosition()) {
						s.pendingMouseDown = nil
					}
					s.clicked = false
				})
				return
			}
			var pending *platform.MouseDownEvent
			persist(func(s *interactiveState) {
				if s.pendingMouseDown != nil {
					pending = s.pendingMouseDown
					s.pendingMouseDown = nil
				}
			})
			if pending != nil {
				d.onClick(gpui.ClickEvent{Down: *pending, Up: e}, w)
				w.StopPropagation()
			}
		})
	}
	if d.onMouseUp != nil {
		w.OnMouseEvent("up", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase == gpui.BubblePhase {
				d.onMouseUp(ev.(platform.MouseUpEvent), w)
			}
		})
	}
	if d.onMouseMove != nil {
		w.OnMouseEvent("move", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase == gpui.BubblePhase {
				d.onMouseMove(ev.(platform.MouseMoveEvent), w)
			}
		})
	}
	// Drag detection on move.
	if d.onDrag != nil {
		w.OnMouseEvent("move", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			e := ev.(platform.MouseMoveEvent)
			var drag *gpui.DragEvent
			persist(func(s *interactiveState) {
				if pending := s.pendingMouseDown; pending != nil {
					dx := e.Position.X.AsF32() - pending.Position.X.AsF32()
					dy := e.Position.Y.AsF32() - pending.Position.Y.AsF32()
					if dx*dx+dy*dy > DragThreshold*DragThreshold {
						s.pendingMouseDown = nil
						s.clicked = false
						drag = &gpui.DragEvent{
							Position: e.Position,
							Delta:    geometry.Point[geometry.Pixels]{X: geometry.Px(dx), Y: geometry.Px(dy)},
						}
					}
				}
			})
			if drag != nil {
				d.onDrag(*drag, w)
				w.StopPropagation()
			}
		})
	}
	// Scroll: custom listeners run first (upstream on_scroll; handlers may
	// stop propagation themselves). The default offset-translating handler
	// is registered only for scroll containers (upstream
	// paint_scroll_listener, gated on scroll_offset.is_some()).
	if d.onScroll != nil {
		w.OnMouseEvent("scroll", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			e := ev.(platform.ScrollEvent)
			if d.onScroll != nil {
				d.onScroll(e, w)
			}
		})
	} else if d.isScrollContainer() && d.scrollableHitbox.ID == hitbox.ID {
		// Default scroll behavior: translate the offset, clamp to the
		// content extent; propagation continues (upstream does not stop
		// it here, so outer containers can scroll remaining distance).
		// The offset persists via the captured element path (upstream
		// holds an Rc into the element state).
		gid := w.CurrentElementID()
		restrictAxis := st.RestrictScrollToAxis
		w.OnMouseEvent("scroll", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			if !hitbox.ShouldHandleScroll(w) {
				return
			}
			e := ev.(platform.ScrollEvent)
			delta := w.ZoomedScrollDelta(e.Delta.PixelDelta(geometry.Px(20)))
			// Shift+vertical-wheel pans horizontally (the standard wheel
			// behavior on platforms that do not deliver native X deltas,
			// like Windows).
			if e.Modifiers.Shift && delta.X.AsF32() == 0 && delta.Y.AsF32() != 0 {
				delta = geometry.Point[geometry.Pixels]{X: delta.Y, Y: geometry.Px(0)}
			}
			// Lock touch pans to their dominant axis while
			// restrict_scroll_to_axis is set (upstream
			// OngoingScroll::filter in paint_scroll_listener).
			if restrictAxis && e.Delta.Precise() {
				gpui.UpdateElementStateAt(w, gid, func() interactiveState { return interactiveState{} }, func(s *interactiveState) {
					s.ongoingScroll.Filter(&delta.X, &delta.Y, e.TouchPhase)
				})
			}
			scrollSize := w.ElementScrollSize(d)
			bounds := w.ElementBounds(d)
			maxX := scrollSize.Width - bounds.Size.Width
			maxY := scrollSize.Height - bounds.Size.Height
			gpui.UpdateElementStateAt(w, gid, func() interactiveState { return interactiveState{} }, func(s *interactiveState) {
				s.scrollOffset.X += delta.X
				s.scrollOffset.Y += delta.Y
				if s.scrollOffset.Y.AsF32() > 0 {
					s.scrollOffset.Y = 0
				}
				if maxY.AsF32() < 0 {
					// Content shorter than the viewport: no scrolling.
					s.scrollOffset.Y = 0
				} else if s.scrollOffset.Y.AsF32() < -maxY.AsF32() {
					s.scrollOffset.Y = -maxY
				}
				if s.scrollOffset.X.AsF32() > 0 {
					s.scrollOffset.X = 0
				}
				if maxX.AsF32() < 0 {
					s.scrollOffset.X = 0
				} else if s.scrollOffset.X.AsF32() < -maxX.AsF32() {
					s.scrollOffset.X = -maxX
				}
				// User scroll input cancels handle
				// bottom-following (autoscroll stays off
				// until re-armed).
				if d.scrollHandle != nil {
					d.scrollHandle.clearAutoFollow()
					d.scrollHandle.syncNow(s.scrollOffset)
				}
			})
		})
	}
	// Gesture listeners (upstream on_pinch / on_long_press /
	// on_touch_drag / on_back_gesture).
	if d.onPinch != nil {
		w.OnMouseEvent("pinch", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase == gpui.BubblePhase {
				d.onPinch(ev.(platform.PinchEvent), w)
			}
		})
	}
	if d.onLongPress != nil {
		w.OnMouseEvent("long-press", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase == gpui.BubblePhase {
				d.onLongPress(ev.(platform.LongPressEvent), w)
			}
		})
	}
	if d.onTouchDrag != nil {
		w.OnMouseEvent("touch-drag", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase == gpui.BubblePhase {
				d.onTouchDrag(ev.(platform.TouchDragEvent), w)
			}
		})
	}
	if d.onBackGesture != nil {
		w.OnMouseEvent("back-gesture", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase == gpui.BubblePhase {
				d.onBackGesture(ev.(platform.BackGestureEvent), w)
			}
		})
	}
	if d.onFileDrop != nil {
		w.OnMouseEvent("file-drop", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase == gpui.BubblePhase {
				d.onFileDrop(ev.(platform.FileDropEvent), w)
			}
		})
	}
	// Keyboard. The listener is bound to the element's focus handle; the
	// window only dispatches to it when that handle held focus when the
	// event started.
	if d.onKeyDown != nil && state.focusHandle.IsValid() {
		fh := state.focusHandle
		w.OnKeyEventFocused("down", fh, d.keyContext, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase == gpui.BubblePhase {
				d.onKeyDown(ev.(platform.KeyDownEvent), w)
			}
		})
	}
	// Keyboard-invoked click (upstream records pending_keyboard_down on
	// enter/space key-down and completes the ClickEvent on key-up while
	// focus did not move; this port synthesizes the click on key-down).
	if d.onClick != nil && d.focusable && state.focusHandle.IsValid() {
		fh := state.focusHandle
		w.OnKeyEventFocused("down", fh, d.keyContext, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase || w.DefaultPrevented() {
				return
			}
			e := ev.(platform.KeyDownEvent)
			k := e.Keystroke
			if k.Modifiers.Modified() || (k.Key != "enter" && k.Key != "space") {
				return
			}
			center := geometry.PointPixels(
				hitbox.Bounds.Origin.X.AsF32()+hitbox.Bounds.Size.Width.AsF32()/2,
				hitbox.Bounds.Origin.Y.AsF32()+hitbox.Bounds.Size.Height.AsF32()/2)
			d.onClick(gpui.ClickEvent{
				Down: platform.MouseDownEvent{Button: platform.ButtonLeft, Position: center, ClickCount: 1},
				Up:   platform.MouseUpEvent{Button: platform.ButtonLeft, Position: center, ClickCount: 1},
			}, w)
			w.StopPropagation()
			w.Refresh()
		})
	}
	// Focus on click.
	if d.focusable && state.focusHandle.IsValid() {
		w.OnMouseEvent("down", platform.ButtonLeft, hitbox, false, func(ev any, phase gpui.DispatchPhase, w *gpui.Window) {
			if phase != gpui.BubblePhase {
				return
			}
			// Innermost focus wins: a deeper focusable element may
			// have claimed focus earlier in this same event's bubble
			// phase (a text input inside this TrackFocus container).
			// Stealing it back would leave the input uneditable.
			if w.FocusMovedThisDispatch() {
				return
			}
			w.Focus(state.focusHandle)
		})
		w.SetFocusHandle(state.focusHandle)
		w.InsertTabStop(state.focusHandle, 0)
	}
}

// currentHitbox carries the prepaint-computed hitbox to paint.
type hitboxHolder struct{ hb gpui.Hitbox }

// ElementBounds exposes a child element's prepaint bounds.
func elementBoundsOf(e gpui.Element) {}

var _ = app.EntityID{}
var _ = gc.Red()

// ButtonID builds a conventional element id for buttons.
func ButtonID(name string) gpui.ElementId { return gpui.NameElementId("btn-" + name) }

// paintTooltipState advances the tooltip state machine at paint time and
// schedules the overlay for this frame (upstream register_tooltip_mouse_
// handlers plus the window's prepaint check: the tooltip shows once the
// hover delay elapsed and hides when the mouse left the source bounds or
// the source stopped rendering under the pointer).
func (d *Div) paintTooltipState(w *gpui.Window, state *interactiveState, hitbox gpui.Hitbox) {
	if d.tooltipText == "" && d.tooltipBuilder == nil {
		return
	}
	if state.tooltipPhase == tooltipWaiting && w.NowMilliseconds() >= state.tooltipShowAtMS {
		state.tooltipPhase = tooltipVisible
	}
	if state.tooltipPhase != tooltipWaiting {
		state.tooltipTimerActive = false
	}
	if state.tooltipPhase == tooltipVisible {
		// Stationary-mouse case: hide when the source no longer sits under
		// the pointer (upstream check_is_hovered_during_prepaint tests the
		// source bounds; keyboard modality suppresses hover).
		if w.LastInputWasKeyboard() || !hitbox.IsHovered(w) {
			state.tooltipPhase = tooltipNone
			state.tooltipTimerActive = false
		}
	}
	if state.tooltipPhase != tooltipVisible {
		return
	}
	if el := d.buildTooltip(w); el != nil {
		w.SetTooltip(el, state.tooltipPos)
	}
}

// buildTooltip constructs the tooltip view (upstream tooltip_builder; the
// default builds a labeled tooltip surface).
func (d *Div) buildTooltip(w *gpui.Window) gpui.Element {
	if d.tooltipBuilder != nil {
		return d.tooltipBuilder(w)
	}
	return NewTooltip(d.tooltipText)
}

// defaultDragVisual is the fallback surface painted under the cursor when
// the DragState carries no view (upstream drag callers usually render a
// snapshot; this port paints a small labeled chip).
func (d *Div) defaultDragVisual(w *gpui.Window, content string) gpui.Element {
	colors := w.DefaultColors()
	return NewDiv().
		BgColor(colors.Background.Hsla()).
		Border_1().
		BorderColor(colors.Border.Hsla()).
		Rounded_Sm().
		Px_2().
		Py_1().
		TextColor(colors.Text.Hsla()).
		Child(NewText(content))
}
