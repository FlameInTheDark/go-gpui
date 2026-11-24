package gestures

import (
	"math"
	"testing"
	"time"

	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/platform"
)

func px(v float32) geometry.Pixels { return geometry.Pixels(v) }

func pt(x, y float32) geometry.Point[geometry.Pixels] {
	return geometry.Point[geometry.Pixels]{X: px(x), Y: px(y)}
}

func touchEvent(id platform.TouchID, phase platform.TouchPhase, x, y float32) platform.TouchEvent {
	return platform.TouchEvent{ID: id, Phase: phase, Position: pt(x, y)}
}

func TestOngoingScrollLocksToDominantAxis(t *testing.T) {
	now := time.Now()
	var ongoing OngoingScroll
	dx, dy := px(10), px(2)
	ongoing.FilterAt(&dx, &dy, platform.TouchPhaseStarted, now)
	if ongoing.axis == nil || *ongoing.axis != AxisHorizontal {
		t.Fatalf("expected horizontal axis, got %v", ongoing.axis)
	}
	if dx != 10 || dy != 0 {
		t.Fatalf("expected (10,0), got (%v,%v)", dx, dy)
	}

	dx, dy = px(3), px(2)
	ongoing.FilterAt(&dx, &dy, platform.TouchPhaseMoved, now.Add(1*time.Millisecond))
	if ongoing.axis == nil || *ongoing.axis != AxisHorizontal {
		t.Fatalf("expected horizontal axis to persist")
	}
	if dx != 3 || dy != 0 {
		t.Fatalf("expected (3,0), got (%v,%v)", dx, dy)
	}
}

func TestOngoingScrollUnlocksWhenDirectionChanges(t *testing.T) {
	now := time.Now()
	var ongoing OngoingScroll
	dx, dy := px(10), px(2)
	ongoing.FilterAt(&dx, &dy, platform.TouchPhaseStarted, now)

	dx, dy = px(2), px(10)
	ongoing.FilterAt(&dx, &dy, platform.TouchPhaseMoved, now.Add(1*time.Millisecond))
	if ongoing.axis != nil {
		t.Fatalf("expected unlocked axis, got %v", *ongoing.axis)
	}
	if dx != 2 || dy != 10 {
		t.Fatalf("expected (2,10), got (%v,%v)", dx, dy)
	}
}

func TestOngoingScrollStartsNewGestureAtTimeoutBoundary(t *testing.T) {
	now := time.Now()
	var ongoing OngoingScroll
	dx, dy := px(10), px(2)
	ongoing.FilterAt(&dx, &dy, platform.TouchPhaseMoved, now)

	dx, dy = px(2), px(10)
	ongoing.FilterAt(&dx, &dy, platform.TouchPhaseMoved, now.Add(ScrollEventSeparation))
	if ongoing.axis == nil || *ongoing.axis != AxisVertical {
		t.Fatalf("expected vertical axis")
	}
	if dx != 0 || dy != 10 {
		t.Fatalf("expected (0,10), got (%v,%v)", dx, dy)
	}
}

func TestOngoingScrollIgnoresZeroDeltaAndResetsWhenEnded(t *testing.T) {
	now := time.Now()
	var ongoing OngoingScroll
	dx, dy := px(10), px(2)
	ongoing.FilterAt(&dx, &dy, platform.TouchPhaseStarted, now)

	dx, dy = 0, 0
	ongoing.FilterAt(&dx, &dy, platform.TouchPhaseEnded, now.Add(1*time.Millisecond))
	if ongoing.axis != nil {
		t.Fatal("expected axis reset after end")
	}

	dx, dy = px(2), px(3)
	ongoing.FilterAt(&dx, &dy, platform.TouchPhaseMoved, now.Add(2*time.Millisecond))
	if ongoing.axis == nil || *ongoing.axis != AxisVertical {
		t.Fatal("expected vertical axis")
	}
	if dx != 0 || dy != 3 {
		t.Fatalf("expected (0,3), got (%v,%v)", dx, dy)
	}
}

func TestOngoingScrollSupportsMovedOnlyPlatforms(t *testing.T) {
	now := time.Now()
	var ongoing OngoingScroll
	dx, dy := px(10), px(2)
	ongoing.FilterAt(&dx, &dy, platform.TouchPhaseMoved, now)
	if ongoing.axis == nil || *ongoing.axis != AxisHorizontal {
		t.Fatal("expected horizontal axis")
	}
	if dx != 10 || dy != 0 {
		t.Fatalf("expected (10,0), got (%v,%v)", dx, dy)
	}
}

