// Package gestures is the portable touch-gesture recognizer, a port of
// upstream gpui `src/gestures.rs`.
//
// GPUI recognizes gestures from raw platform.TouchEvents using this
// portable recognizer. Taps synthesize mouse presses and releases so
// existing click listeners and text selection work on mobile. Pans produce
// platform.ScrollEvents, including post-release momentum. Long presses and
// touch drags can be claimed by element listeners.
package gestures

import (
	"math"
	"time"

	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/keymap"
	"github.com/FlameInTheDark/go-gpui/platform"
)

// ScrollEventSeparation delimits scroll gestures for platforms that only
// emit TouchPhaseMoved (upstream SCROLL_EVENT_SEPARATION).
const ScrollEventSeparation = 28 * time.Millisecond

// Axis is the dominant movement axis of a gesture.
type Axis uint8

const (
	// AxisVertical means movement is predominantly up/down.
	AxisVertical Axis = 0
	// AxisHorizontal means movement is predominantly left/right.
	AxisHorizontal Axis = 1
)

func dominantAxis(dx, dy geometry.Pixels) Axis {
	if pixelsAbs(dx) <= pixelsAbs(dy) {
		return AxisVertical
	}
	return AxisHorizontal
}

func pixelsAbs(v geometry.Pixels) float32 {
	if v < 0 {
		return float32(-v)
	}
	return float32(v)
}

func lockDeltaToAxis(dx, dy *geometry.Pixels, axis Axis) {
	if axis == AxisVertical {
		*dx = 0
	} else {
		*dy = 0
	}
}

func movementsOppose(lx, ly, rx, ry geometry.Pixels) bool {
	return float32(lx)*float32(rx)+float32(ly)*float32(ry) < 0
}

// OngoingScroll tracks the dominant axis across the events in a scroll
// gesture (upstream gestures::OngoingScroll).
type OngoingScroll struct {
	hasLastEvent bool
	lastEvent    time.Time
	axis         *Axis
}

// Filter filters the given delta to the dominant axis of the current
// scroll gesture. Gestures are delimited by their touch phase when
// available, with a timeout fallback for platforms that only emit
// TouchPhaseMoved.
func (o *OngoingScroll) Filter(dx, dy *geometry.Pixels, touchPhase platform.TouchPhase) {
	o.FilterAt(dx, dy, touchPhase, time.Now())
}

// FilterAt is Filter with an injectable clock (used by tests).
func (o *OngoingScroll) FilterAt(dx, dy *geometry.Pixels, touchPhase platform.TouchPhase, now time.Time) {
	const unlockPercent = 1.9
	const unlockLowerBound = float32(6)

	if touchPhase == platform.TouchPhaseEnded || touchPhase == platform.TouchPhaseCancelled {
		o.hasLastEvent = false
		o.axis = nil
		return
	}

	x := pixelsAbs(*dx)
	y := pixelsAbs(*dy)
	if x == 0 && y == 0 {
		if touchPhase == platform.TouchPhaseStarted {
			o.hasLastEvent = false
			o.axis = nil
		}
		return
	}

	startsNewGesture := touchPhase == platform.TouchPhaseStarted ||
		!o.hasLastEvent || now.Sub(o.lastEvent) >= ScrollEventSeparation
	var axis *Axis
	if o.axis != nil {
		a := *o.axis
		axis = &a
	}
	if startsNewGesture {
		a := dominantAxis(*dx, *dy)
		axis = &a
	} else if max32(x, y) >= unlockLowerBound {
		if axis != nil {
			if *axis == AxisVertical && x > y && x >= y*unlockPercent {
				axis = nil
			} else if *axis == AxisHorizontal && y > x && y >= x*unlockPercent {
				axis = nil
			}
		}
	}

	o.hasLastEvent = true
	o.lastEvent = now
	o.axis = axis
	if axis != nil {
		lockDeltaToAxis(dx, dy, *axis)
	}
}

func max32(a, b float32) float32 {
	if a >= b {
		return a
	}
	return b
}

// GestureTuning holds the feel constants consumed by gesture recognizers
// (upstream gestures::GestureTuning), provided best-effort per platform.
type GestureTuning struct {
	// TouchSlop is the distance a touch may travel before it stops being a
	// potential tap and becomes a pan/drag.
	TouchSlop geometry.Pixels
	// MultiTapInterval is the maximum interval between taps for them to
	// accumulate a tap count.
	MultiTapInterval time.Duration
	// MultiTapSlop is the maximum distance between taps for them to
	// accumulate a tap count.
	MultiTapSlop geometry.Pixels
	// LongPressDuration is how long a touch must remain within TouchSlop
	// to be recognized as a long press.
	LongPressDuration time.Duration
	// ScrollPhysics is how scroll momentum decelerates after a fling.
	ScrollPhysics ScrollPhysics
	// MinFlingVelocity is the minimum release velocity, in pixels per
	// second, required to start scroll momentum.
	MinFlingVelocity float32
}

