// Package spring ports gpui's spring animation physics: an exact analytic
// integrator for the damped harmonic oscillator (frame-rate independent),
// plus AnimationPhase interpolation helpers.
package spring

import (
	"math"
	"time"
)

// CriticalDampingTolerance matches upstream.
const CriticalDampingTolerance = 1e-4

// Config is a spring configuration.
type Config struct {
	Stiffness float32 // > 0
	Damping   float32 // >= 0
	Mass      float32 // > 0
}

// NewConfig validates and constructs a config.
func NewConfig(stiffness, damping, mass float32) Config {
	if !(stiffness > 0) || stiffness != stiffness {
		stiffness = 1
	}
	if damping < 0 || damping != damping {
		damping = 0
	}
	if !(mass > 0) || mass != mass {
		mass = 1
	}
	return Config{Stiffness: stiffness, Damping: damping, Mass: mass}
}

// Canonical returns (omega0, zeta).
func (c Config) Canonical() (float64, float64) {
	omega0 := math.Sqrt(float64(c.Stiffness / c.Mass))
	zeta := float64(c.Damping) / (2 * math.Sqrt(float64(c.Stiffness*c.Mass)))
	return omega0, zeta
}

// State is a spring's position and velocity.
type State struct {
	Position float32
	Velocity float32
}

// Step advances the spring exactly by dt towards a fixed target
// (upstream SpringConfig::step).
func (c Config) Step(state State, target float32, dt time.Duration) State {
	if dt <= 0 {
		return state
	}
	seconds := dt.Seconds()
	p0 := float64(state.Position - target)
	v0 := float64(state.Velocity)
	omega0, zeta := c.Canonical()

	var m00, m01, m10, m11 float64
	switch {
	case zeta < 1-CriticalDampingTolerance: // underdamped
		decay := zeta * omega0
		omegaD := omega0 * math.Sqrt(1-zeta*zeta)
		e := math.Exp(-decay * seconds)
		s, co := math.Sincos(omegaD * seconds)
		m00 = e * (co + decay*s/omegaD)
		m01 = e * s / omegaD
		m10 = -e * omega0 * omega0 * s / omegaD
		m11 = e * (co - decay*s/omegaD)
	case zeta > 1+CriticalDampingTolerance: // overdamped
		r := math.Sqrt(zeta*zeta - 1)
		slow := -omega0 * (zeta - r)
		fast := -omega0 * (zeta + r)
		es := math.Exp(slow * seconds)
		ef := math.Exp(fast * seconds)
		// x(t) = A*es + B*ef with x(0)=p0, x'(0)=v0.
		A := (v0 - fast*p0) / (slow - fast)
		B := p0 - A
		m00 = 1 // unused marker; direct evaluation below
		m01 = 0
		m10 = 0
		m11 = 0
		p1 := A*es + B*ef
		v1 := A*slow*es + B*fast*ef
		return State{Position: target + float32(p1), Velocity: float32(v1)}
	default: // critically damped
		e := math.Exp(-omega0 * seconds)
		m00 = e * (1 + omega0*seconds)
		m01 = e * seconds
		m10 = -e * omega0 * omega0 * seconds
		m11 = e * (1 - omega0*seconds)
	}

	p1 := target + float32(m00*p0+m01*v0)
	v1 := float32(m10*p0 + m11*v0)
	return State{Position: p1, Velocity: v1}
}

// StepRamp advances towards a linearly moving target (first-order hold).
func (c Config) StepRamp(state State, target, targetVelocity float32, dt time.Duration) State {
	// Steady-state lag: x_ss = target + 2*zeta*v/omega0.
	omega0, zeta := c.Canonical()
	lag := float32(2*zeta/omega0) * targetVelocity
	shifted := State{Position: state.Position + lag, Velocity: state.Velocity - targetVelocity}
	out := c.Step(shifted, target, dt)
	out.Position -= lag
	out.Velocity += targetVelocity
	return out
}

// IsSettled reports |p - target| <= eps && |v| <= eps*omega0.
func (c Config) IsSettled(state State, target, epsilon float32) bool {
	omega0, _ := c.Canonical()
	return math.Abs(float64(state.Position-target)) <= float64(epsilon) &&
		math.Abs(float64(state.Velocity)) <= float64(epsilon)*omega0
}

// SettleTime returns a conservative upper bound (upstream settle_time).
func (c Config) SettleTime(state State, target, epsilon float32) time.Duration {
	omega0, zeta := c.Canonical()
	if omega0 == 0 {
		return time.Duration(math.MaxInt64)
	}
	// Envelope decay constant.
	var lambda float64
	switch {
	case zeta < 1:
		lambda = zeta * omega0
	case zeta == 1:
		lambda = omega0
	default:
		r := math.Sqrt(zeta*zeta - 1)
		lambda = omega0 * (zeta - r)
	}
	if lambda <= 0 {
		return time.Duration(math.MaxInt64)
	}
	p0 := math.Abs(float64(state.Position - target))
	v0 := math.Abs(float64(state.Velocity))
	amp := math.Sqrt(p0*p0 + (v0/omega0)*(v0/omega0))
	if amp == 0 {
		return 0
	}
	return time.Duration(math.Log(float64(amp)/math.Max(float64(epsilon), 1e-12)/lambda)*1e9) * time.Nanosecond
}

// SampledEasing adapts the spring to a duration-based easing function:
// it returns the duration and a progress->value function (upstream
// sampled_easing).
func (c Config) SampledEasing(epsilon float32) (time.Duration, func(float32) float32) {
	d := c.SettleTime(State{Position: 0, Velocity: 0}, 1, epsilon)
	if d <= 0 {
		d = time.Millisecond
	}
	total := d.Seconds()
	state := State{Position: 0, Velocity: 0}
	samples := make([]float32, 1+int(total*120))
	for i := range samples {
		if i > 0 {
			state = c.Step(state, 1, time.Second/120)
		}
		samples[i] = state.Position
	}
	return d, func(progress float32) float32 {
		if progress <= 0 {
			return 0
		}
		if progress >= 1 {
			return 1
		}
		idx := progress * float32(len(samples)-1)
		i := int(idx)
		frac := idx - float32(i)
		if i >= len(samples)-1 {
			return samples[len(samples)-1]
		}
		return samples[i] + (samples[i+1]-samples[i])*frac
	}
}

// Phase is the unbounded animation phase coordinate.
type Phase float32

// Interpolate linearly interpolates.
func (p Phase) Interpolate(to Phase, t float32) Phase {
	return Phase(float32(p) + (float32(to)-float32(p))*t)
}

// InterpolateClamped clamps the result to the segment.
func (p Phase) InterpolateClamped(to Phase, t float32) Phase {
	v := float32(p) + (float32(to)-float32(p))*t
	lo, hi := float32(p), float32(to)
	if lo > hi {
		lo, hi = hi, lo
	}
	if v < lo {
		v = lo
	}
	if v > hi {
		v = hi
	}
	return Phase(v)
}

// InterpolateBetween interpolates p between from and to by phase t.
func (p Phase) InterpolateBetween(from, to Phase, t float32) Phase {
	return Phase(float32(from) + (float32(to)-float32(from))*t)
}