func TestTouchWithinSlopResolvesToTap(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()
	touch := platform.TouchID(1)

	if recognized := recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 10), now); len(recognized) != 0 {
		t.Fatalf("expected nothing recognized, got %v", recognized)
	}
	if recognized := recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 12, 11), now.Add(20*time.Millisecond)); len(recognized) != 0 {
		t.Fatalf("expected nothing recognized, got %v", recognized)
	}

	recognized := recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseEnded, 12, 11), now.Add(60*time.Millisecond))
	if len(recognized) != 1 || recognized[0].TapDown == nil || recognized[0].TapUp == nil {
		t.Fatalf("expected tap, got %v", recognized)
	}
	down, up := recognized[0].TapDown, recognized[0].TapUp
	if down.ClickCount != 1 {
		t.Fatalf("expected click count 1, got %d", down.ClickCount)
	}
	if down.Position != pt(12, 11) {
		t.Fatalf("wrong tap position %v", down.Position)
	}
	if up.ClickCount != 1 {
		t.Fatalf("expected click count 1, got %d", up.ClickCount)
	}
}

func TestConsecutiveTapsAccumulateTapCount(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()

	recognizer.HandleEventAt(touchEvent(1, platform.TouchPhaseStarted, 10, 10), now)
	recognizer.HandleEventAt(touchEvent(1, platform.TouchPhaseEnded, 10, 10), now.Add(40*time.Millisecond))

	secondDown := now.Add(200 * time.Millisecond)
	recognizer.HandleEventAt(touchEvent(2, platform.TouchPhaseStarted, 14, 10), secondDown)
	recognized := recognizer.HandleEventAt(touchEvent(2, platform.TouchPhaseEnded, 14, 10), secondDown.Add(40*time.Millisecond))
	if len(recognized) != 1 || recognized[0].TapDown == nil {
		t.Fatalf("expected tap, got %v", recognized)
	}
	if recognized[0].TapDown.ClickCount != 2 {
		t.Fatalf("expected accumulated click count 2, got %d", recognized[0].TapDown.ClickCount)
	}
}

func TestDistantTapsDoNotAccumulate(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()

	recognizer.HandleEventAt(touchEvent(1, platform.TouchPhaseStarted, 10, 10), now)
	recognizer.HandleEventAt(touchEvent(1, platform.TouchPhaseEnded, 10, 10), now.Add(40*time.Millisecond))

	secondDown := now.Add(200 * time.Millisecond)
	recognizer.HandleEventAt(touchEvent(2, platform.TouchPhaseStarted, 40, 10), secondDown)
	recognized := recognizer.HandleEventAt(touchEvent(2, platform.TouchPhaseEnded, 40, 10), secondDown.Add(40*time.Millisecond))
	if recognized[0].TapDown.ClickCount != 1 {
		t.Fatalf("expected click count 1, got %d", recognized[0].TapDown.ClickCount)
	}
}

func TestTouchBeyondSlopResolvesToPan(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()
	touch := platform.TouchID(1)

	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 10), now)
	recognized := recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 10, 30), now.Add(20*time.Millisecond))
	if len(recognized) != 1 || recognized[0].Scroll == nil {
		t.Fatalf("expected pan, got %v", recognized)
	}
	scroll := recognized[0].Scroll
	if scroll.TouchPhase != platform.TouchPhaseStarted {
		t.Fatalf("expected started phase, got %v", scroll.TouchPhase)
	}
	pd := scroll.Delta.PixelDelta(20)
	dx, dy := pd.X, pd.Y
	if dx != 0 || dy != 20 {
		t.Fatalf("expected (0,20) delta, got (%v,%v)", dx, dy)
	}

	recognized = recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 10, 50), now.Add(40*time.Millisecond))
	if len(recognized) != 1 || recognized[0].Scroll == nil {
		t.Fatalf("expected continued pan")
	}
	scroll = recognized[0].Scroll
	pd = scroll.Delta.PixelDelta(20)
	dx, dy = pd.X, pd.Y
	if dx != 0 || dy != 20 {
		t.Fatalf("expected (0,20) delta, got (%v,%v)", dx, dy)
	}
}