// DefaultGestureTuning returns GPUI's own (iOS flavored) defaults.
func DefaultGestureTuning() GestureTuning {
	return GestureTuning{
		TouchSlop:         8,
		MultiTapInterval:  400 * time.Millisecond,
		MultiTapSlop:      16,
		LongPressDuration: 500 * time.Millisecond,
		ScrollPhysics:     IOSScrollPhysics(),
		MinFlingVelocity:  50,
	}
}

// ScrollPhysics models how free scrolling decelerates after a fling
// (upstream gestures::ScrollPhysics). Only deceleration: boundary behavior
// is the scroll container's policy.
type ScrollPhysics struct {
	kind       uint8 // 0 exponential, 1 friction spline
	decayPerMs float32
	friction   float32
	pxPerInch  float32
}

// ExponentialScrollPhysics is the UIScrollView model:
// velocity(t) = v0 * decayPerMs^ms.
func ExponentialScrollPhysics(decayPerMs float32) ScrollPhysics {
	return ScrollPhysics{kind: 0, decayPerMs: decayPerMs}
}

// IOSScrollPhysics is UIScrollView's normal deceleration rate (0.998).
func IOSScrollPhysics() ScrollPhysics {
	return ExponentialScrollPhysics(0.998)
}

// FrictionSplineScrollPhysics is the friction spline of Android's
// OverScroller: fling duration and distance follow a logarithmic
// deceleration law, and progress along the fling follows a cubic-Bezier
// ease-out curve. Transcribed from AOSP's SplineOverScroller (Apache-2.0).
func FrictionSplineScrollPhysics(friction, pixelsPerInch float32) ScrollPhysics {
	return ScrollPhysics{kind: 1, friction: friction, pxPerInch: pixelsPerInch}
}

// AndroidScrollPhysics is Android's OverScroller with stock friction at
// Android's nominal density of 160 dp/inch.
func AndroidScrollPhysics() ScrollPhysics {
	return FrictionSplineScrollPhysics(0.015, 160)
}

func (p ScrollPhysics) flingDuration(speed float32) time.Duration {
	switch p.kind {
	case 0:
		if speed <= momentumStopVelocity {
			return 0
		}
		milliseconds := math.Log(float64(momentumStopVelocity/speed)) / math.Log(float64(p.decayPerMs))
		return time.Duration(milliseconds / 1000 * float64(time.Second))
	default:
		if speed <= 0 {
			return 0
		}
		deceleration := splineDeceleration(speed, p.friction, p.pxPerInch)
		seconds := math.Exp(deceleration / (splineDecelerationRate() - 1))
		return time.Duration(seconds * float64(time.Second))
	}
}

// FlingDuration is how long a fling released at speed px/s coasts.
func (p ScrollPhysics) FlingDuration(speed float32) time.Duration {
	return p.flingDuration(speed)
}

// FlingDistance is the distance traveled `elapsed` into a fling released
// at `speed` pixels per second, in pixels along the fling direction.
// Evaluated in closed form so the trajectory is independent of tick timing.
func (p ScrollPhysics) FlingDistance(speed float32, elapsed time.Duration) float32 {
	duration := p.flingDuration(speed)
	if duration == 0 {
		return 0
	}
	if elapsed > duration {
		elapsed = duration
	}
	switch p.kind {
	case 0:
		// Integral of v0*k^ms dms, with speed converted to px/ms.
		milliseconds := float32(elapsed.Seconds()) * 1000
		return (speed / 1000) * (pow32(p.decayPerMs, milliseconds) - 1) / float32(math.Log(float64(p.decayPerMs)))
	default:
		deceleration := splineDeceleration(speed, p.friction, p.pxPerInch)
		rate := splineDecelerationRate()
		totalDistance := float64(p.friction) * splinePhysicalCoefficient(p.pxPerInch) *
			math.Exp(rate/(rate-1)*deceleration)
		progress := elapsed.Seconds() / duration.Seconds()
		return float32(totalDistance) * splineDistanceCoefficient(float32(progress))
	}
}

func pow32(base, exp float32) float32 {
	return float32(math.Pow(float64(base), float64(exp)))
}

// Android's SplineOverScroller friction model, transcribed from AOSP
// (Apache-2.0). SPLINE_TIME (used for programmatic scroll animations, not
// flings) is intentionally not transcribed.
const (
	splineNBSamples  = 100
	splineInflexion  = 0.35
	splineStartTense = 0.5
	splineEndTense   = 1.0
)

