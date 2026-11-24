package gpui

import (
	"sync"
	"time"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/layout"
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/style"
	"github.com/FlameInTheDark/go-gpui/text"
)

// PromptLevel is the severity of a modal prompt (upstream PromptLevel).
type PromptLevel uint8

const (
	// PromptLevelInfo is an informational dialog.
	PromptLevelInfo PromptLevel = 0
	// PromptLevelWarning warns about a destructive action.
	PromptLevelWarning PromptLevel = 1
	// PromptLevelCritical reports a fatal condition.
	PromptLevelCritical PromptLevel = 2
)

// PromptAction is one answer of a prompt (upstream PromptAction: the label
// of a response button; the response index is the action's position).
type PromptAction struct {
	// Label is the button text.
	Label string
}

// NewPromptAction builds an action from its label.
func NewPromptAction(label string) PromptAction { return PromptAction{Label: label} }

// PromptResponse is the answer channel of a prompt (upstream
// PromptResponse { index, changed }). Index reports the chosen action's
// position once answered; Changed signals when the prompt is answered or
// superseded by a newer prompt.
type PromptResponse struct {
	mu       sync.Mutex
	index    int
	answered bool
	changed  chan struct{}
}

// Index reports the chosen action index and whether the prompt was
// answered yet.
func (r *PromptResponse) Index() (int, bool) {
	if r == nil {
		return 0, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.index, r.answered
}

// Changed returns a channel closed when the prompt is answered or superseded.
// Check Index to tell whether an answer was recorded.
func (r *PromptResponse) Changed() <-chan struct{} {
	if r == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return r.changed
}

// answer records the chosen action.
func (r *PromptResponse) answer(index int) {
	r.mu.Lock()
	r.index = index
	r.answered = true
	r.mu.Unlock()
}

// supersede closes the changed channel.
func (r *PromptResponse) supersede() {
	select {
	case <-r.changed:
	default:
		close(r.changed)
	}
}

// PromptInputResponse is the answer channel of a text-input prompt
// (upstream prompt_for_input's Task<Option<String>>). Value reports the
// entered string once committed; cancelled prompts stay unanswered.
type PromptInputResponse struct {
	mu      sync.Mutex
	value   string
	commit  bool
	changed chan struct{}
}

// Value reports the committed input and whether it was committed
// (cancelled prompts never commit).
func (r *PromptInputResponse) Value() (string, bool) {
	if r == nil {
		return "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.value, r.commit
}

// Changed returns a channel closed when the input is committed or the prompt
// is superseded. Check Value to tell whether input was committed.
func (r *PromptInputResponse) Changed() <-chan struct{} {
	if r == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return r.changed
}

func (r *PromptInputResponse) commitValue(v string) {
	r.mu.Lock()
	r.value = v
	r.commit = true
	r.mu.Unlock()
}

func (r *PromptInputResponse) supersede() {
	select {
	case <-r.changed:
	default:
		close(r.changed)
	}
}

// promptLayer is one open prompt (upstream window.prompts is a stack of
// layers; this port keeps the stack in Window.prompts).
type promptLayer struct {
	level   PromptLevel
	title   string
	message string
	detail  string
	// actions are the response buttons (plain prompts).
	actions []PromptAction
	// input marks a prompt_for_input variant.
	input       bool
	placeholder string

	// Editable state (input prompts).
	value []byte // UTF-8 buffer
	caret int    // byte offset
	// focus is the input field's focus handle.
	focus FocusHandle

	// response is the plain-prompt answer; inputResponse the input one.
	response      *PromptResponse
	inputResponse *PromptInputResponse

	// interaction state (repainted every frame).
	hoverButton int // -1 none
	pressButton int // -1 none
	inputHover  bool
	inputFocus  bool

	// element is this layer's overlay dialog (reused across phases so
	// prepaint-computed geometry survives into paint).
	element *promptDialog
	node    LayoutID
	valid   bool

	// blinkVisible toggles the caret every 530ms while focused.
	blinkVisible bool
}

// promptID deduplicates identical prompts (upstream compares the prompt
// view; this port compares the tuple).
func (p *promptLayer) promptID() string {
	id := string(rune('0'+p.level)) + "|" + p.title + "|" + p.message + "|" + p.detail
	for _, a := range p.actions {
		id += "|" + a.Label
	}
	return id
}

// ---------------------------------------------------------------------------
// Window API (upstream Window::prompt / prompt_for_input / dismiss_prompt /
// prompt_answer)
// ---------------------------------------------------------------------------

// Prompt opens a modal in-app dialog with the given level, title, message,
// detail and actions (upstream Window::prompt). It returns the response
// handle, or nil when an identical prompt is already open on the top
// layer (upstream returns Option::None for a duplicate prompt view).
//
// The dialog renders centered above everything; number keys 1..n pick an
// action, Enter picks the first, Escape picks the last (cancel
// convention). The answer is also reported through DismissPrompt.
func (w *Window) Prompt(level PromptLevel, title, message, detail string, actions []PromptAction) *PromptResponse {
	if len(actions) == 0 {
		actions = []PromptAction{{Label: "OK"}}
	}
	layer := &promptLayer{
		level:       level,
		title:       title,
		message:     message,
		detail:      detail,
		actions:     actions,
		hoverButton: -1,
		pressButton: -1,
		response:    &PromptResponse{index: -1, changed: make(chan struct{})},
	}
	return w.pushPromptLayer(layer)
}

// PromptForInput opens a modal dialog with a single-line text input
// (upstream Window::prompt_for_input). The returned response commits the
// entered value on Enter; Escape cancels (never commits). defaultValue
// pre-fills the field.
func (w *Window) PromptForInput(level PromptLevel, title, message, detail, placeholder, defaultValue string) *PromptInputResponse {
	layer := &promptLayer{
		level:         level,
		title:         title,
		message:       message,
		detail:        detail,
		input:         true,
		placeholder:   placeholder,
		value:         []byte(defaultValue),
		caret:         len(defaultValue),
		focus:         NewFocusHandle(),
		hoverButton:   -1,
		pressButton:   -1,
		inputResponse: &PromptInputResponse{changed: make(chan struct{})},
	}
	return w.pushInputPromptLayer(layer)
}

func (w *Window) pushPromptLayer(layer *promptLayer) *PromptResponse {
	w.supersedePrompts()
	// Deduplicate an identical top prompt (upstream re-uses the view).
	if n := len(w.prompts); n > 0 && w.prompts[n-1].promptID() == layer.promptID() {
		return nil
	}
	w.prompts = append(w.prompts, layer)
	w.afterPromptChange()
	return layer.response
}

func (w *Window) pushInputPromptLayer(layer *promptLayer) *PromptInputResponse {
	w.supersedePrompts()
	w.prompts = append(w.prompts, layer)
	w.Focus(layer.focus)
	w.afterPromptChange()
	return layer.inputResponse
}

// supersedePrompts closes every open response's changed channel: a new
// prompt supersedes all older ones (upstream invalidates the old tasks).
func (w *Window) supersedePrompts() {
	for _, p := range w.prompts {
		if p.response != nil {
			p.response.supersede()
		}
		if p.inputResponse != nil {
			p.inputResponse.supersede()
		}
	}
}

func (w *Window) afterPromptChange() {
	w.dirty = true
	if w.drawPhase == PhaseNone {
		w.ScheduleFrame()
	}
}

// DismissPrompt closes the prompt at the given layer and returns its
// response with the chosen answer (upstream Window::dismiss_prompt). The
// response is nil when no prompt occupies the layer.
func (w *Window) DismissPrompt(layer int) *PromptResponse {
	if layer < 0 || layer >= len(w.prompts) {
		return nil
	}
	p := w.prompts[layer]
	w.prompts = append(w.prompts[:layer], w.prompts[layer+1:]...)
	w.releasePromptFocus(p)
	w.afterPromptChange()
	return p.response
}

// PromptAnswer returns the current answer of the prompt at the given
// layer without closing it (upstream Window::prompt_answer).
func (w *Window) PromptAnswer(layer int) *PromptResponse {
	if layer < 0 || layer >= len(w.prompts) {
		return nil
	}
	return w.prompts[layer].response
}

// PromptCount reports open prompt layers.
func (w *Window) PromptCount() int { return len(w.prompts) }

// releasePromptFocus blurs the input prompt's focus when it closes (the
// focused element vanished with the dialog).
func (w *Window) releasePromptFocus(p *promptLayer) {
	if p.input && w.focus != nil && *w.focus == p.focus {
		w.Blur()
	}
}

// answerPrompt records the chosen action and closes the layer.
func (w *Window) answerPrompt(layer *promptLayer, index int) {
	if layer.response != nil {
		layer.response.answer(index)
		// Finalize: close the changed channel so waiters blocked on it
		// (the response-is-ready contract callers rely on) wake up. Upstream
		// resolves the Task; Changed() is this port's completion signal, so it
		// must fire on the answer too, not only on supersession.
		layer.response.supersede()
	}
	if layer.inputResponse != nil {
		layer.inputResponse.commitValue(string(layer.value))
		layer.inputResponse.supersede()
	}
	w.dismissPromptLayer(layer)
}

// dismissPromptLayer removes the layer from the prompt stack.
func (w *Window) dismissPromptLayer(layer *promptLayer) {
	for i, p := range w.prompts {
		if p == layer {
			w.prompts = append(w.prompts[:i], w.prompts[i+1:]...)
			break
		}
	}
	w.releasePromptFocus(layer)
	w.afterPromptChange()
}

// isPromptLayerOpen reports whether the layer is still in the stack.
func (w *Window) isPromptLayerOpen(layer *promptLayer) bool {
	for _, p := range w.prompts {
		if p == layer {
			return true
		}
	}
	return false
}

// isTopPromptLayer reports whether the layer is the topmost prompt (only
// the top prompt consumes keys).
func (w *Window) isTopPromptLayer(layer *promptLayer) bool {
	n := len(w.prompts)
	return n > 0 && w.prompts[n-1] == layer
}

// ---------------------------------------------------------------------------
// Prompt dialog element (rendered from the window's overlay path; built on
// the low-level paint API because the gpui package cannot use the elements
// package's Div).
// ---------------------------------------------------------------------------

// promptDialog is the element rendering one prompt layer.
type promptDialog struct {
	layer *promptLayer
	// shaped text lines from the last geometry pass (index 0 title,
	// 1 message, 2 optional detail).
	lines []*text.LineLayout
	// computed geometry (window coordinates).
	buttonRects []geometry.Bounds[geometry.Pixels]
	inputRect   geometry.Bounds[geometry.Pixels]
	cardRect    geometry.Bounds[geometry.Pixels]
}

// ElementID implements Element.
func (p *promptDialog) ElementID() ElementId { return NameElementId("prompt-dialog") }

// RequestLayout implements Element.
func (p *promptDialog) RequestLayout(w *Window) {
	st := style.DefaultStyle()
	st.Size.Width = geometry.LengthFromDefinite(geometry.RelativeDefinite(1.0))
	st.Size.Height = geometry.LengthFromDefinite(geometry.RelativeDefinite(1.0))
	w.RequestLayout(st, nil)
}

// promptFontRun resolves the window base font for prompt text.
func promptFontRun(w *Window, str string) []text.FontRun {
	sys := w.TextSystem()
	id, _ := sys.FontID(w.TextStyle().ToFont())
	return []text.FontRun{{Len: len(str), FontID: id}}
}

// shapePromptText shapes one prompt string at a scaled size.
func shapePromptText(w *Window, str string, scale float32) *text.LineLayout {
	sys := w.TextSystem()
	ts := w.TextStyle()
	size := geometry.Px(ts.FontSize.ToPixels(w.RemSize()).AsF32() * scale)
	return sys.LayoutLine(str, size, promptFontRun(w, str))
}

// Prepaint implements Element.
func (p *promptDialog) Prepaint(w *Window, bounds geometry.Bounds[geometry.Pixels]) {
	p.computeGeometry(w, bounds)
	layer := p.layer

	// Scrim swallows clicks below the dialog (block-mouse hitbox over the
	// whole window); the card blocks too.
	scrim := geometry.Bounds[geometry.Pixels]{
		Origin: geometry.Point[geometry.Pixels]{},
		Size:   w.plat.ContentSize(),
	}
	w.InsertHitbox(scrim, HitboxBlockMouse)
	w.InsertHitbox(p.cardRect, HitboxBlockMouse)

	// Buttons: hover + click tracking (down then up on the same button).
	for i := range p.buttonRects {
		idx := i
		hb := w.InsertHitbox(p.buttonRects[i], HitboxNormal)
		w.OnMouseEvent("move", platform.ButtonLeft, hb, false, func(ev any, phase DispatchPhase, w *Window) {
			if phase == BubblePhase {
				layer.hoverButton = idx
			}
		})
		w.OnMouseEvent("exit", platform.ButtonLeft, hb, false, func(ev any, phase DispatchPhase, w *Window) {
			if phase == BubblePhase {
				layer.hoverButton = -1
			}
		})
		w.OnMouseEvent("down", platform.ButtonLeft, hb, false, func(ev any, phase DispatchPhase, w *Window) {
			if phase == BubblePhase {
				layer.pressButton = idx
			}
		})
		w.OnMouseEvent("up", platform.ButtonLeft, hb, false, func(ev any, phase DispatchPhase, w *Window) {
			if phase == BubblePhase && layer.pressButton == idx {
				w.answerPrompt(layer, idx)
			}
			layer.pressButton = -1
		})
	}

	if !layer.input {
		// Plain prompt keys (only the top prompt consumes): digits select,
		// Enter first, Escape last.
		w.OnKeyEvent("down", "Prompt", func(ev any, phase DispatchPhase, w *Window) {
			e, ok := ev.(platform.KeyDownEvent)
			if !ok || phase != BubblePhase || !w.isTopPromptLayer(layer) {
				return
			}
			if e.Keystroke.Modifiers.Modified() {
				return
			}
			switch e.Keystroke.Key {
			case "escape":
				w.answerPrompt(layer, len(layer.actions)-1)
				w.StopPropagation()
				w.PreventDefault()
			case "enter":
				w.answerPrompt(layer, 0)
				w.StopPropagation()
				w.PreventDefault()
			default:
				if len(e.Keystroke.Key) == 1 && e.Keystroke.Key[0] >= '1' && e.Keystroke.Key[0] <= '9' {
					idx := int(e.Keystroke.Key[0] - '1')
					if idx < len(layer.actions) {
						w.answerPrompt(layer, idx)
						w.StopPropagation()
						w.PreventDefault()
					}
				}
			}
		})
		return
	}

	// Input field: click to focus + position the caret.
	hb := w.InsertHitbox(p.inputRect, HitboxNormal)
	w.OnMouseEvent("move", platform.ButtonLeft, hb, false, func(ev any, phase DispatchPhase, w *Window) {
		if phase == BubblePhase {
			layer.inputHover = true
		}
	})
	w.OnMouseEvent("exit", platform.ButtonLeft, hb, false, func(ev any, phase DispatchPhase, w *Window) {
		if phase == BubblePhase {
			layer.inputHover = false
		}
	})
	w.OnMouseEvent("down", platform.ButtonLeft, hb, false, func(ev any, phase DispatchPhase, w *Window) {
		if phase == BubblePhase {
			w.Focus(layer.focus)
			if e, ok := ev.(platform.MouseDownEvent); ok {
				p.setCaretForX(w, e.Position.X.AsF32()-p.inputRect.Origin.X.AsF32())
			}
		}
	})
	w.WithFocusPath(layer.focus, func() {
		w.SetFocusHandle(layer.focus)
	})
	w.InsertTabStop(layer.focus, 0)
	layer.inputFocus = w.Focused() == layer.focus

	// Editing keys (focus-bound, top prompt only).
	focus := layer.focus
	w.OnKeyEventFocused("down", focus, "Prompt", func(ev any, phase DispatchPhase, w *Window) {
		e, ok := ev.(platform.KeyDownEvent)
		if !ok || phase != BubblePhase || !w.isTopPromptLayer(layer) {
			return
		}
		if p.handleInputKey(w, e) {
			w.StopPropagation()
			w.PreventDefault()
		}
	})
	// Caret blink: repaint every 530ms while the input has focus.
	if layer.inputFocus && w.CanScheduleTimers() {
		w.ScheduleAfter(530*time.Millisecond, func(w *Window) {
			if w.isPromptLayerOpen(layer) {
				layer.blinkVisible = !layer.blinkVisible
				w.dirty = true
				if w.drawPhase == PhaseNone {
					w.ScheduleFrame()
				}
			}
		})
	}
}

// setCaretForX moves the input caret to the byte offset nearest x.
func (p *promptDialog) setCaretForX(w *Window, x float32) {
	l := shapePromptText(w, string(p.layer.value), 1.0)
	p.layer.caret = clampPromptInt(l.ClosestIndexForX(geometry.Px(x)), 0, len(p.layer.value))
}

// handleInputKey applies an editing key; reports whether it consumed.
func (p *promptDialog) handleInputKey(w *Window, e platform.KeyDownEvent) bool {
	k := e.Keystroke
	layer := p.layer
	if k.Modifiers.Modified() && !(k.Modifiers.Control && k.Key == "a") {
		return false
	}
	switch k.Key {
	case "escape":
		// Cancel: close without committing.
		w.dismissPromptLayer(layer)
		return true
	case "enter":
		layer.inputResponse.commitValue(string(layer.value))
		w.dismissPromptLayer(layer)
		return true
	case "backspace":
		if layer.caret > 0 {
			start := layer.caret - 1
			for start > 0 && (layer.value[start]&0xC0) == 0x80 {
				start--
			}
			layer.value = append(layer.value[:start], layer.value[layer.caret:]...)
			layer.caret = start
		}
		return true
	case "delete":
		if layer.caret < len(layer.value) {
			end := layer.caret + 1
			for end < len(layer.value) && (layer.value[end]&0xC0) == 0x80 {
				end++
			}
			layer.value = append(layer.value[:layer.caret], layer.value[end:]...)
		}
		return true
	case "left":
		if layer.caret > 0 {
			layer.caret--
			for layer.caret > 0 && (layer.value[layer.caret]&0xC0) == 0x80 {
				layer.caret--
			}
		}
		return true
	case "right":
		if layer.caret < len(layer.value) {
			layer.caret++
			for layer.caret < len(layer.value) && (layer.value[layer.caret]&0xC0) == 0x80 {
				layer.caret++
			}
		}
		return true
	case "home":
		layer.caret = 0
		return true
	case "end":
		layer.caret = len(layer.value)
		return true
	case "a":
		// select-all is a no-op for a single-line field; consume.
		return true
	}
	if k.KeyChar != "" && !k.Modifiers.Modified() {
		layer.value = append(layer.value, k.KeyChar...)
		layer.caret += len(k.KeyChar)
		return true
	}
	return false
}

func clampPromptInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// promptEdges is a uniform absolute edge width helper.
func promptEdges(v geometry.Pixels) geometry.EdgesAbsolute {
	l := geometry.PixelsLength(v)
	return geometry.EdgesAbsolute{Top: l, Right: l, Bottom: l, Left: l}
}

// promptCorners is a uniform absolute corner radius helper.
func promptCorners(v geometry.Pixels) geometry.CornersAbsolute {
	l := geometry.PixelsLength(v)
	return geometry.CornersAbsolute{TopLeft: l, TopRight: l, BottomRight: l, BottomLeft: l}
}

// computeGeometry lays out the card content (called from Prepaint; bounds
// is the full-window overlay rect).
func (p *promptDialog) computeGeometry(w *Window, bounds geometry.Bounds[geometry.Pixels]) {
	rem := w.RemSize().AsF32()
	if rem <= 0 {
		rem = 16
	}
	pad := rem * 1.25
	fontSize := style.DefaultStyle().Text.FontSize.ToPixels(w.RemSize()).AsF32()

	p.lines = p.lines[:0]
	maxW := float32(0)
	shapeAt := func(str string, scale float32) *text.LineLayout {
		l := shapePromptText(w, str, scale)
		p.lines = append(p.lines, l)
		if l.Width.AsF32() > maxW {
			maxW = l.Width.AsF32()
		}
		return l
	}

	titleL := shapeAt(p.layer.title, 1.15)
	msgL := shapeAt(p.layer.message, 1.0)
	if p.layer.detail != "" {
		shapeAt(p.layer.detail, 0.9)
	}

	cardW := maxW + pad*2
	maxCard := bounds.Size.Width.AsF32() * 0.9
	if cardW > maxCard {
		cardW = maxCard
	}
	if cardW < rem*18 {
		cardW = rem * 18
	}

	// Vertical stack (window-space y, card-relative below).
	titleH := titleL.Ascent.AsF32() + titleL.Descent.AsF32() + rem*0.5
	msgH := msgL.Ascent.AsF32() + msgL.Descent.AsF32() + rem*0.35
	detailH := float32(0)
	if p.layer.detail != "" {
		dl := p.lines[len(p.lines)-1]
		detailH = dl.Ascent.AsF32() + dl.Descent.AsF32() + rem*0.35
	}
	inputH := float32(0)
	if p.layer.input {
		inputH = fontSize*2.0 + rem*0.5
	}
	buttonH := rem * 1.9
	cardH := pad + titleH + msgH + detailH + inputH + buttonH + pad

	// Center the card in the window (upstream centers prompts).
	cx := bounds.Size.Width.AsF32()
	cy := bounds.Size.Height.AsF32()
	ox := (cx - cardW) / 2
	oy := (cy - cardH) / 2
	if oy < rem {
		oy = rem
	}
	p.cardRect = geometry.Bounds[geometry.Pixels]{
		Origin: geometry.PointPixels(ox, oy),
		Size:   geometry.SizePixels(cardW, cardH),
	}

	// Buttons: right-aligned row.
	p.buttonRects = p.buttonRects[:0]
	if !p.layer.input {
		x := ox + cardW - pad
		for i := len(p.layer.actions) - 1; i >= 0; i-- {
			lbl := shapePromptText(w, p.layer.actions[i].Label, 0.95)
			bw := lbl.Width.AsF32() + rem*1.8 + rem*0.1
			x -= bw
			p.buttonRects = append([]geometry.Bounds[geometry.Pixels]{{
				Origin: geometry.PointPixels(x, oy+cardH-pad-buttonH),
				Size:   geometry.SizePixels(bw, buttonH),
			}}, p.buttonRects...)
			x -= rem * 0.4
		}
	}
	p.inputRect = geometry.Bounds[geometry.Pixels]{}
	if p.layer.input {
		p.inputRect = geometry.Bounds[geometry.Pixels]{
			Origin: geometry.PointPixels(ox+pad, oy+pad+titleH+msgH+detailH),
			Size:   geometry.SizePixels(cardW-pad*2, fontSize*2.0),
		}
	}
}

// Paint implements Element.
func (p *promptDialog) Paint(w *Window) {
	colors := w.DefaultColors()
	layer := p.layer
	rem := w.RemSize().AsF32()
	if rem <= 0 {
		rem = 16
	}

	// Scrim: dim the whole window behind the dialog.
	w.PaintQuad(solidQuadStyle(gc.Hsla{H: 0, S: 0, L: 0, A: 0.5}), geometry.Bounds[geometry.Pixels]{
		Origin: geometry.Point[geometry.Pixels]{},
		Size:   w.plat.ContentSize(),
	})

	// Card.
	card := p.cardRect
	w.PaintQuad(solidQuadStyle(colors.Container.Hsla()), card)
	border := colors.Border.Hsla()
	border.A = 0.6
	st := solidQuadStyle(colors.Container.Hsla())
	st.BorderColor = &border
	st.BorderWidths = promptEdges(geometry.Px(1))
	w.PaintQuad(st, card)

	// Level badge (colored bar left of the title).
	levelColor := gc.Hsla{H: 210, S: 0.7, L: 0.55, A: 1}
	switch layer.level {
	case PromptLevelWarning:
		levelColor = gc.Hsla{H: 40, S: 0.9, L: 0.55, A: 1}
	case PromptLevelCritical:
		levelColor = gc.Hsla{H: 0, S: 0.8, L: 0.55, A: 1}
	}
	pad := rem * 1.25
	titleL := p.lines[0]
	msgL := p.lines[1]
	w.PaintQuad(solidQuadStyle(levelColor), geometry.Bounds[geometry.Pixels]{
		Origin: geometry.PointPixels(card.Origin.X.AsF32()+pad, card.Origin.Y.AsF32()+pad+rem*0.05),
		Size:   geometry.SizePixels(rem*0.18, titleL.Ascent.AsF32()+titleL.Descent.AsF32()),
	})

	textColor := colors.Text.Hsla()
	titleX := card.Origin.X.AsF32() + pad + rem*0.5
	paintPromptLine(w, titleL, geometry.PointPixels(titleX, card.Origin.Y.AsF32()+pad+titleL.Ascent.AsF32()), textColor)

	paintPromptLine(w, msgL, geometry.PointPixels(titleX, card.Origin.Y.AsF32()+pad+promptLineH(titleL, rem*0.5)+msgL.Ascent.AsF32()), textColor)

	if len(p.lines) > 2 {
		detailL := p.lines[2]
		dim := textColor
		dim.A = 0.65
		paintPromptLine(w, detailL, geometry.PointPixels(titleX, card.Origin.Y.AsF32()+pad+promptLineH(titleL, rem*0.5)+promptLineH(msgL, rem*0.35)+detailL.Ascent.AsF32()), dim)
	}

	// Input field.
	if layer.input {
		rect := p.inputRect
		fieldBg := colors.Background.Hsla()
		fieldBg.A = 0.85
		fst := solidQuadStyle(fieldBg)
		focusBorder := colors.Border.Hsla()
		if layer.inputFocus {
			focusBorder = gc.Hsla{H: 210, S: 0.8, L: 0.6, A: 1}
		} else if layer.inputHover {
			focusBorder.A = 0.9
		} else {
			focusBorder.A = 0.6
		}
		fst.BorderColor = &focusBorder
		fst.BorderWidths = promptEdges(geometry.Px(1))
		w.PaintQuad(fst, rect)

		inner := rect.Origin.X.AsF32() + rem*0.45
		valueL := shapePromptText(w, string(layer.value), 1.0)
		baseline := rect.Origin.Y.AsF32() + (rect.Size.Height.AsF32()-(valueL.Ascent.AsF32()+valueL.Descent.AsF32()))/2 + valueL.Ascent.AsF32()
		if len(layer.value) > 0 {
			paintPromptLine(w, valueL, geometry.PointPixels(inner, baseline), textColor)
		} else if layer.placeholder != "" {
			ph := shapePromptText(w, layer.placeholder, 1.0)
			dim := textColor
			dim.A = 0.4
			paintPromptLine(w, ph, geometry.PointPixels(inner, baseline), dim)
		}
		// Caret.
		if layer.inputFocus && layer.blinkVisible {
			caretX := inner + valueL.XForIndex(layer.caret).AsF32()
			w.PaintQuad(solidQuadStyle(textColor), geometry.Bounds[geometry.Pixels]{
				Origin: geometry.PointPixels(caretX, rect.Origin.Y.AsF32()+rem*0.3),
				Size:   geometry.SizePixels(1.5, rect.Size.Height.AsF32()-rem*0.6),
			})
		}
	}

	// Buttons.
	for i, rect := range p.buttonRects {
		hovered := layer.hoverButton == i
		pressed := layer.pressButton == i
		accent := gc.Hsla{H: 210, S: 0.85, L: 0.55, A: 1}
		var bg gc.Hsla
		fg := textColor
		switch {
		case pressed:
			bg = accent
			bg.L -= 0.1
			fg = gc.Hsla{H: 0, S: 0, L: 1, A: 1}
		case hovered:
			bg = accent
			fg = gc.Hsla{H: 0, S: 0, L: 1, A: 1}
		default:
			bg = colors.Background.Hsla()
			bg.A = 0.9
		}
		bst := solidQuadStyle(bg)
		bst.CornerRadii = promptCorners(geometry.Px(rem * 0.3))
		bst.BorderColor = &border
		bst.BorderWidths = promptEdges(geometry.Px(1))
		w.PaintQuad(bst, rect)

		lbl := shapePromptText(w, layer.actions[i].Label, 0.95)
		lx := rect.Origin.X.AsF32() + (rect.Size.Width.AsF32()-lbl.Width.AsF32())/2
		lx = promptNumberBadge(w, lx, rect, i, textColor, rem)
		paintPromptLine(w, lbl, geometry.PointPixels(lx, rect.Origin.Y.AsF32()+(rect.Size.Height.AsF32()-(lbl.Ascent.AsF32()+lbl.Descent.AsF32()))/2+lbl.Ascent.AsF32()), fg)
	}
}

// promptNumberBadge paints the small action index digit left of the label
// (upstream prompts annotate buttons with their number key) and returns
// the label x adjusted for the badge.
func promptNumberBadge(w *Window, labelX float32, rect geometry.Bounds[geometry.Pixels], index int, color gc.Hsla, rem float32) float32 {
	digit := string(rune('1' + index))
	dl := shapePromptText(w, digit, 0.8)
	dim := color
	dim.A = 0.5
	paintPromptLine(w, dl, geometry.PointPixels(rect.Origin.X.AsF32()+rem*0.35, rect.Origin.Y.AsF32()+(rect.Size.Height.AsF32()-(dl.Ascent.AsF32()+dl.Descent.AsF32()))/2+dl.Ascent.AsF32()), dim)
	return labelX + dl.Width.AsF32() + rem*0.15
}

// promptLineH returns a line's height plus leading.
func promptLineH(l *text.LineLayout, lead float32) float32 {
	return l.Ascent.AsF32() + l.Descent.AsF32() + lead
}

// paintPromptLine paints a shaped line at the given pen origin.
func paintPromptLine(w *Window, l *text.LineLayout, origin geometry.Point[geometry.Pixels], color gc.Hsla) {
	for _, run := range l.Runs {
		for _, g := range run.Glyphs {
			w.PaintGlyph(
				geometry.PointPixels(origin.X.AsF32()+g.Position.X.AsF32(), origin.Y.AsF32()+g.Position.Y.AsF32()),
				run.FontID, g.ID, l.FontSize, color)
		}
	}
}

// solidQuadStyle builds a solid-background style for PaintQuad.
func solidQuadStyle(bg gc.Hsla) style.Style {
	st := style.DefaultStyle()
	b := gc.SolidBackground(bg)
	st.Background = &b
	return st
}

// ---------------------------------------------------------------------------
// Overlay integration (upstream draw(): prompts render above the active
// drag and tooltip)
// ---------------------------------------------------------------------------

// prepaintPrompts lays out and prepaints every open prompt as overlay
// roots, above the drag/tooltip overlays (their hitboxes register last so
// they receive input first).
func (w *Window) prepaintPrompts() {
	for _, p := range w.prompts {
		p.hoverButton = -1
		p.inputHover = false
		el := &promptDialog{layer: p}
		p.element = el
		node := w.requestLayoutElement(el)
		content := w.plat.ContentSize()
		w.layoutTree.ComputeLayout(layout.Node(node), layout.DefiniteAvailable(content.Width.AsF32(), content.Height.AsF32()))
		p.node = node
		p.valid = true
		// The dialog spans the whole window; no extra offset.
		w.PrepaintElementAt(el, node, geometry.Point[geometry.Pixels]{})
	}
}

// paintPrompts paints every open prompt in a top layer, above the drag
// view and tooltip.
func (w *Window) paintPrompts() {
	for _, p := range w.prompts {
		if !p.valid || p.element == nil {
			continue
		}
		close := w.PaintLayer()
		w.WithContentMask(geometry.Bounds[geometry.Pixels]{
			Origin: geometry.Point[geometry.Pixels]{},
			Size:   w.plat.ContentSize(),
		}, func() {
			w.PaintElement(p.element)
		})
		close()
	}
}