func TestTouchPanStaysLockedToItsInitialDominantAxis(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()
	touch := platform.TouchID(1)

	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 10), now)
	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 10, 25), now.Add(20*time.Millisecond))
	recognized := recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 25, 40), now.Add(40*time.Millisecond))
	scroll := recognized[0].Scroll
	pd := scroll.Delta.PixelDelta(20)
	dx, dy := pd.X, pd.Y
	if dx != 0 || dy != 15 {
		t.Fatalf("expected axis-locked (0,15), got (%v,%v)", dx, dy)
	}
}

func TestPanConvergesOnReleaseWithoutPrediction(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()
	touch := platform.TouchID(1)

	var total geometry.Pixels
	collect := func(recognized []RecognizedGesture) {
		for _, r := range recognized {
			if r.Scroll != nil {
				total += r.Scroll.Delta.PixelDelta(20).Y
			}
		}
	}

	collect(recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 10), now))
	collect(recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 10, 30), now.Add(20*time.Millisecond)))
	collect(recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 10, 60), now.Add(40*time.Millisecond)))
	collect(recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseEnded, 10, 60), now.Add(60*time.Millisecond)))
	if total != 50 {
		t.Fatalf("expected total 50px scrolled, got %v", total)
	}
}

func TestPredictedPositionsLeadThePanButTotalsConverge(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()
	touch := platform.TouchID(1)
	predicted := pt(10, 42)

	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 10), now)
	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 10, 30), now.Add(20*time.Millisecond))

	event := touchEvent(touch, platform.TouchPhaseMoved, 10, 40)
	event.PredictedPosition = &predicted
	recognized := recognizer.HandleEventAt(event, now.Add(40*time.Millisecond))
	scroll := recognized[0].Scroll
	dy := scroll.Delta.PixelDelta(20).Y
	if dy != 12 { // emitted to prediction (42) - already emitted (30)
		t.Fatalf("expected predicted lead of 12, got %v", dy)
	}

	recognized = recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseEnded, 10, 45), now.Add(60*time.Millisecond))
	scroll = recognized[len(recognized)-1].Scroll
	dy = scroll.Delta.PixelDelta(20).Y
	if dy != 3 { // converge to raw release position (45) - emitted (42)
		t.Fatalf("expected convergence delta 3, got %v", dy)
	}
}

func TestSlowReleaseDoesNotStartMomentum(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()
	touch := platform.TouchID(1)

	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 10), now)
	for i := 1; i <= 5; i++ {
		at := now.Add(time.Duration(i) * 100 * time.Millisecond)
		recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 10, float32(10+i*3)), at)
	}
	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseEnded, 10, 25), now.Add(600*time.Millisecond))
	if recognizer.HasMomentum() {
		t.Fatal("slow release must not start momentum")
	}
}

func TestFastReleaseStartsMomentumThatDecaysToAStop(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()
	touch := platform.TouchID(1)

	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 100), now)
	for i := 1; i <= 5; i++ {
		at := now.Add(time.Duration(i*20) * time.Millisecond)
		recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 10, float32(100-15*i)), at)
	}
	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseEnded, 10, 25), now.Add(120*time.Millisecond))
	if !recognizer.HasMomentum() {
		t.Fatal("fast release must start momentum")
	}

	var total geometry.Pixels
	lastPhase := platform.TouchPhaseMoved
	for i := 0; recognizer.HasMomentum() && i < 10000; i++ {
		step := recognizer.TickMomentumAt(now.Add(150 * time.Millisecond).Add(time.Duration(i) * 16 * time.Millisecond))
		if step == nil || step.Scroll == nil {
			break
		}
		total += step.Scroll.Delta.PixelDelta(20).Y
		lastPhase = step.Scroll.TouchPhase
	}
	if lastPhase != platform.TouchPhaseEnded {
		t.Fatalf("momentum must end with Ended phase, got %v", lastPhase)
	}
	if total >= 0 {
		t.Fatalf("momentum must continue upward (negative dy), got %v", total)
	}
	if recognizer.TickMomentumAt(now.Add(10*time.Second)) != nil {
		t.Fatal("finished momentum must tick nil")
	}
}