var (
	splineP1 = float32(splineStartTense * splineInflexion)
	splineP2 = float32(1.0 - splineEndTense*(1.0-splineInflexion))
)

// splineDecelerationRate is Android's DECELERATION_RATE:
// ln(0.78) / ln(0.9).
func splineDecelerationRate() float64 { return math.Log(0.78) / math.Log(0.9) }

// splinePositionTable is AOSP's SPLINE_POSITION: fractional fling
// distance sampled at 100 evenly spaced fractions of the fling duration,
// from a cubic Bezier with control points shaped by INFLEXION and the
// start/end tensions.
var splinePositionTable = func() [splineNBSamples + 1]float32 {
	var table [splineNBSamples + 1]float32
	xMin := float32(0)
	for i := 0; i < splineNBSamples; i++ {
		alpha := float32(i) / splineNBSamples
		xMax := float32(1)
		var x, coefficient float32
		for {
			x = xMin + (xMax-xMin)/2
			coefficient = 3 * x * (1 - x)
			t := coefficient*((1-x)*splineP1+x*splineP2) + x*x*x
			if abs32(t-alpha) < 1e-5 {
				break
			}
			if t > alpha {
				xMax = x
			} else {
				xMin = x
			}
		}
		table[i] = coefficient*((1-x)*splineStartTense+x) + x*x*x
	}
	table[splineNBSamples] = 1
	return table
}()

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// splinePhysicalCoefficient is SensorManager.GRAVITY_EARTH * 39.37 in/m *
// ppi * 0.84, AOSP's mPhysicalCoeff: gravity in pixels times an empirical
// "look and feel" tuning factor.
func splinePhysicalCoefficient(pixelsPerInch float32) float64 {
	return 9.80665 * 39.37 * float64(pixelsPerInch) * 0.84
}

// splineDeceleration is AOSP's getSplineDeceleration.
func splineDeceleration(speed, friction, pixelsPerInch float32) float64 {
	return math.Log(splineInflexion * float64(speed) / (float64(friction) * splinePhysicalCoefficient(pixelsPerInch)))
}

// splineDistanceCoefficient is the fraction of the total fling distance
// covered at fraction `t` of the fling duration: table lookup plus linear
// interpolation, as in SplineOverScroller.update.
func splineDistanceCoefficient(t float32) float32 {
	if t >= 1 {
		return 1
	}
	index := int(float32(splineNBSamples) * t)
	if index > splineNBSamples-1 {
		index = splineNBSamples - 1
	}
	timeLower := float32(index) / splineNBSamples
	timeUpper := float32(index+1) / splineNBSamples
	distanceLower := splinePositionTable[index]
	distanceUpper := splinePositionTable[index+1]
	velocityCoefficient := (distanceUpper - distanceLower) / (timeUpper - timeLower)
	return distanceLower + (t-timeLower)*velocityCoefficient
}

// GestureKinds is the set of gesture kinds that participate in
// recognition (upstream gestures::GestureKinds). Used to declare which
// gestures a platform recognizes natively rather than leaving to gpui
// core's portable recognizers.
type GestureKinds struct {
	Tap       bool
	LongPress bool
	Pan       bool
	Pinch     bool
}

// NoGestureKinds: gpui core's portable recognizers handle everything.
func NoGestureKinds() GestureKinds { return GestureKinds{} }

// AllGestureKinds enables every gesture kind.
func AllGestureKinds() GestureKinds {
	return GestureKinds{Tap: true, LongPress: true, Pan: true, Pinch: true}
}

const (
	// maxFlingVelocity is the ceiling on recognized fling velocity, in
	// pixels per second (matches Flutter's kMaxFlingVelocity).
	maxFlingVelocity = 8000.0
	// momentumStopVelocity is the speed, in px/s, below which momentum is
	// imperceptible; the exponential model treats reaching it as the end
	// of the fling.
	momentumStopVelocity = 10.0
	// velocityWindow is how far back the release-velocity estimate looks.
	velocityWindow = 100 * time.Millisecond
	// velocityAssumeStoppedGap: a pause between samples longer than this
	// means the finger stopped (Flutter's
	// kAssumePointerMoveStoppedMilliseconds).
	velocityAssumeStoppedGap = 40 * time.Millisecond
	velocityMaxSamples       = 20
)

