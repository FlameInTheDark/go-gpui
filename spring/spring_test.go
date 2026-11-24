package spring

import (
	"math"
	"testing"
	"time"
)

func TestSpringConverges(t *testing.T) {
	c := NewConfig(100, 10, 1)
	s := State{Position: 0, Velocity: 0}
	for i := 0; i < 1000; i++ {
		s = c.Step(s, 1, 16*time.Millisecond)
	}
	if math.Abs(float64(s.Position)-1) > 0.001 {
		t.Fatalf("position: %v", s.Position)
	}
	if math.Abs(float64(s.Velocity)) > 0.01 {
		t.Fatalf("velocity: %v", s.Velocity)
	}
	if !c.IsSettled(s, 1, 0.001) {
		t.Fatal("should be settled")
	}
}

func TestSpringFrameRateIndependence(t *testing.T) {
	c := NewConfig(170, 26, 1)
	// Two different frame rates reach the same state.
	s60 := State{}
	for i := 0; i < 60; i++ {
		s60 = c.Step(s60, 1, time.Second/60)
	}
	s144 := State{}
	for i := 0; i < 144; i++ {
		s144 = c.Step(s144, 1, time.Second/144)
	}
	d := math.Abs(float64(s60.Position - s144.Position))
	if d > 0.001 {
		t.Fatalf("frame rate dependence: %v (%v vs %v)", d, s60.Position, s144.Position)
	}
}

func TestOverdampedAndCritical(t *testing.T) {
	// Overdamped: no overshoot.
	c := NewConfig(100, 40, 1)
	s := State{}
	for i := 0; i < 500; i++ {
		s = c.Step(s, 1, 16*time.Millisecond)
		if s.Position > 1.001 {
			t.Fatalf("overdamped overshoot: %v", s.Position)
		}
	}
	if math.Abs(float64(s.Position)-1) > 0.01 {
		t.Fatalf("overdamped final: %v", s.Position)
	}
	// Critical.
	c2 := NewConfig(100, 20, 1) // zeta = 20/(2*sqrt(100)) = 1
	s2 := State{}
	for i := 0; i < 500; i++ {
		s2 = c2.Step(s2, 1, 16*time.Millisecond)
	}
	if math.Abs(float64(s2.Position)-1) > 0.01 {
		t.Fatalf("critical final: %v", s2.Position)
	}
}

func TestUnderdampedOvershoots(t *testing.T) {
	c := NewConfig(170, 12, 1) // zeta < 1
	s := State{}
	maxPos := float32(0)
	for i := 0; i < 600; i++ {
		s = c.Step(s, 1, 16*time.Millisecond)
		if s.Position > maxPos {
			maxPos = s.Position
		}
	}
	if maxPos <= 1.0 {
		t.Fatalf("underdamped should overshoot: %v", maxPos)
	}
}

func TestSampledEasing(t *testing.T) {
	c := NewConfig(100, 14, 1)
	d, ease := c.SampledEasing(0.001)
	if d <= 0 {
		t.Fatal("duration")
	}
	if ease(0) != 0 || math.Abs(float64(ease(1))-1) > 0.01 {
		t.Fatalf("endpoints: %v %v", ease(0), ease(1))
	}
}

func TestPhaseInterpolate(t *testing.T) {
	p := Phase(0)
	if p.Interpolate(1, 0.5) != 0.5 {
		t.Fatal("interpolate")
	}
	if Phase(0.9).Interpolate(0.1, 0.5) != 0.5 {
		t.Fatal("interpolate between")
	}
}