func TestNewTouchInterruptsMomentum(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()
	touch := platform.TouchID(1)

	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 100), now)
	for i := 1; i <= 5; i++ {
		at := now.Add(time.Duration(i*20) * time.Millisecond)
		recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 10, float32(100-15*i)), at)
	}
	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseEnded, 10, 25), now.Add(120*time.Millisecond))
	if !recognizer.HasMomentum() {
		t.Fatal("expected momentum")
	}

	recognized := recognizer.HandleEventAt(touchEvent(2, platform.TouchPhaseStarted, 50, 50), now.Add(150*time.Millisecond))
	if recognizer.HasMomentum() {
		t.Fatal("new touch must interrupt momentum")
	}
	var sawEnd bool
	for _, r := range recognized {
		if r.Scroll != nil && r.Scroll.TouchPhase == platform.TouchPhaseEnded {
			sawEnd = true
		}
	}
	if !sawEnd {
		t.Fatal("interrupted momentum must emit a terminal scroll event")
	}
}

func TestCatchingAFlingPansImmediatelyAndNeverTaps(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()

	recognizer.HandleEventAt(touchEvent(1, platform.TouchPhaseStarted, 100, 300), now)
	for step := 1; step <= 3; step++ {
		at := now.Add(time.Duration(step*16) * time.Millisecond)
		recognizer.HandleEventAt(touchEvent(1, platform.TouchPhaseMoved, 100, float32(300-step*33)), at)
	}
	recognizer.HandleEventAt(touchEvent(1, platform.TouchPhaseEnded, 100, 200), now.Add(64*time.Millisecond))
	if !recognizer.HasMomentum() {
		t.Fatal("expected momentum")
	}

	recognizer.HandleEventAt(touchEvent(2, platform.TouchPhaseStarted, 100, 200), now.Add(200*time.Millisecond))
	if recognizer.HasMomentum() {
		t.Fatal("catching touch must interrupt momentum")
	}

	// A movement well within the slop scrolls immediately.
	recognized := recognizer.HandleEventAt(touchEvent(2, platform.TouchPhaseMoved, 100, 197), now.Add(216*time.Millisecond))
	if len(recognized) != 1 || recognized[0].Scroll == nil {
		t.Fatalf("expected scroll, got %v", recognized)
	}
	scroll := recognized[0].Scroll
	if scroll.TouchPhase != platform.TouchPhaseMoved {
		t.Fatalf("expected moved phase, got %v", scroll.TouchPhase)
	}
	if dy := scroll.Delta.PixelDelta(16).Y; dy != -3 {
		t.Fatalf("expected (0,-3) delta, got %v", dy)
	}

	// Releasing the catch is not a tap.
	recognized = recognizer.HandleEventAt(touchEvent(2, platform.TouchPhaseEnded, 100, 197), now.Add(232*time.Millisecond))
	if len(recognized) != 1 || recognized[0].Scroll == nil || recognized[0].Scroll.TouchPhase != platform.TouchPhaseEnded {
		t.Fatalf("expected terminal scroll, got %v", recognized)
	}
	for _, r := range recognized {
		if r.TapDown != nil {
			t.Fatal("a touch that caught a fling must never tap")
		}
	}
}

func TestCancelledPanEmitsCancelledScrollAndNoTap(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()
	touch := platform.TouchID(1)

	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 10), now)
	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 10, 40), now.Add(20*time.Millisecond))
	recognized := recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseCancelled, 10, 40), now.Add(40*time.Millisecond))
	if len(recognized) != 1 || recognized[0].Scroll == nil || recognized[0].Scroll.TouchPhase != platform.TouchPhaseCancelled {
		t.Fatalf("expected cancelled scroll, got %v", recognized)
	}
}

func TestConcurrentTouchesAreIgnoredWhileOneIsActive(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()

	recognizer.HandleEventAt(touchEvent(1, platform.TouchPhaseStarted, 10, 10), now)
	// A second touch starting while the first is pending is ignored.
	if recognized := recognizer.HandleEventAt(touchEvent(2, platform.TouchPhaseStarted, 30, 30), now.Add(10*time.Millisecond)); len(recognized) != 0 {
		t.Fatalf("second touch must be ignored, got %v", recognized)
	}
	// Ending the second touch changes nothing.
	if recognized := recognizer.HandleEventAt(touchEvent(2, platform.TouchPhaseEnded, 30, 30), now.Add(20*time.Millisecond)); len(recognized) != 0 {
		t.Fatalf("second touch end must be ignored, got %v", recognized)
	}
	// The first still resolves to a tap.
	recognized := recognizer.HandleEventAt(touchEvent(1, platform.TouchPhaseEnded, 10, 12), now.Add(60*time.Millisecond))
	if len(recognized) != 1 || recognized[0].TapDown == nil {
		t.Fatalf("expected tap for first touch")
	}
}