// RecognizedGesture is a semantic event recognized from raw touches,
// ready to dispatch through the window's existing input paths (upstream
// RecognizedTouchGesture). Exactly one of the event pointers is set.
type RecognizedGesture struct {
	// Scroll is non-nil for one step of a pan (or its post-release
	// momentum), delivered to scroll listeners at the pan's start.
	Scroll *platform.ScrollEvent
	// TapDown/TapUp synthesize a recognized tap as a mouse press and
	// release (both set together).
	TapDown *platform.MouseDownEvent
	TapUp   *platform.MouseUpEvent
	// TouchDrag is a claimed touch-drag phase.
	TouchDrag *platform.TouchDragEvent
	// LongPress is a claimed long-press phase.
	LongPress *platform.LongPressEvent
}

type touchState uint8

const (
	stateIdle touchState = iota
	statePending
	statePanning
	stateCaught
	stateLongPressing
	stateTouchDragging
)

type activeTouch struct {
	id              platform.TouchID
	startPosition   geometry.Point[geometry.Pixels]
	lastPosition    geometry.Point[geometry.Pixels]
	emittedPosition geometry.Point[geometry.Pixels]
	lastMovementX   geometry.Pixels
	lastMovementY   geometry.Pixels
	velocity        velocityTracker
}

type completedTap struct {
	position geometry.Point[geometry.Pixels]
	time     time.Time
	count    int
}

// momentumState is one fling in progress: a closed-form curve of elapsed
// time — each tick evaluates it and emits the increment — so the fling is
// exactly frame-rate independent.
type momentumState struct {
	position        geometry.Point[geometry.Pixels]
	directionX      float32
	directionY      float32
	speed           float32
	startedAt       time.Time
	duration        time.Duration
	emittedDistance float32
}

// TouchGestureRecognizer is the portable recognizer behind raw touch
// input: it watches the TouchEvent stream for one touch at a time and
// resolves it into either a tap or a pan, following the competition model
// described in the package docs. Pans continue into post-release momentum
// when the touch lifts at speed; the window drives that phase through
// TickMomentum (upstream TouchGestureRecognizer).
//
// Taps are surfaced as synthesized mouse presses, which keeps every
// existing mouse-driven behavior (click listeners, caret placement,
// double-tap selection) working. Pinch recognition is not implemented yet
// (upstream parity), and additional touches are ignored while one is being
// recognized.
type TouchGestureRecognizer struct {
	tuning GestureTuning
	state  touchState
	// the touch being tracked in every non-idle state
	touch *activeTouch
	// pending-state fields (statePending)
	deadline         time.Time
	longPressOffered bool
	touchDragOffered bool
	// panning-state field (statePanning)
	panAxis Axis

	momentumActive bool
	momentum       momentumState
	lastTap        *completedTap
}

// NewTouchGestureRecognizer creates a recognizer with the given tuning.
func NewTouchGestureRecognizer(tuning GestureTuning) *TouchGestureRecognizer {
	return &TouchGestureRecognizer{tuning: tuning, state: stateIdle}
}

func scrollEventAt(position geometry.Point[geometry.Pixels], dx, dy geometry.Pixels, touchPhase platform.TouchPhase) platform.ScrollEvent {
	return platform.ScrollEvent{
		Position:   position,
		Delta:      platform.ScrollDeltaPixels(dx, dy),
		Modifiers:  keymap.Modifiers{},
		TouchPhase: touchPhase,
		Source:     platform.ScrollSourceTouch,
	}
}

// HandleEvent feeds one raw touch event, returning recognized gestures
// (upstream handle_event).
func (r *TouchGestureRecognizer) HandleEvent(event platform.TouchEvent) []RecognizedGesture {
	return r.HandleEventAt(event, time.Now())
}

