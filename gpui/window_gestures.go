package gpui

import (
	"time"

	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gestures"
	"github.com/FlameInTheDark/go-gpui/platform"
)

// resolveGestureTuning converts the platform's registered tuning options
// (platform.SetGestureTuning) into the recognizer's feel constants,
// falling back to GPUI's own defaults (upstream GestureTuning::default()).
func resolveGestureTuning() gestures.GestureTuning {
	tuning := gestures.DefaultGestureTuning()
	opts := platform.GestureTuning()
	if opts == nil {
		return tuning
	}
	if opts.TouchSlop > 0 {
		tuning.TouchSlop = geometry.Pixels(opts.TouchSlop)
	}
	if opts.MultiTapInterval > 0 {
		tuning.MultiTapInterval = opts.MultiTapInterval
	}
	if opts.MultiTapSlop > 0 {
		tuning.MultiTapSlop = geometry.Pixels(opts.MultiTapSlop)
	}
	if opts.LongPressDuration > 0 {
		tuning.LongPressDuration = opts.LongPressDuration
	}
	switch opts.ScrollPhysicsKind {
	case "ios":
		if opts.DecayPerMs > 0 {
			tuning.ScrollPhysics = gestures.ExponentialScrollPhysics(opts.DecayPerMs)
		}
	case "android":
		tuning.ScrollPhysics = gestures.AndroidScrollPhysics()
	}
	if opts.MinFlingVelocity > 0 {
		tuning.MinFlingVelocity = opts.MinFlingVelocity
	}
	return tuning
}

// handleTouchEvent feeds a raw touch through the portable recognizer and
// dispatches the recognized gestures (upstream Window::dispatch_touch).
func (w *Window) handleTouchEvent(event platform.TouchEvent) platform.DispatchEventResult {
	recognized := w.touchGestures.HandleEvent(event)
	tapped := false
	for _, gesture := range recognized {
		if gesture.TapDown != nil {
			tapped = true
		}
		w.dispatchRecognizedGesture(gesture)
	}
	if event.Phase == platform.TouchPhaseStarted {
		w.scheduleLongPressTimer()
	} else if _, _, ok := w.touchGestures.PendingLongPress(); !ok {
		w.cancelLongPressTimer()
	}
	// The platform's touch-release handler may inspect the input handler
	// as soon as this dispatch returns. Input handlers are registered
	// during draw, so draw now to make them reflect any focus change the
	// tap just caused (upstream draws when a tap left the window dirty).
	if tapped && w.dirty && w.drawPhase == PhaseNone {
		w.Draw()
	}
	if w.touchGestures.HasMomentum() {
		w.scheduleTouchMomentumTick()
	}
	return DispatchEventResult{Propagate: true}
}

// dispatchRecognizedGesture routes one recognized gesture through the
// window's existing input paths (upstream
// dispatch_recognized_touch_gesture).
func (w *Window) dispatchRecognizedGesture(gesture gestures.RecognizedGesture) {
	switch {
	case gesture.Scroll != nil:
		w.mousePosition = gesture.Scroll.Position
		w.DispatchMouseEvent(*gesture.Scroll)
	case gesture.TapDown != nil && gesture.TapUp != nil:
		w.mousePosition = gesture.TapUp.Position
		w.lastInputKeyboard = false
		if w.pressedButtons == nil {
			w.pressedButtons = map[platform.MouseButton]bool{}
		}
		w.pressedButtons[gesture.TapDown.Button] = true
		w.DispatchMouseEvent(*gesture.TapDown)
		delete(w.pressedButtons, gesture.TapUp.Button)
		w.DispatchMouseEvent(*gesture.TapUp)
	case gesture.TouchDrag != nil:
		w.mousePosition = gesture.TouchDrag.StartPosition
		w.lastInputKeyboard = false
		w.defaultPrevented = false
		started := gesture.TouchDrag.Phase == platform.TouchPhaseStarted
		w.DispatchMouseEvent(*gesture.TouchDrag)
		if started {
			w.touchGestures.ResolveTouchDrag(w.defaultPrevented)
		}
	case gesture.LongPress != nil:
		if gesture.LongPress.Phase == platform.TouchPhaseStarted {
			w.mousePosition = gesture.LongPress.StartPosition
		} else {
			w.mousePosition = gesture.LongPress.Position
		}
		w.lastInputKeyboard = false
		w.defaultPrevented = false
		started := gesture.LongPress.Phase == platform.TouchPhaseStarted
		w.DispatchMouseEvent(*gesture.LongPress)
		if started {
			w.touchGestures.ResolveLongPress(w.defaultPrevented)
		}
	}
}

// scheduleLongPressTimer arms the pending long-press offer after a touch
// starts (upstream schedule_long_press_timer): after the long-press
// duration elapses, the pending touch is offered to element listeners.
func (w *Window) scheduleLongPressTimer() {
	w.cancelLongPressTimer()
	touchID, delay, ok := w.touchGestures.PendingLongPress()
	if !ok {
		return
	}
	w.longPressTimer = time.AfterFunc(delay, func() {
		w.dispatchOnMainThread(func() {
			w.longPressTimer = nil
			if event := w.touchGestures.OfferLongPress(touchID); event != nil {
				w.dispatchRecognizedGesture(gestures.RecognizedGesture{LongPress: event})
			}
		})
	})
}

func (w *Window) cancelLongPressTimer() {
	if w.longPressTimer != nil {
		w.longPressTimer.Stop()
		w.longPressTimer = nil
	}
}

// dispatchOnMainThread posts a closure onto the UI thread, matching the
// window's mainDispatch wiring (headless tests have none and run inline).
func (w *Window) dispatchOnMainThread(f func()) {
	if w.mainDispatch != nil {
		w.mainDispatch(f)
		return
	}
	f()
}

// scheduleTouchMomentumTick drives post-fling momentum one frame at a
// time (upstream schedule_touch_momentum_tick through on_next_frame).
func (w *Window) scheduleTouchMomentumTick() {
	w.nextFrameCallbacks = append(w.nextFrameCallbacks, func(w *Window) {
		if gesture := w.touchGestures.TickMomentum(); gesture != nil {
			w.dispatchRecognizedGesture(*gesture)
		}
		if w.touchGestures.HasMomentum() {
			w.scheduleTouchMomentumTick()
		}
	})
}