func TestClaimedTouchDragEmitsPhasedStreamWithoutPanOrTap(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()
	touch := platform.TouchID(1)

	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 10), now)
	if event := recognizer.OfferTouchDrag(touch); event == nil || event.Phase != platform.TouchPhaseStarted {
		t.Fatalf("expected touch drag offer, got %v", event)
	}
	recognizer.ResolveTouchDrag(true)

	recognized := recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 30, 10), now.Add(20*time.Millisecond))
	if len(recognized) != 1 || recognized[0].TouchDrag == nil {
		t.Fatalf("expected touch drag move, got %v", recognized)
	}
	recognized = recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseEnded, 30, 10), now.Add(40*time.Millisecond))
	if len(recognized) != 1 || recognized[0].TouchDrag == nil || recognized[0].TouchDrag.Phase != platform.TouchPhaseEnded {
		t.Fatalf("expected touch drag end, got %v", recognized)
	}
	if recognized[0].TapDown != nil || recognized[0].Scroll != nil {
		t.Fatal("claimed drag must not produce tap or scroll")
	}
}

func TestUnclaimedTouchDragRemainsAPanCandidate(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()
	touch := platform.TouchID(1)

	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 10), now)
	if event := recognizer.OfferTouchDrag(touch); event == nil {
		t.Fatal("expected offer")
	}
	recognizer.ResolveTouchDrag(false)

	recognized := recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 10, 40), now.Add(20*time.Millisecond))
	if len(recognized) != 1 || recognized[0].Scroll == nil {
		t.Fatalf("expected pan after unclaimed drag, got %v", recognized)
	}
}

func TestClaimedLongPressEmitsPhasedStreamWithoutTap(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()
	touch := platform.TouchID(1)

	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 10), now)
	if event := recognizer.OfferLongPress(touch); event == nil || event.Phase != platform.TouchPhaseStarted {
		t.Fatalf("expected long press offer")
	}
	recognizer.ResolveLongPress(true)

	recognized := recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 12, 11), now.Add(20*time.Millisecond))
	if len(recognized) != 1 || recognized[0].LongPress == nil || recognized[0].LongPress.Phase != platform.TouchPhaseMoved {
		t.Fatalf("expected long press move, got %v", recognized)
	}
	recognized = recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseEnded, 12, 11), now.Add(40*time.Millisecond))
	if len(recognized) != 1 || recognized[0].TapDown != nil {
		t.Fatal("claimed long press must not tap")
	}
}

func TestUnclaimedLongPressRemainsATapCandidate(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()
	touch := platform.TouchID(1)

	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 10), now)
	recognizer.OfferLongPress(touch)
	recognizer.ResolveLongPress(false)

	recognized := recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseEnded, 12, 11), now.Add(40*time.Millisecond))
	if len(recognized) != 1 || recognized[0].TapDown == nil {
		t.Fatal("unclaimed long press must remain a tap candidate")
	}
}

func TestLongPressOfferIsOneShot(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()
	touch := platform.TouchID(1)

	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 10), now)
	if recognizer.OfferLongPress(touch) == nil {
		t.Fatal("expected first offer")
	}
	if recognizer.OfferLongPress(touch) != nil {
		t.Fatal("offer must be one-shot")
	}
	if recognizer.OfferLongPress(99) != nil {
		t.Fatal("offer must be touch-specific")
	}
}

func TestSplinePositionTableMatchesTheBezierCurve(t *testing.T) {
	// The table must be a sampling of the bezier curve: for each table
	// entry, re-derive the parameter and check the curve's position.
	for i := 0; i <= splineNBSamples; i++ {
		alpha := float32(i) / splineNBSamples
		xMin, xMax := float32(0), float32(1)
		var x float32
		for iter := 0; iter < 100; iter++ {
			x = xMin + (xMax-xMin)/2
			coefficient := 3 * x * (1 - x)
			time := coefficient*((1-x)*splineP1+x*splineP2) + x*x*x
			if abs32(time-alpha) < 1e-5 {
				break
			}
			if time > alpha {
				xMax = x
			} else {
				xMin = x
			}
		}
		coefficient := 3 * x * (1 - x)
		expected := coefficient*((1-x)*splineStartTense+x) + x*x*x
		if abs32(splinePositionTable[i]-expected) > 1e-4 {
			t.Fatalf("spline table mismatch at %d: %v vs %v", i, splinePositionTable[i], expected)
		}
	}
}