// HandleEventAt is HandleEvent with an injectable clock.
func (r *TouchGestureRecognizer) HandleEventAt(event platform.TouchEvent, now time.Time) []RecognizedGesture {
	var recognized []RecognizedGesture
	switch event.Phase {
	case platform.TouchPhaseStarted:
		caughtFling := false
		if r.momentumActive {
			m := r.momentum
			r.momentumActive = false
			rec := scrollEventAt(m.position, 0, 0, platform.TouchPhaseEnded)
			recognized = append(recognized, RecognizedGesture{Scroll: &rec})
			caughtFling = true
		}
		if r.state == stateIdle {
			touch := activeTouch{
				id:              event.ID,
				startPosition:   event.Position,
				lastPosition:    event.Position,
				emittedPosition: event.Position,
			}
			touch.velocity.push(now, event.Position)
			if caughtFling {
				// A touch that catches a fling is a drag from the first
				// pixel: waiting out the slop would freeze the content
				// mid-scroll and then jump. It can also never be a tap;
				// releasing it just leaves the content stopped. Which way
				// it pans is its own to decide.
				rec := scrollEventAt(touch.startPosition, 0, 0, platform.TouchPhaseStarted)
				recognized = append(recognized, RecognizedGesture{Scroll: &rec})
				r.state = stateCaught
				r.touch = &touch
			} else {
				r.state = statePending
				r.touch = &touch
				r.deadline = now.Add(r.tuning.LongPressDuration)
				r.longPressOffered = false
				r.touchDragOffered = false
			}
		}
	case platform.TouchPhaseMoved:
		touch := r.touch
		switch r.state {
		case statePending:
			if touch == nil || touch.id != event.ID {
				return recognized
			}
			touch.velocity.push(now, event.Position)
			touch.lastPosition = event.Position
			accX, accY := event.Position.X-touch.startPosition.X, event.Position.Y-touch.startPosition.Y
			if magnitudeF64(accX, accY) > float64(r.tuning.TouchSlop) {
				// Carry the full movement so far into the first scroll
				// step: the content catches up to the finger instead of
				// losing the slop distance.
				target := event.Position
				if event.PredictedPosition != nil {
					target = *event.PredictedPosition
				}
				axis := dominantAxis(accX, accY)
				dx, dy := target.X-touch.startPosition.X, target.Y-touch.startPosition.Y
				lockDeltaToAxis(&dx, &dy, axis)
				touch.lastMovementX, touch.lastMovementY = accX, accY
				lockDeltaToAxis(&touch.lastMovementX, &touch.lastMovementY, axis)
				if movementsOppose(dx, dy, touch.lastMovementX, touch.lastMovementY) {
					target = event.Position
					dx, dy = accX, accY
					lockDeltaToAxis(&dx, &dy, axis)
				}
				touch.emittedPosition = target
				rec := scrollEventAt(touch.startPosition, dx, dy, platform.TouchPhaseStarted)
				recognized = append(recognized, RecognizedGesture{Scroll: &rec})
				r.state = statePanning
				r.panAxis = axis
			}
		case stateCaught:
			if touch == nil || touch.id != event.ID {
				return recognized
			}
			accX, accY := event.Position.X-touch.startPosition.X, event.Position.Y-touch.startPosition.Y
			if accX == 0 && accY == 0 {
				return recognized
			}
			// The first movement picks the axis; from here on this is an
			// ordinary pan, so let the pan arm emit it.
			r.state = statePanning
			r.panAxis = dominantAxis(accX, accY)
			return r.HandleEventAt(event, now)
		case statePanning:
			if touch == nil || touch.id != event.ID {
				return recognized
			}
			rawDX, rawDY := event.Position.X-touch.lastPosition.X, event.Position.Y-touch.lastPosition.Y
			lockDeltaToAxis(&rawDX, &rawDY, r.panAxis)
			if rawDX != 0 || rawDY != 0 {
				touch.lastMovementX, touch.lastMovementY = rawDX, rawDY
			}
			touch.velocity.push(now, event.Position)
			touch.lastPosition = event.Position
			target := event.Position
			if event.PredictedPosition != nil {
				target = *event.PredictedPosition
			}
			dx, dy := target.X-touch.emittedPosition.X, target.Y-touch.emittedPosition.Y
			lockDeltaToAxis(&dx, &dy, r.panAxis)
			// Prediction error must not reverse content while the raw
			// touch still advances. Fall back to the raw position so a
			// real finger reversal remains responsive.
			if movementsOppose(dx, dy, touch.lastMovementX, touch.lastMovementY) {
				target = event.Position
				dx, dy = target.X-touch.emittedPosition.X, target.Y-touch.emittedPosition.Y
				lockDeltaToAxis(&dx, &dy, r.panAxis)
				if movementsOppose(dx, dy, touch.lastMovementX, touch.lastMovementY) {
					dx, dy = 0, 0
				}
			}
			touch.emittedPosition = target
			rec := scrollEventAt(touch.startPosition, dx, dy, platform.TouchPhaseMoved)
			recognized = append(recognized, RecognizedGesture{Scroll: &rec})
		case stateLongPressing:
			if touch == nil || touch.id != event.ID {
				return recognized
			}
			touch.lastPosition = event.Position
			rec := platform.LongPressEvent{
				Phase:         platform.TouchPhaseMoved,
				StartPosition: touch.startPosition,
				Position:      event.Position,
			}
			recognized = append(recognized, RecognizedGesture{LongPress: &rec})
		case stateTouchDragging:
			if touch == nil || touch.id != event.ID {
				return recognized
			}
			touch.lastPosition = event.Position
			rec := platform.TouchDragEvent{
				Phase:         platform.TouchPhaseMoved,
				StartPosition: touch.startPosition,
				Position:      event.Position,
			}
			recognized = append(recognized, RecognizedGesture{TouchDrag: &rec})
		}
	case platform.TouchPhaseEnded:
		touch := r.touch
		switch r.state {
		case statePending:
			if touch == nil || touch.id != event.ID {
				return recognized
			}
			r.state = stateIdle
			r.touch = nil
			tapCount := 1
			if r.lastTap != nil &&
				now.Sub(r.lastTap.time) <= r.tuning.MultiTapInterval &&
				magnitudeF64(event.Position.X-r.lastTap.position.X, event.Position.Y-r.lastTap.position.Y) <= float64(r.tuning.MultiTapSlop) {
				tapCount = r.lastTap.count + 1
			}
			r.lastTap = &completedTap{position: event.Position, time: now, count: tapCount}
			down := platform.MouseDownEvent{
				Button:     platform.ButtonLeft,
				Position:   event.Position,
				ClickCount: tapCount,
			}
			up := platform.MouseUpEvent{
				Button:     platform.ButtonLeft,
				Position:   event.Position,
				ClickCount: tapCount,
			}
			recognized = append(recognized, RecognizedGesture{TapDown: &down, TapUp: &up})
		case stateCaught:
			if touch == nil || touch.id != event.ID {
				return recognized
			}
			r.state = stateIdle
			r.touch = nil
			// Never moved: the content it stopped stays where it is.
			rec := scrollEventAt(touch.startPosition, 0, 0, platform.TouchPhaseEnded)
			recognized = append(recognized, RecognizedGesture{Scroll: &rec})
		case statePanning:
			if touch == nil || touch.id != event.ID {
				return recognized
			}
			r.state = stateIdle
			r.touch = nil
			// The release deliberately contributes no velocity sample:
			// it usually repeats the last movement's position with a later
			// timestamp, which would dilute the estimate. But a release
			// long after the last movement means the finger had already
			// stopped, so nothing flings.
			fingerStopped := true
			if latest, ok := touch.velocity.latestSampleTime(); ok {
				fingerStopped = now.Sub(latest) > velocityAssumeStoppedGap
			}
			var vx, vy float32
			if !fingerStopped {
				vx, vy = touch.velocity.velocity()
			}
			if r.panAxis == AxisVertical {
				vx = 0
			} else {
				vy = 0
			}
			speed := float32(math.Sqrt(float64(vx*vx + vy*vy)))
			releaseDX, releaseDY := event.Position.X-touch.emittedPosition.X, event.Position.Y-touch.emittedPosition.Y
			lockDeltaToAxis(&releaseDX, &releaseDY, r.panAxis)
			if speed >= r.tuning.MinFlingVelocity {
				dirX, dirY := vx/speed, vy/speed
				if speed > maxFlingVelocity {
					speed = maxFlingVelocity
				}
				duration := r.tuning.ScrollPhysics.flingDuration(speed)
				if duration > 0 {
					totalDistance := r.tuning.ScrollPhysics.FlingDistance(speed, duration)
					// Prediction may have left the content ahead of the
					// raw release position. Emitting that correction here
					// would visibly snap the content backwards just as the
					// fling launches, so fold it into the fling instead:
					// start the curve already advanced by the overshoot,
					// keeping the total travel exact while monotonic.
					overshoot := -(float32(releaseDX)*dirX + float32(releaseDY)*dirY)
					emittedDistance := float32(0)
					if overshoot > 0 && overshoot < totalDistance {
						releaseDX += geometry.Pixels(dirX * overshoot)
						releaseDY += geometry.Pixels(dirY * overshoot)
						emittedDistance = overshoot
					}
					r.momentum = momentumState{
						position:        touch.startPosition,
						directionX:      dirX,
						directionY:      dirY,
						speed:           speed,
						startedAt:       now,
						duration:        duration,
						emittedDistance: emittedDistance,
					}
					r.momentumActive = true
				}
			}
			rec := scrollEventAt(touch.startPosition, releaseDX, releaseDY, platform.TouchPhaseEnded)
			recognized = append(recognized, RecognizedGesture{Scroll: &rec})
		case stateLongPressing:
			if touch == nil || touch.id != event.ID {
				return recognized
			}
			r.state = stateIdle
			r.touch = nil
			rec := platform.LongPressEvent{
				Phase:         platform.TouchPhaseEnded,
				StartPosition: touch.startPosition,
				Position:      event.Position,
			}
			recognized = append(recognized, RecognizedGesture{LongPress: &rec})
		case stateTouchDragging:
			if touch == nil || touch.id != event.ID {
				return recognized
			}
			r.state = stateIdle
			r.touch = nil
			rec := platform.TouchDragEvent{
				Phase:         platform.TouchPhaseEnded,
				StartPosition: touch.startPosition,
				Position:      event.Position,
			}
			recognized = append(recognized, RecognizedGesture{TouchDrag: &rec})
		}
	case platform.TouchPhaseCancelled:
		touch := r.touch
		switch r.state {
		case statePending:
			if touch != nil && touch.id == event.ID {
				r.state = stateIdle
				r.touch = nil
			}
		case statePanning, stateCaught:
			if touch != nil && touch.id == event.ID {
				r.state = stateIdle
				r.touch = nil
				rec := scrollEventAt(touch.startPosition, 0, 0, platform.TouchPhaseCancelled)
				recognized = append(recognized, RecognizedGesture{Scroll: &rec})
			}
		case stateLongPressing:
			if touch != nil && touch.id == event.ID {
				r.state = stateIdle
				r.touch = nil
				rec := platform.LongPressEvent{
					Phase:         platform.TouchPhaseCancelled,
					StartPosition: touch.startPosition,
					Position:      event.Position,
				}
				recognized = append(recognized, RecognizedGesture{LongPress: &rec})
			}
		case stateTouchDragging:
			if touch != nil && touch.id == event.ID {
				r.state = stateIdle
				r.touch = nil
				rec := platform.TouchDragEvent{
					Phase:         platform.TouchPhaseCancelled,
					StartPosition: touch.startPosition,
					Position:      event.Position,
				}
				recognized = append(recognized, RecognizedGesture{TouchDrag: &rec})
			}
		}
	}
	return recognized
}

func magnitudeF64(x, y geometry.Pixels) float64 {
	return math.Hypot(float64(x), float64(y))
}

// PendingLongPress reports the touch id and remaining time of a pending
// long press that has not been offered yet (upstream pending_long_press).
func (r *TouchGestureRecognizer) PendingLongPress() (platform.TouchID, time.Duration, bool) {
	if r.state != statePending || r.longPressOffered || r.touch == nil {
		return 0, 0, false
	}
	remaining := time.Until(r.deadline)
	if remaining < 0 {
		remaining = 0
	}
	return r.touch.id, remaining, true
}

// OfferLongPress offers the pending long press to element listeners
// (upstream offer_long_press); one-shot per pending touch.
func (r *TouchGestureRecognizer) OfferLongPress(id platform.TouchID) *platform.LongPressEvent {
	if r.state != statePending || r.touch == nil || r.touch.id != id || r.longPressOffered {
		return nil
	}
	r.longPressOffered = true
	event := platform.LongPressEvent{
		Phase:         platform.TouchPhaseStarted,
		StartPosition: r.touch.startPosition,
		Position:      r.touch.lastPosition,
	}
	return &event
}

// ResolveLongPress transitions to the long-pressing state when claimed
// (upstream resolve_long_press).
func (r *TouchGestureRecognizer) ResolveLongPress(claimed bool) {
	if !claimed {
		return
	}
	if r.state == statePending && r.longPressOffered {
		r.state = stateLongPressing
	}
}