func TestFlingCurvesAreSaneForBothPhysics(t *testing.T) {
	for _, physics := range []ScrollPhysics{IOSScrollPhysics(), AndroidScrollPhysics()} {
		for _, speed := range []float32{50, 500, 5000} {
			duration := physics.flingDuration(speed)
			if duration <= 0 {
				t.Fatalf("expected positive fling duration for %v px/s", speed)
			}
			total := physics.FlingDistance(speed, duration)
			if total <= 0 {
				t.Fatalf("expected positive fling distance for %v px/s", speed)
			}
			half := physics.FlingDistance(speed, duration/2)
			if half < 0 || half > total {
				t.Fatalf("fling distance must be monotonic: half=%v total=%v", half, total)
			}
		}
	}
}

func TestMomentumIsFrameRateIndependent(t *testing.T) {
	recognizer := NewTouchGestureRecognizer(DefaultGestureTuning())
	now := time.Now()
	touch := platform.TouchID(1)

	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 100), now)
	for i := 1; i <= 5; i++ {
		at := now.Add(time.Duration(i*20) * time.Millisecond)
		recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 10, float32(100-15*i)), at)
	}
	recognizer.HandleEventAt(touchEvent(touch, platform.TouchPhaseEnded, 10, 25), now.Add(120*time.Millisecond))
	if !recognizer.HasMomentum() {
		t.Fatal("expected momentum")
	}

	// Sample at 60Hz.
	var total60 geometry.Pixels
	for i := 0; recognizer.HasMomentum(); i++ {
		step := recognizer.TickMomentumAt(now.Add(150 * time.Millisecond).Add(time.Duration(i) * (1000 / 60) * time.Millisecond))
		if step == nil {
			break
		}
		dy := step.Scroll.Delta.PixelDelta(20).Y
		total60 += dy
	}

	// Recreate and sample at 15Hz: same curve, coarser ticks.
	recognizer2 := NewTouchGestureRecognizer(DefaultGestureTuning())
	recognizer2.HandleEventAt(touchEvent(touch, platform.TouchPhaseStarted, 10, 100), now)
	for i := 1; i <= 5; i++ {
		at := now.Add(time.Duration(i*20) * time.Millisecond)
		recognizer2.HandleEventAt(touchEvent(touch, platform.TouchPhaseMoved, 10, float32(100-15*i)), at)
	}
	recognizer2.HandleEventAt(touchEvent(touch, platform.TouchPhaseEnded, 10, 25), now.Add(120*time.Millisecond))
	var total15 geometry.Pixels
	for i := 0; recognizer2.HasMomentum(); i++ {
		step := recognizer2.TickMomentumAt(now.Add(150 * time.Millisecond).Add(time.Duration(i) * (1000 / 15) * time.Millisecond))
		if step == nil {
			break
		}
		dy := step.Scroll.Delta.PixelDelta(20).Y
		total15 += dy
	}
	if math.Abs(float64(total60-total15)) > 5 {
		t.Fatalf("momentum must be frame-rate independent: 60Hz=%v 15Hz=%v", total60, total15)
	}
}

func TestFlickVelocityReflectsReleaseSpeedNotWindowAverage(t *testing.T) {
	var tracker velocityTracker
	now := time.Now()
	// Accelerating flick: speed at release is much higher than average.
	positions := []float32{0, 2, 6, 14, 30, 62}
	for i, p := range positions {
		tracker.push(now.Add(time.Duration(i*20)*time.Millisecond), pt(10, p))
	}
	vx, vy := tracker.velocity()
	// Average over the window is 62/0.1 = 620 px/s; release speed should
	// be closer to (62-30)/0.02 = 1600 px/s.
	if vy < 900 {
		t.Fatalf("expected release speed (>900 px/s), got %v", vy)
	}
	// The least-squares fit may leave float residue on the motionless axis.
	if abs32(vx) > 0.001 {
		t.Fatalf("expected no horizontal velocity, got %v", vx)
	}
}

func TestSamplesBeforeAPauseDoNotContributeVelocity(t *testing.T) {
	var tracker velocityTracker
	now := time.Now()
	tracker.push(now, pt(10, 0))
	tracker.push(now.Add(20*time.Millisecond), pt(10, 10))
	// A long pause, then a slow movement.
	tracker.push(now.Add(200*time.Millisecond), pt(10, 12))
	tracker.push(now.Add(220*time.Millisecond), pt(10, 14))
	_, vy := tracker.velocity()
	if vy > 400 {
		t.Fatalf("pre-pause samples must not contribute: got %v px/s", vy)
	}
}