// OfferTouchDrag offers the pending touch drag to element listeners
// (upstream offer_touch_drag); one-shot per pending touch.
func (r *TouchGestureRecognizer) OfferTouchDrag(id platform.TouchID) *platform.TouchDragEvent {
	if r.state != statePending || r.touch == nil || r.touch.id != id || r.touchDragOffered {
		return nil
	}
	r.touchDragOffered = true
	event := platform.TouchDragEvent{
		Phase:         platform.TouchPhaseStarted,
		StartPosition: r.touch.startPosition,
		Position:      r.touch.lastPosition,
	}
	return &event
}

// ResolveTouchDrag transitions to the touch-dragging state when claimed
// (upstream resolve_touch_drag).
func (r *TouchGestureRecognizer) ResolveTouchDrag(claimed bool) {
	if !claimed {
		return
	}
	if r.state == statePending && r.touchDragOffered {
		r.state = stateTouchDragging
	}
}

// HasMomentum reports whether a fling momentum is in progress.
func (r *TouchGestureRecognizer) HasMomentum() bool { return r.momentumActive }

// TickMomentum advances post-fling momentum by one frame, returning the
// scroll step to dispatch, or nil when no momentum is in progress. The
// final step carries TouchPhaseEnded to close the synthetic scroll stream.
func (r *TouchGestureRecognizer) TickMomentum() *RecognizedGesture {
	return r.TickMomentumAt(time.Now())
}

// TickMomentumAt is TickMomentum with an injectable clock.
func (r *TouchGestureRecognizer) TickMomentumAt(now time.Time) *RecognizedGesture {
	if !r.momentumActive {
		return nil
	}
	m := &r.momentum
	elapsed := now.Sub(m.startedAt)
	distance := r.tuning.ScrollPhysics.FlingDistance(m.speed, elapsed)
	// Prediction overshoot can start momentum ahead of its curve. Hold
	// that position until the curve catches up instead of stepping back.
	step := distance - m.emittedDistance
	if step < 0 {
		step = 0
	}
	if distance > m.emittedDistance {
		m.emittedDistance = distance
	}
	dx := geometry.Pixels(m.directionX * step)
	dy := geometry.Pixels(m.directionY * step)
	phase := platform.TouchPhaseMoved
	if elapsed >= m.duration {
		r.momentumActive = false
		phase = platform.TouchPhaseEnded
	}
	rec := scrollEventAt(m.position, dx, dy, phase)
	return &RecognizedGesture{Scroll: &rec}
}

// velocityTracker estimates the velocity a touch had at its newest sample
// (upstream VelocityTracker).
type velocityTracker struct {
	samples []velocitySample
}

type velocitySample struct {
	time     time.Time
	position geometry.Point[geometry.Pixels]
}

func (t *velocityTracker) push(at time.Time, position geometry.Point[geometry.Pixels]) {
	t.samples = append(t.samples, velocitySample{at, position})
	if len(t.samples) > velocityMaxSamples {
		t.samples = t.samples[len(t.samples)-velocityMaxSamples:]
	}
}

func (t *velocityTracker) latestSampleTime() (time.Time, bool) {
	if len(t.samples) == 0 {
		return time.Time{}, false
	}
	return t.samples[len(t.samples)-1].time, true
}

// velocity is the velocity at the newest sample, in pixels per second.
//
// Fits a second-degree polynomial by least squares over the trailing
// velocityWindow and takes its derivative at the newest sample, like
// Flutter's VelocityTracker and Android's lsq2 strategy. An endpoint
// difference over the same window would report the window's average
// speed, which for a flick — still accelerating at lift-off — is roughly
// half the speed the finger actually had at release.
func (t *velocityTracker) velocity() (float32, float32) {
	if len(t.samples) == 0 {
		return 0, 0
	}
	newest := t.samples[len(t.samples)-1]
	var timesSeconds []float64
	var horizontal []float64
	var vertical []float64
	previousTime := newest.time
	for i := len(t.samples) - 1; i >= 0; i-- {
		s := t.samples[i]
		age := newest.time.Sub(s.time)
		if age > velocityWindow || previousTime.Sub(s.time) > velocityAssumeStoppedGap {
			break
		}
		previousTime = s.time
		timesSeconds = append(timesSeconds, -age.Seconds())
		horizontal = append(horizontal, float64(s.position.X))
		vertical = append(vertical, float64(s.position.Y))
	}

	endpointEstimate := func(values []float64) float32 {
		elapsed := -timesSeconds[len(timesSeconds)-1]
		if elapsed <= 2.220446049250313e-16 {
			return 0
		}
		return float32((values[0] - values[len(values)-1]) / elapsed)
	}
	if len(timesSeconds) < 3 {
		return endpointEstimate(horizontal), endpointEstimate(vertical)
	}
	vx, ok := quadraticVelocityAtNewest(timesSeconds, horizontal)
	if !ok {
		vx = float64(endpointEstimate(horizontal))
	}
	vy, ok := quadraticVelocityAtNewest(timesSeconds, vertical)
	if !ok {
		vy = float64(endpointEstimate(vertical))
	}
	return float32(vx), float32(vy)
}

// quadraticVelocityAtNewest is a least-squares fit of
// value = a0 + a1*t + a2*t^2 returning a1: the fitted curve's velocity at
// t = 0, which callers place at the newest sample. false when the samples
// are too degenerate to fit (all simultaneous, for example).
func quadraticVelocityAtNewest(times, values []float64) (float64, bool) {
	count := float64(len(times))
	var sumT1, sumT2, sumT3, sumT4 float64
	var sumV, sumVT, sumVT2 float64
	for i, t := range times {
		v := values[i]
		t2 := t * t
		sumT1 += t
		sumT2 += t2
		sumT3 += t2 * t
		sumT4 += t2 * t2
		sumV += v
		sumVT += v * t
		sumVT2 += v * t2
	}
	// Cramer's rule on the 3x3 normal equations, solved for the linear
	// coefficient only.
	determinant := count*(sumT2*sumT4-sumT3*sumT3) -
		sumT1*(sumT1*sumT4-sumT3*sumT2) +
		sumT2*(sumT1*sumT3-sumT2*sumT2)
	if math.Abs(determinant) < 1e-12 {
		return 0, false
	}
	linearDeterminant := count*(sumVT*sumT4-sumT3*sumVT2) -
		sumV*(sumT1*sumT4-sumT3*sumT2) +
		sumT2*(sumT1*sumVT2-sumVT*sumT2)
	return linearDeterminant / determinant, true
}
