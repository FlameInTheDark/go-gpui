// Package geometry provides the coordinate and measurement types used
// throughout go-gpui. It is a faithful Go port of the types in
// the upstream Rust gpui geometry.rs (Apache-2.0).
//
// The core ideas mirrored from the Rust original:
//
//   - Pixels are logical (user-space) lengths; DevicePixels are physical
//     integers; ScaledPixels are device-space floats. Conversion between
//     them goes through a scale factor, and the three use *different*
//     rounding rules that must not be mixed up.
//   - Point/Size/Bounds/Edges/Corners are generic over the unit (any defined
//     float32/int32 type). Go does not allow methods on instantiated generic
//     types, so unit-specific operations are package-level functions (e.g.
//     IntersectBounds) while unit-agnostic operations stay methods.
//   - The Length family (Length > DefiniteLength > AbsoluteLength) models
//     CSS-like lengths with px, rem and percentage units plus "auto".
package geometry

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Axis / Anchor
// ---------------------------------------------------------------------------

// Axis is either the horizontal or the vertical axis.
type Axis int

const (
	AxisVertical Axis = iota
	AxisHorizontal
)

// Invert returns the other axis.
func (a Axis) Invert() Axis {
	if a == AxisVertical {
		return AxisHorizontal
	}
	return AxisVertical
}

// Anchor names a corner (or edge center) of a rectangle.
type Anchor int

const (
	AnchorTopLeft Anchor = iota
	AnchorTopRight
	AnchorBottomLeft
	AnchorBottomRight
	AnchorTopCenter
	AnchorBottomCenter
	AnchorLeftCenter
	AnchorRightCenter
)

// Opposite returns the anchor across both axes.
func (a Anchor) Opposite() Anchor {
	switch a {
	case AnchorTopLeft:
		return AnchorBottomRight
	case AnchorTopRight:
		return AnchorBottomLeft
	case AnchorBottomLeft:
		return AnchorTopRight
	case AnchorBottomRight:
		return AnchorTopLeft
	case AnchorTopCenter:
		return AnchorBottomCenter
	case AnchorBottomCenter:
		return AnchorTopCenter
	case AnchorLeftCenter:
		return AnchorRightCenter
	case AnchorRightCenter:
		return AnchorLeftCenter
	}
	return a
}

// OtherSideAlong returns the anchor on the other side of the given axis.
func (a Anchor) OtherSideAlong(axis Axis) Anchor {
	switch axis {
	case AxisHorizontal:
		switch a {
		case AnchorTopLeft:
			return AnchorBottomLeft
		case AnchorTopRight:
			return AnchorBottomRight
		case AnchorBottomLeft:
			return AnchorTopLeft
		case AnchorBottomRight:
			return AnchorTopRight
		case AnchorTopCenter:
			return AnchorBottomCenter
		case AnchorBottomCenter:
			return AnchorTopCenter
		}
	case AxisVertical:
		switch a {
		case AnchorTopLeft:
			return AnchorTopRight
		case AnchorTopRight:
			return AnchorTopLeft
		case AnchorBottomLeft:
			return AnchorBottomRight
		case AnchorBottomRight:
			return AnchorBottomLeft
		case AnchorLeftCenter:
			return AnchorRightCenter
		case AnchorRightCenter:
			return AnchorLeftCenter
		}
	}
	return a
}

// IsCenter reports whether the anchor is an edge center.
func (a Anchor) IsCenter() bool {
	switch a {
	case AnchorTopCenter, AnchorBottomCenter, AnchorLeftCenter, AnchorRightCenter:
		return true
	}
	return false
}

// IsBottom reports whether the anchor is on the bottom edge.
func (a Anchor) IsBottom() bool {
	switch a {
	case AnchorBottomLeft, AnchorBottomRight, AnchorBottomCenter:
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Unit types
// ---------------------------------------------------------------------------

// Pixels is a length in logical (user-space) pixels.
type Pixels float32

// Common Pixel constants.
const (
	PixelsZero = Pixels(0)
	PixelsMax  = Pixels(math.MaxFloat32)
	PixelsMin  = Pixels(-math.MaxFloat32)
)

// Px constructs Pixels from a float.
func Px(v float32) Pixels { return Pixels(v) }

// PixelsFromF64 constructs Pixels from a float64.
func PixelsFromF64(v float64) Pixels { return Pixels(float32(v)) }

// AsF32 returns the raw value.
func (p Pixels) AsF32() float32 { return float32(p) }

// ToF64 widens the value.
func (p Pixels) ToF64() float64 { return float64(p) }

// ToI32 truncates toward zero, saturating on overflow and mapping NaN to 0
// (Rust `as` semantics).
func (p Pixels) ToI32() int32 {
	v := float32(p)
	if math.IsNaN(float64(v)) {
		return 0
	}
	if v >= math.MaxInt32 {
		return math.MaxInt32
	}
	if v <= math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

func (p Pixels) Floor() Pixels { return Pixels(math.Floor(float64(p))) }
func (p Pixels) Round() Pixels { return Pixels(math.Round(float64(p))) }
func (p Pixels) Ceil() Pixels  { return Pixels(math.Ceil(float64(p))) }
func (p Pixels) Abs() Pixels   { return Pixels(math.Abs(float64(p))) }

// Signum returns -1.0, 0.0 or 1.0.
func (p Pixels) Signum() float32 {
	if p > 0 {
		return 1
	} else if p < 0 {
		return -1
	}
	return 0
}

// Pow raises the value to a power.
func (p Pixels) Pow(e float32) Pixels { return Pixels(math.Pow(float64(p), float64(e))) }

// Scale multiplies by a scale factor, producing device-space pixels.
func (p Pixels) Scale(f float32) ScaledPixels { return ScaledPixels(float32(p) * f) }

// IsZero reports whether the value equals zero.
func (p Pixels) IsZero() bool { return p == 0 }

// Add returns p + o.
func (p Pixels) Add(o Pixels) Pixels { return p + o }

// Sub returns p - o.
func (p Pixels) Sub(o Pixels) Pixels { return p - o }

// Neg returns -p.
func (p Pixels) Neg() Pixels { return -p }

// MulScalar multiplies by a scalar.
func (p Pixels) MulScalar(s float32) Pixels { return Pixels(float32(p) * s) }

// DivScalar divides by a scalar.
func (p Pixels) DivScalar(s float32) Pixels { return Pixels(float32(p) / s) }

// Quo divides by another Pixels value, yielding a plain ratio.
func (p Pixels) Quo(o Pixels) float32 { return float32(p / o) }

// Rem returns the remainder of p/o with the sign of p.
func (p Pixels) Rem(o Pixels) Pixels { return Pixels(math.Mod(float64(p), float64(o))) }

// Cmp compares two Pixels totally (like f32::total_cmp). Returns -1, 0 or 1.
func (p Pixels) Cmp(o Pixels) int { return totalCmp32(float32(p), float32(o)) }

// Min returns the smaller value.
func (p Pixels) Min(o Pixels) Pixels {
	if p.Cmp(o) <= 0 {
		return p
	}
	return o
}

// Max returns the larger value.
func (p Pixels) Max(o Pixels) Pixels {
	if p.Cmp(o) >= 0 {
		return p
	}
	return o
}

func (p Pixels) String() string { return fmt.Sprintf("%gpx", float32(p)) }

// DevicePixels is a length in physical device pixels.
type DevicePixels int32

// DevicePx constructs DevicePixels.
func DevicePx(v int32) DevicePixels { return DevicePixels(v) }

// DevicePxFromInt constructs DevicePixels from an int.
func DevicePxFromInt(v int) DevicePixels { return DevicePixels(int32(v)) }

func (d DevicePixels) AsI32() int32  { return int32(d) }
func (d DevicePixels) AsU32() uint32 { return uint32(d) }
func (d DevicePixels) AsInt() int    { return int(d) }
func (d DevicePixels) IsZero() bool  { return d == 0 }

func (d DevicePixels) Add(o DevicePixels) DevicePixels { return d + o }
func (d DevicePixels) Sub(o DevicePixels) DevicePixels { return d - o }
func (d DevicePixels) DivScalar(s int32) DevicePixels  { return DevicePixels(int32(d) / s) }
func (d DevicePixels) Half() DevicePixels              { return d / 2 }

// Min returns the smaller value.
func (d DevicePixels) Min(o DevicePixels) DevicePixels {
	if d < o {
		return d
	}
	return o
}

// Max returns the larger value.
func (d DevicePixels) Max(o DevicePixels) DevicePixels {
	if d > o {
		return d
	}
	return o
}

// ToBytes converts to a packed byte count at the given bytes per pixel.
func (d DevicePixels) ToBytes(bytesPerPixel uint8) uint32 {
	return uint32(d) * uint32(bytesPerPixel)
}

// ToScaled widens to ScaledPixels.
func (d DevicePixels) ToScaled() ScaledPixels { return ScaledPixels(float32(d)) }

func (d DevicePixels) String() string { return fmt.Sprintf("%d px (device)", int32(d)) }

// ScaledPixels is a device-space float length (logical pixels * scale factor).
type ScaledPixels float32

// ScaledPx constructs ScaledPixels.
func ScaledPx(v float32) ScaledPixels { return ScaledPixels(v) }

func (p ScaledPixels) AsF32() float32 { return float32(p) }
func (p ScaledPixels) IsZero() bool   { return p == 0 }

func (p ScaledPixels) Add(o ScaledPixels) ScaledPixels  { return p + o }
func (p ScaledPixels) Sub(o ScaledPixels) ScaledPixels  { return p - o }
func (p ScaledPixels) Neg() ScaledPixels                { return -p }
func (p ScaledPixels) MulScalar(s float32) ScaledPixels { return ScaledPixels(float32(p) * s) }
func (p ScaledPixels) DivScalar(s float32) ScaledPixels { return ScaledPixels(float32(p) / s) }
func (p ScaledPixels) Quo(o ScaledPixels) float32       { return float32(p / o) }

func (p ScaledPixels) Rem(o ScaledPixels) ScaledPixels {
	return ScaledPixels(math.Mod(float64(p), float64(o)))
}

func (p ScaledPixels) Floor() ScaledPixels { return ScaledPixels(math.Floor(float64(p))) }
func (p ScaledPixels) Round() ScaledPixels { return ScaledPixels(math.Round(float64(p))) }
func (p ScaledPixels) Ceil() ScaledPixels  { return ScaledPixels(math.Ceil(float64(p))) }

// ToDevicePixels converts by ceiling, matching the Rust From impl.
func (p ScaledPixels) ToDevicePixels() DevicePixels {
	return DevicePixels(int32(math.Ceil(float64(p))))
}

// Cmp compares totally like f32::total_cmp.
func (p ScaledPixels) Cmp(o ScaledPixels) int { return totalCmp32(float32(p), float32(o)) }

// Min returns the smaller value.
func (p ScaledPixels) Min(o ScaledPixels) ScaledPixels {
	if p.Cmp(o) <= 0 {
		return p
	}
	return o
}

// Max returns the larger value.
func (p ScaledPixels) Max(o ScaledPixels) ScaledPixels {
	if p.Cmp(o) >= 0 {
		return p
	}
	return o
}

// Rems is a length in multiples of the root font size.
type Rems float32

// RemsZero is zero rems.
const RemsZero = Rems(0)

// RemsFrom constructs Rems.
func RemsFrom(v float32) Rems { return Rems(v) }

func (r Rems) IsZero() bool             { return r == 0 }
func (r Rems) Add(o Rems) Rems          { return r + o }
func (r Rems) Sub(o Rems) Rems          { return r - o }
func (r Rems) Neg() Rems                { return -r }
func (r Rems) MulScalar(s float32) Rems { return Rems(float32(r) * s) }
func (r Rems) DivScalar(s float32) Rems { return Rems(float32(r) / s) }
func (r Rems) Half() Rems               { return r / 2.0 }

// MulPixels multiplies rems by a rem size, yielding Pixels.
func (r Rems) MulPixels(remSize Pixels) Pixels { return Pixels(float32(r) * remSize.AsF32()) }

// ToPixels converts to Pixels at the given rem size.
func (r Rems) ToPixels(remSize Pixels) Pixels { return r.MulPixels(remSize) }

func (r Rems) String() string { return fmt.Sprintf("%grem", float32(r)) }

// Radians is an angle in radians.
type Radians float32

// RadiansFrom constructs Radians.
func RadiansFrom(v float32) Radians { return Radians(v) }

func (r Radians) Add(o Radians) Radians       { return r + o }
func (r Radians) Sub(o Radians) Radians       { return r - o }
func (r Radians) Neg() Radians                { return -r }
func (r Radians) DivScalar(s float32) Radians { return Radians(float32(r) / s) }
func (r Radians) MulScalar(s float32) Radians { return Radians(float32(r) * s) }

// Percentage is a fraction in [0, 1].
type Percentage float32

// PercentageFrom constructs Percentage, clamped to [0,1].
func PercentageFrom(v float32) Percentage {
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	return Percentage(v)
}

// ToRadians converts to a fraction of a full turn.
func (p Percentage) ToRadians() Radians { return Radians(float32(p) * 2 * float32(math.Pi)) }

// totalCmp32 implements f32::total_cmp.
func totalCmp32(a, b float32) int {
	ia, ib := int32(math.Float32bits(a)), int32(math.Float32bits(b))
	ia ^= int32(uint32(ia>>31) >> 1)
	ib ^= int32(uint32(ib>>31) >> 1)
	switch {
	case ia < ib:
		return -1
	case ia > ib:
		return 1
	default:
		return 0
	}
}

// ---------------------------------------------------------------------------
// Point
// ---------------------------------------------------------------------------

// Number is the constraint satisfied by the unit types (Pixels,
// DevicePixels, ScaledPixels, Rems, Radians, plain float32/int32...).
type Number interface {
	~float32 | ~int32
}

// Point is a 2D point generic over its unit.
type Point[T Number] struct {
	X T
	Y T
}

// NewPoint constructs a Point.
func NewPoint[T Number](x, y T) Point[T] { return Point[T]{X: x, Y: y} }

// PointPixels is a convenience constructor for logical points.
func PointPixels(x, y float32) Point[Pixels] { return Point[Pixels]{Px(x), Px(y)} }

// PointDevice constructs a device-space point.
func PointDevice(x, y int32) Point[DevicePixels] {
	return Point[DevicePixels]{DevicePx(x), DevicePx(y)}
}

// MapPoint converts both coordinates with f.
func MapPoint[T, U Number](p Point[T], f func(T) U) Point[U] {
	return Point[U]{X: f(p.X), Y: f(p.Y)}
}

// ScalePoint multiplies a logical point by a scale factor.
func ScalePoint(p Point[Pixels], f float32) Point[ScaledPixels] {
	return Point[ScaledPixels]{X: p.X.Scale(f), Y: p.Y.Scale(f)}
}

// Magnitude returns sqrt(x^2 + y^2).
func (p Point[T]) Magnitude() float64 {
	return math.Hypot(float64(p.X), float64(p.Y))
}

// RelativeTo returns p - origin.
func (p Point[T]) RelativeTo(origin Point[T]) Point[T] {
	return Point[T]{X: p.X - origin.X, Y: p.Y - origin.Y}
}

// PointPixelsMax returns the per-axis maximum using total ordering (ties
// prefer b, matching Rust's `>`).
func PointPixelsMax(a, b Point[Pixels]) Point[Pixels] {
	return Point[Pixels]{
		X: pickPixels(a.X, b.X, a.X.Cmp(b.X) > 0),
		Y: pickPixels(a.Y, b.Y, a.Y.Cmp(b.Y) > 0),
	}
}

// PointPixelsMin returns the per-axis minimum (ties prefer a, like `<=`).
func PointPixelsMin(a, b Point[Pixels]) Point[Pixels] {
	return Point[Pixels]{
		X: pickPixels(a.X, b.X, a.X.Cmp(b.X) <= 0),
		Y: pickPixels(a.Y, b.Y, a.Y.Cmp(b.Y) <= 0),
	}
}

// PointPixelsClamp clamps each axis to [min, max].
func PointPixelsClamp(p, minP, maxP Point[Pixels]) Point[Pixels] {
	return PointPixelsMax(minP, PointPixelsMin(p, maxP))
}

func pickPixels(a, b Pixels, useA bool) Pixels {
	if useA {
		return a
	}
	return b
}

func (p Point[T]) String() string { return fmt.Sprintf("(%v, %v)", p.X, p.Y) }

// ---------------------------------------------------------------------------
// Size
// ---------------------------------------------------------------------------

// Size is a 2D size generic over its unit.
type Size[T Number] struct {
	Width  T
	Height T
}

// NewSize constructs a Size.
func NewSize[T Number](w, h T) Size[T] { return Size[T]{Width: w, Height: h} }

// SizePixels constructs a logical size.
func SizePixels(w, h float32) Size[Pixels] { return Size[Pixels]{Px(w), Px(h)} }

// SizeDevice constructs a device size.
func SizeDevice(w, h int32) Size[DevicePixels] {
	return Size[DevicePixels]{DevicePx(w), DevicePx(h)}
}

// MapSize converts both dimensions with f.
func MapSize[T, U Number](s Size[T], f func(T) U) Size[U] {
	return Size[U]{Width: f(s.Width), Height: f(s.Height)}
}

// ScaleSize multiplies a logical size by a scale factor.
func ScaleSize(s Size[Pixels], f float32) Size[ScaledPixels] {
	return Size[ScaledPixels]{Width: s.Width.Scale(f), Height: s.Height.Scale(f)}
}

// Center returns the center point of the size.
func (s Size[T]) Center() Point[T] { return Point[T]{X: s.Width / 2, Y: s.Height / 2} }

// SizePixelsMax returns per-dimension max (ties prefer a).
func SizePixelsMax(a, b Size[Pixels]) Size[Pixels] {
	return Size[Pixels]{
		Width:  pickPixels(a.Width, b.Width, a.Width.Cmp(b.Width) >= 0),
		Height: pickPixels(a.Height, b.Height, a.Height.Cmp(b.Height) >= 0),
	}
}

// SizePixelsMin returns per-dimension min (ties prefer b).
func SizePixelsMin(a, b Size[Pixels]) Size[Pixels] {
	return Size[Pixels]{
		Width:  pickPixels(a.Width, b.Width, a.Width.Cmp(b.Width) < 0),
		Height: pickPixels(a.Height, b.Height, a.Height.Cmp(b.Height) < 0),
	}
}

// SizeToDevicePixels rounds each dimension to device pixels.
func SizeToDevicePixels(s Size[Pixels], scaleFactor float32) Size[DevicePixels] {
	return Size[DevicePixels]{
		Width:  DevicePixels(int32(math.Round(float64(s.Width.AsF32() * scaleFactor)))),
		Height: DevicePixels(int32(math.Round(float64(s.Height.AsF32() * scaleFactor)))),
	}
}

// DeviceSizeToPixels divides by the scale factor.
func DeviceSizeToPixels(s Size[DevicePixels], scaleFactor float32) Size[Pixels] {
	return Size[Pixels]{
		Width:  Px(float32(s.Width) / scaleFactor),
		Height: Px(float32(s.Height) / scaleFactor),
	}
}

func (s Size[T]) String() string { return fmt.Sprintf("%v × %v", s.Width, s.Height) }

// ---------------------------------------------------------------------------
// Bounds
// ---------------------------------------------------------------------------

// Bounds is an axis-aligned rectangle generic over its unit.
type Bounds[T Number] struct {
	Origin Point[T]
	Size   Size[T]
}

// NewBounds constructs Bounds from an origin and size.
func NewBounds[T Number](origin Point[T], size Size[T]) Bounds[T] {
	return Bounds[T]{Origin: origin, Size: size}
}

// BoundsPixels constructs logical bounds.
func BoundsPixels(x, y, w, h float32) Bounds[Pixels] {
	return Bounds[Pixels]{
		Origin: Point[Pixels]{Px(x), Px(y)},
		Size:   Size[Pixels]{Px(w), Px(h)},
	}
}

// BoundsDevice constructs device bounds.
func BoundsDevice(x, y, w, h int32) Bounds[DevicePixels] {
	return Bounds[DevicePixels]{
		Origin: Point[DevicePixels]{DevicePx(x), DevicePx(y)},
		Size:   Size[DevicePixels]{DevicePx(w), DevicePx(h)},
	}
}

// FromCorners builds bounds from a top-left and bottom-right corner.
func FromCorners[T Number](topLeft, bottomRight Point[T]) Bounds[T] {
	return Bounds[T]{
		Origin: topLeft,
		Size: Size[T]{
			Width:  bottomRight.X - topLeft.X,
			Height: bottomRight.Y - topLeft.Y,
		},
	}
}

// CenteredAt returns bounds of the given size centered on a point.
func CenteredAt[T Number](center Point[T], size Size[T]) Bounds[T] {
	return Bounds[T]{
		Origin: Point[T]{X: center.X - size.Width/2, Y: center.Y - size.Height/2},
		Size:   size,
	}
}

// FromAnchorAndSize positions a size relative to an origin per the anchor.
func FromAnchorAndSize[T Number](anchor Anchor, origin Point[T], size Size[T]) Bounds[T] {
	switch anchor {
	case AnchorTopLeft:
		return NewBounds(origin, size)
	case AnchorTopRight:
		return NewBounds(Point[T]{X: origin.X - size.Width, Y: origin.Y}, size)
	case AnchorBottomLeft:
		return NewBounds(Point[T]{X: origin.X, Y: origin.Y - size.Height}, size)
	case AnchorBottomRight:
		return NewBounds(Point[T]{X: origin.X - size.Width, Y: origin.Y - size.Height}, size)
	case AnchorTopCenter:
		return NewBounds(Point[T]{X: origin.X - size.Width/2, Y: origin.Y}, size)
	case AnchorBottomCenter:
		return NewBounds(Point[T]{X: origin.X - size.Width/2, Y: origin.Y - size.Height}, size)
	case AnchorLeftCenter:
		return NewBounds(Point[T]{X: origin.X, Y: origin.Y - size.Height/2}, size)
	case AnchorRightCenter:
		return NewBounds(Point[T]{X: origin.X - size.Width, Y: origin.Y - size.Height/2}, size)
	}
	return NewBounds(origin, size)
}

func (b Bounds[T]) Top() T    { return b.Origin.Y }
func (b Bounds[T]) Left() T   { return b.Origin.X }
func (b Bounds[T]) Right() T  { return b.Origin.X + b.Size.Width }
func (b Bounds[T]) Bottom() T { return b.Origin.Y + b.Size.Height }

func (b Bounds[T]) TopLeft() Point[T]    { return b.Origin }
func (b Bounds[T]) TopRight() Point[T]   { return Point[T]{X: b.Right(), Y: b.Top()} }
func (b Bounds[T]) BottomLeft() Point[T] { return Point[T]{X: b.Left(), Y: b.Bottom()} }
func (b Bounds[T]) BottomRight() Point[T] {
	return Point[T]{X: b.Right(), Y: b.Bottom()}
}
func (b Bounds[T]) TopCenter() Point[T] {
	return Point[T]{X: b.Origin.X + b.Size.Width/2, Y: b.Top()}
}
func (b Bounds[T]) BottomCenter() Point[T] {
	return Point[T]{X: b.Origin.X + b.Size.Width/2, Y: b.Bottom()}
}
func (b Bounds[T]) LeftCenter() Point[T] {
	return Point[T]{X: b.Left(), Y: b.Origin.Y + b.Size.Height/2}
}
func (b Bounds[T]) RightCenter() Point[T] {
	return Point[T]{X: b.Right(), Y: b.Origin.Y + b.Size.Height/2}
}

// Corner returns the point named by the anchor.
func (b Bounds[T]) Corner(anchor Anchor) Point[T] {
	switch anchor {
	case AnchorTopLeft:
		return b.TopLeft()
	case AnchorTopRight:
		return b.TopRight()
	case AnchorBottomLeft:
		return b.BottomLeft()
	case AnchorBottomRight:
		return b.BottomRight()
	case AnchorTopCenter:
		return b.TopCenter()
	case AnchorBottomCenter:
		return b.BottomCenter()
	case AnchorLeftCenter:
		return b.LeftCenter()
	case AnchorRightCenter:
		return b.RightCenter()
	}
	return b.Origin
}

// Center returns the midpoint.
func (b Bounds[T]) Center() Point[T] { return b.Size.Center() }

// HalfPerimeter returns width + height.
func (b Bounds[T]) HalfPerimeter() T { return b.Size.Width + b.Size.Height }

// Dilate grows the bounds by amount on all sides.
func (b Bounds[T]) Dilate(amount T) Bounds[T] {
	return Bounds[T]{
		Origin: Point[T]{X: b.Origin.X - amount, Y: b.Origin.Y - amount},
		Size:   Size[T]{Width: b.Size.Width + 2*amount, Height: b.Size.Height + 2*amount},
	}
}

// Inset shrinks the bounds by amount on all sides.
func (b Bounds[T]) Inset(amount T) Bounds[T] { return b.Dilate(-amount) }

// Extend grows the bounds by per-edge amounts.
func (b Bounds[T]) Extend(edges Edges[T]) Bounds[T] {
	return Bounds[T]{
		Origin: Point[T]{X: b.Origin.X - edges.Left, Y: b.Origin.Y - edges.Top},
		Size: Size[T]{
			Width:  b.Size.Width + edges.Left + edges.Right,
			Height: b.Size.Height + edges.Top + edges.Bottom,
		},
	}
}

// SpaceWithin returns the gap from b to outer on each edge:
// top=self.top-outer.top; right=outer.right-self.right;
// bottom=outer.bottom-self.bottom; left=self.left-outer.left.
func (b Bounds[T]) SpaceWithin(outer Bounds[T]) Edges[T] {
	return Edges[T]{
		Top:    b.Top() - outer.Top(),
		Right:  outer.Right() - b.Right(),
		Bottom: outer.Bottom() - b.Bottom(),
		Left:   b.Left() - outer.Left(),
	}
}

// MapBounds converts origin and size with f.
func MapBounds[T, U Number](b Bounds[T], f func(T) U) Bounds[U] {
	return Bounds[U]{
		Origin: MapPoint(b.Origin, f),
		Size:   MapSize(b.Size, f),
	}
}

// ScaleBounds multiplies by a scale factor.
func ScaleBounds(b Bounds[Pixels], f float32) Bounds[ScaledPixels] {
	return Bounds[ScaledPixels]{
		Origin: ScalePoint(b.Origin, f),
		Size:   ScaleSize(b.Size, f),
	}
}

// BoundsContains reports whether the point lies in the half-open rectangle.
func BoundsContains(b Bounds[Pixels], p Point[Pixels]) bool {
	return p.X.Cmp(b.Origin.X) >= 0 && p.X.Cmp(b.Right()) < 0 &&
		p.Y.Cmp(b.Origin.Y) >= 0 && p.Y.Cmp(b.Bottom()) < 0
}

// DeviceBoundsContains is BoundsContains for device pixels.
func DeviceBoundsContains(b Bounds[DevicePixels], p Point[DevicePixels]) bool {
	return p.X >= b.Origin.X && p.X < b.Right() &&
		p.Y >= b.Origin.Y && p.Y < b.Bottom()
}

// ScaledBoundsContains is BoundsContains for scaled pixels.
func ScaledBoundsContains(b Bounds[ScaledPixels], p Point[ScaledPixels]) bool {
	return p.X.Cmp(b.Origin.X) >= 0 && p.X.Cmp(b.Right()) < 0 &&
		p.Y.Cmp(b.Origin.Y) >= 0 && p.Y.Cmp(b.Bottom()) < 0
}

// BoundsIntersects reports strict overlap; touching edges do not intersect.
func BoundsIntersects(a, b Bounds[Pixels]) bool {
	i := IntersectBounds(a, b)
	return i.Size.Width.AsF32() > 0 && i.Size.Height.AsF32() > 0
}

// BoundsIsEmpty reports whether either dimension is non-positive.
func BoundsIsEmpty(b Bounds[Pixels]) bool {
	return b.Size.Width.AsF32() <= 0 || b.Size.Height.AsF32() <= 0
}

// ScaledBoundsIsEmpty reports whether either dimension is non-positive.
func ScaledBoundsIsEmpty(b Bounds[ScaledPixels]) bool {
	return b.Size.Width.AsF32() <= 0 || b.Size.Height.AsF32() <= 0
}

// BoundsIsContainedWithin reports whether b fits strictly inside other; a
// bounds equal to other returns false because Contains is half-open.
func BoundsIsContainedWithin(b, other Bounds[Pixels]) bool {
	return BoundsContains(other, b.Origin) && BoundsContains(other, b.BottomRight())
}

// IntersectBounds clamps the two rectangles together; degenerate results
// collapse to zero size, never negative.
func IntersectBounds(a, b Bounds[Pixels]) Bounds[Pixels] {
	topLeft := PointPixelsMax(a.Origin, b.Origin)
	bottomRight := PointPixelsMax(
		PointPixelsMin(a.BottomRight(), b.BottomRight()),
		topLeft,
	)
	return FromCorners(topLeft, bottomRight)
}

// IntersectScaledBounds is IntersectBounds for scaled pixels.
func IntersectScaledBounds(a, b Bounds[ScaledPixels]) Bounds[ScaledPixels] {
	x0 := a.Origin.X.Max(b.Origin.X)
	y0 := a.Origin.Y.Max(b.Origin.Y)
	x1 := a.BottomRight().X.Min(b.BottomRight().X)
	y1 := a.BottomRight().Y.Min(b.BottomRight().Y)
	x0, y0 = x0.Max(0), y0.Max(0)
	return FromCorners(Point[ScaledPixels]{X: x0, Y: y0}, Point[ScaledPixels]{X: x1, Y: y1})
}

// UnionBounds returns the smallest enclosing rectangle.
func UnionBounds(a, b Bounds[Pixels]) Bounds[Pixels] {
	topLeft := PointPixelsMin(a.Origin, b.Origin)
	bottomRight := PointPixelsMax(a.BottomRight(), b.BottomRight())
	return FromCorners(topLeft, bottomRight)
}

// JoinBounds is UnionBounds but an empty operand is the identity.
func JoinBounds(a, b Bounds[Pixels]) Bounds[Pixels] {
	if BoundsIsEmpty(a) {
		return b
	}
	if BoundsIsEmpty(b) {
		return a
	}
	return UnionBounds(a, b)
}

// LocalizePoint converts a point to bounds-local coordinates; ok is false
// when the point is not contained.
func LocalizePoint(b Bounds[Pixels], p Point[Pixels]) (Point[Pixels], bool) {
	if !BoundsContains(b, p) {
		return Point[Pixels]{}, false
	}
	return p.RelativeTo(b.Origin), true
}

// BoundsToDevicePixels converts with per-component rounding.
func BoundsToDevicePixels(b Bounds[Pixels], scaleFactor float32) Bounds[DevicePixels] {
	return Bounds[DevicePixels]{
		Origin: Point[DevicePixels]{
			X: DevicePixels(int32(math.Round(float64(b.Origin.X.AsF32() * scaleFactor)))),
			Y: DevicePixels(int32(math.Round(float64(b.Origin.Y.AsF32() * scaleFactor)))),
		},
		Size: SizeToDevicePixels(b.Size, scaleFactor),
	}
}

// DeviceBoundsToPixels divides by the scale factor.
func DeviceBoundsToPixels(b Bounds[DevicePixels], scaleFactor float32) Bounds[Pixels] {
	return Bounds[Pixels]{
		Origin: Point[Pixels]{
			X: Px(float32(b.Origin.X) / scaleFactor),
			Y: Px(float32(b.Origin.Y) / scaleFactor),
		},
		Size: DeviceSizeToPixels(b.Size, scaleFactor),
	}
}

func (b Bounds[T]) String() string {
	return fmt.Sprintf("%v - {right: %v, bottom: %v} (size %v)", b.Origin, b.Right(), b.Bottom(), b.Size)
}

// ---------------------------------------------------------------------------
// Edges / Corners
// ---------------------------------------------------------------------------

// Edges holds per-edge values in top/right/bottom/left order.
type Edges[T Number] struct {
	Top    T
	Right  T
	Bottom T
	Left   T
}

// AllEdges returns uniform edges.
func AllEdges[T Number](v T) Edges[T] {
	return Edges[T]{Top: v, Right: v, Bottom: v, Left: v}
}

// MapEdges converts each edge value.
func MapEdges[T, U Number](e Edges[T], f func(T) U) Edges[U] {
	return Edges[U]{Top: f(e.Top), Right: f(e.Right), Bottom: f(e.Bottom), Left: f(e.Left)}
}

// ScaleEdges multiplies logical edges by a scale factor.
func ScaleEdges(e Edges[Pixels], f float32) Edges[ScaledPixels] {
	return Edges[ScaledPixels]{
		Top: e.Top.Scale(f), Right: e.Right.Scale(f),
		Bottom: e.Bottom.Scale(f), Left: e.Left.Scale(f),
	}
}

// Corners holds per-corner values in top-left/top-right/bottom-right/bottom-left order.
type Corners[T Number] struct {
	TopLeft     T
	TopRight    T
	BottomRight T
	BottomLeft  T
}

// AllCorners returns uniform corners.
func AllCorners[T Number](v T) Corners[T] {
	return Corners[T]{TopLeft: v, TopRight: v, BottomRight: v, BottomLeft: v}
}

// MapCorners converts each corner value.
func MapCorners[T, U Number](c Corners[T], f func(T) U) Corners[U] {
	return Corners[U]{TopLeft: f(c.TopLeft), TopRight: f(c.TopRight), BottomRight: f(c.BottomRight), BottomLeft: f(c.BottomLeft)}
}

// ScaleCorners multiplies logical corners by a scale factor.
func ScaleCorners(c Corners[Pixels], f float32) Corners[ScaledPixels] {
	return Corners[ScaledPixels]{
		TopLeft: c.TopLeft.Scale(f), TopRight: c.TopRight.Scale(f),
		BottomRight: c.BottomRight.Scale(f), BottomLeft: c.BottomLeft.Scale(f),
	}
}

// ClampRadiiForQuadSize clamps each corner radius to half the shorter quad
// side (upstream Corners::clamp_radii_for_quad_size; this is what makes
// rounded-full produce a stadium/ellipse instead of an invalid SDF).
func ClampRadiiForQuadSize[T Number](c Corners[T], width, height T) Corners[T] {
	maxR := width
	if height < maxR {
		maxR = height
	}
	maxR = maxR / 2
	clamp := func(v T) T {
		if v > maxR {
			return maxR
		}
		return v
	}
	return Corners[T]{TopLeft: clamp(c.TopLeft), TopRight: clamp(c.TopRight), BottomRight: clamp(c.BottomRight), BottomLeft: clamp(c.BottomLeft)}
}

// ---------------------------------------------------------------------------
// Length system
// ---------------------------------------------------------------------------

// AbsoluteLength is a px or rem length.
type AbsoluteLength struct {
	kind   uint8 // 0 = pixels, 1 = rems
	pixels Pixels
	rems   Rems
}

// PixelsLength wraps Pixels as an AbsoluteLength.
func PixelsLength(p Pixels) AbsoluteLength { return AbsoluteLength{kind: 0, pixels: p} }

// RemsLength wraps Rems as an AbsoluteLength.
func RemsLength(r Rems) AbsoluteLength { return AbsoluteLength{kind: 1, rems: r} }

// IsZero reports a zero length.
func (a AbsoluteLength) IsZero() bool {
	if a.kind == 0 {
		return a.pixels.IsZero()
	}
	return a.rems.IsZero()
}

// ToPixels resolves rem lengths against the given rem size.
func (a AbsoluteLength) ToPixels(remSize Pixels) Pixels {
	if a.kind == 0 {
		return a.pixels
	}
	return a.rems.ToPixels(remSize)
}

// ToRems converts px to rems against the given rem size.
func (a AbsoluteLength) ToRems(remSize Pixels) Rems {
	if a.kind == 1 {
		return a.rems
	}
	return Rems(a.pixels.AsF32() / remSize.AsF32())
}

func (a AbsoluteLength) String() string {
	if a.kind == 0 {
		return a.pixels.String()
	}
	return a.rems.String()
}

// DefiniteLength is an absolute length or a fraction of a base size.
type DefiniteLength struct {
	kind     uint8 // 0 = absolute, 1 = fraction
	absolute AbsoluteLength
	fraction float32
}

// AbsoluteDefinite wraps an AbsoluteLength.
func AbsoluteDefinite(a AbsoluteLength) DefiniteLength { return DefiniteLength{kind: 0, absolute: a} }

// PixelsDefinite wraps Pixels.
func PixelsDefinite(p Pixels) DefiniteLength {
	return DefiniteLength{kind: 0, absolute: PixelsLength(p)}
}

// RemsDefinite wraps Rems.
func RemsDefinite(r Rems) DefiniteLength { return DefiniteLength{kind: 0, absolute: RemsLength(r)} }

// RelativeDefinite is a fraction (percentage / 100).
func RelativeDefinite(f float32) DefiniteLength { return DefiniteLength{kind: 1, fraction: f} }

// IsZero reports whether the length is zero (fractions compare to 0).
func (d DefiniteLength) IsZero() bool {
	if d.kind == 0 {
		return d.absolute.IsZero()
	}
	return d.fraction == 0
}

// IsFraction reports whether this is a fractional length.
func (d DefiniteLength) IsFraction() bool { return d.kind == 1 }

// Fraction returns the fraction if IsFraction.
func (d DefiniteLength) Fraction() (float32, bool) {
	if d.kind == 1 {
		return d.fraction, true
	}
	return 0, false
}

// Absolute returns the absolute length if not fractional.
func (d DefiniteLength) Absolute() (AbsoluteLength, bool) {
	if d.kind == 0 {
		return d.absolute, true
	}
	return AbsoluteLength{}, false
}

// ToPixels resolves the length against a base size and rem size.
func (d DefiniteLength) ToPixels(baseSize AbsoluteLength, remSize Pixels) Pixels {
	if d.kind == 0 {
		return d.absolute.ToPixels(remSize)
	}
	if baseSize.kind == 0 {
		return Px(baseSize.pixels.AsF32() * d.fraction)
	}
	return Px(float32(baseSize.rems) * remSize.AsF32() * d.fraction)
}

func (d DefiniteLength) String() string {
	if d.kind == 1 {
		return fmt.Sprintf("%d%%", int32(d.fraction*100))
	}
	return d.absolute.String()
}

// Length is a DefiniteLength or "auto".
type Length struct {
	auto     bool
	definite DefiniteLength
}

// AutoLength is the "auto" length.
func AutoLength() Length { return Length{auto: true} }

// DefiniteLengthFrom wraps a DefiniteLength.
func DefiniteLengthFrom(d DefiniteLength) Length { return Length{definite: d} }

// LengthFromPixels wraps Pixels.
func LengthFromPixels(p Pixels) Length { return Length{definite: PixelsDefinite(p)} }

// LengthFromRems wraps Rems.
func LengthFromRems(r Rems) Length { return Length{definite: RemsDefinite(r)} }

// LengthFromAbsolute wraps an AbsoluteLength.
func LengthFromAbsolute(a AbsoluteLength) Length { return Length{definite: AbsoluteDefinite(a)} }

// LengthFromDefinite wraps a DefiniteLength (alias of DefiniteLengthFrom).
func LengthFromDefinite(d DefiniteLength) Length { return DefiniteLengthFrom(d) }

// IsAuto reports the "auto" state.
func (l Length) IsAuto() bool { return l.auto }

// IsZero reports a zero length; auto is never zero.
func (l Length) IsZero() bool { return !l.auto && l.definite.IsZero() }

// Definite returns the definite part if not auto.
func (l Length) Definite() (DefiniteLength, bool) {
	if l.auto {
		return DefiniteLength{}, false
	}
	return l.definite, true
}

// ToPixels resolves the length, returning ok=false for auto.
func (l Length) ToPixels(baseSize AbsoluteLength, remSize Pixels) (Pixels, bool) {
	if l.auto {
		return PixelsZero, false
	}
	return l.definite.ToPixels(baseSize, remSize), true
}

func (l Length) String() string {
	if l.auto {
		return "auto"
	}
	return l.definite.String()
}

// Phi is the golden ratio as a relative length.
func Phi() DefiniteLength { return RelativeDefinite(1.618033988749895) }

// ParsePixels parses strings like "12px".
func ParsePixels(s string) (Pixels, error) {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, "px") {
		return PixelsZero, fmt.Errorf("invalid pixels: %q", s)
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(s, "px"), 32)
	if err != nil {
		return PixelsZero, fmt.Errorf("invalid pixels: %q", s)
	}
	return Px(float32(v)), nil
}

// ParseRems parses strings like "1.5rem".
func ParseRems(s string) (Rems, error) {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, "rem") {
		return RemsZero, fmt.Errorf("invalid rems: %q", s)
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(s, "rem"), 32)
	if err != nil {
		return RemsZero, fmt.Errorf("invalid rems: %q", s)
	}
	return Rems(float32(v)), nil
}

// ParseAbsoluteLength parses "42px" or "2rem".
func ParseAbsoluteLength(s string) (AbsoluteLength, error) {
	if strings.HasSuffix(s, "rem") {
		r, err := ParseRems(s)
		if err != nil {
			return AbsoluteLength{}, err
		}
		return RemsLength(r), nil
	}
	p, err := ParsePixels(s)
	if err != nil {
		return AbsoluteLength{}, err
	}
	return PixelsLength(p), nil
}

// ParseDefiniteLength parses "10px", "1rem" or "50%".
func ParseDefiniteLength(s string) (DefiniteLength, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 32)
		if err != nil {
			return DefiniteLength{}, fmt.Errorf("invalid length: %q", s)
		}
		return RelativeDefinite(float32(v) / 100), nil
	}
	a, err := ParseAbsoluteLength(s)
	if err != nil {
		return DefiniteLength{}, err
	}
	return AbsoluteDefinite(a), nil
}

// ParseLength parses "auto", "10px", "1rem" or "50%".
func ParseLength(s string) (Length, error) {
	s = strings.TrimSpace(s)
	if s == "auto" {
		return AutoLength(), nil
	}
	d, err := ParseDefiniteLength(s)
	if err != nil {
		return Length{}, err
	}
	return DefiniteLengthFrom(d), nil
}

// ---------------------------------------------------------------------------
// Length-valued edges / corners (dedicated structs; the Length types are
// tagged structs, not numeric units)
// ---------------------------------------------------------------------------

// EdgesDefinite is top/right/bottom/left of DefiniteLength values.
type EdgesDefinite struct {
	Top, Right, Bottom, Left DefiniteLength
}

// EdgesAbsolute is top/right/bottom/left of AbsoluteLength values.
type EdgesAbsolute struct {
	Top, Right, Bottom, Left AbsoluteLength
}

// EdgesOfLength is top/right/bottom/left of Length values.
type EdgesOfLength struct {
	Top, Right, Bottom, Left Length
}

// AutoEdgesOfLength returns all-auto edges.
func AutoEdgesOfLength() EdgesOfLength {
	return EdgesOfLength{Top: AutoLength(), Right: AutoLength(), Bottom: AutoLength(), Left: AutoLength()}
}

// ZeroEdgesOfLength returns zero (0px) Length edges.
func ZeroEdgesOfLength() EdgesOfLength {
	return EdgesOfLength{
		Top: LengthFromPixels(PixelsZero), Right: LengthFromPixels(PixelsZero),
		Bottom: LengthFromPixels(PixelsZero), Left: LengthFromPixels(PixelsZero),
	}
}

// ZeroEdgesDefinite returns zero DefiniteLength edges.
func ZeroEdgesDefinite() EdgesDefinite {
	return EdgesDefinite{
		Top: PixelsDefinite(PixelsZero), Right: PixelsDefinite(PixelsZero),
		Bottom: PixelsDefinite(PixelsZero), Left: PixelsDefinite(PixelsZero),
	}
}

// ZeroEdgesAbsolute returns zero AbsoluteLength edges.
func ZeroEdgesAbsolute() EdgesAbsolute {
	return EdgesAbsolute{
		Top: PixelsLength(PixelsZero), Right: PixelsLength(PixelsZero),
		Bottom: PixelsLength(PixelsZero), Left: PixelsLength(PixelsZero),
	}
}

// Resolve resolves definite edges: top/bottom against parent height,
// left/right against parent width (taffy percentage semantics).
func (e EdgesDefinite) Resolve(parentWidth, parentHeight AbsoluteLength, remSize Pixels) Edges[Pixels] {
	return Edges[Pixels]{
		Top:    e.Top.ToPixels(parentHeight, remSize),
		Right:  e.Right.ToPixels(parentWidth, remSize),
		Bottom: e.Bottom.ToPixels(parentHeight, remSize),
		Left:   e.Left.ToPixels(parentWidth, remSize),
	}
}

// ToPixels resolves absolute edges against a rem size.
func (e EdgesAbsolute) ToPixels(remSize Pixels) Edges[Pixels] {
	return Edges[Pixels]{
		Top:    e.Top.ToPixels(remSize),
		Right:  e.Right.ToPixels(remSize),
		Bottom: e.Bottom.ToPixels(remSize),
		Left:   e.Left.ToPixels(remSize),
	}
}

// MaxEdge returns the maximum edge value.
func MaxEdge(e Edges[Pixels]) Pixels {
	return e.Top.Max(e.Right.Max(e.Bottom.Max(e.Left)))
}

// CornersAbsolute is the four corner AbsoluteLength values.
type CornersAbsolute struct {
	TopLeft, TopRight, BottomRight, BottomLeft AbsoluteLength
}

// ZeroCornersAbsolute returns zero corners.
func ZeroCornersAbsolute() CornersAbsolute {
	return CornersAbsolute{
		TopLeft: PixelsLength(PixelsZero), TopRight: PixelsLength(PixelsZero),
		BottomRight: PixelsLength(PixelsZero), BottomLeft: PixelsLength(PixelsZero),
	}
}

// ToPixels resolves absolute corners against a rem size.
func (c CornersAbsolute) ToPixels(remSize Pixels) Corners[Pixels] {
	return Corners[Pixels]{
		TopLeft:     c.TopLeft.ToPixels(remSize),
		TopRight:    c.TopRight.ToPixels(remSize),
		BottomRight: c.BottomRight.ToPixels(remSize),
		BottomLeft:  c.BottomLeft.ToPixels(remSize),
	}
}

// MaxCorner returns the maximum corner value.
func MaxCorner(c Corners[Pixels]) Pixels {
	return c.TopLeft.Max(c.TopRight.Max(c.BottomRight.Max(c.BottomLeft)))
}

// ClampCornerRadii clamps each corner radius to half the shorter side.
func ClampCornerRadii(c Corners[Pixels], size Size[Pixels]) Corners[Pixels] {
	max := Px(float32(math.Min(float64(size.Width.AsF32()), float64(size.Height.AsF32()))) / 2)
	return Corners[Pixels]{
		TopLeft:     c.TopLeft.Min(max),
		TopRight:    c.TopRight.Min(max),
		BottomRight: c.BottomRight.Min(max),
		BottomLeft:  c.BottomLeft.Min(max),
	}
}

// ---------------------------------------------------------------------------
// Grid (ported for API compatibility; grid layout itself is not yet ported)
// ---------------------------------------------------------------------------

// GridPlacement mirrors taffy's GridPlacement.
type GridPlacement struct {
	kind uint8 // 0 auto, 1 line, 2 span
	line int16
	span uint16
}

// GridPlacementAuto is the auto placement.
var GridPlacementAuto = GridPlacement{kind: 0}

// GridPlacementLine places at a grid line (negative counts from the end).
func GridPlacementLine(line int16) GridPlacement { return GridPlacement{kind: 1, line: line} }

// GridPlacementSpan spans the given number of tracks.
func GridPlacementSpan(span uint16) GridPlacement { return GridPlacement{kind: 2, span: span} }

// GridLocation describes a row/column range.
type GridLocation struct {
	Start GridPlacement
	End   GridPlacement
}

// ---------------------------------------------------------------------------
// TransformationMatrix
// ---------------------------------------------------------------------------

// TransformationMatrix is a 2D affine transform: a 2x2 rotation/scale matrix
// (row-major) plus a translation.
type TransformationMatrix struct {
	RotationScale [2][2]float32
	Translation   [2]float32
}

// IdentityMatrix returns the unit transform.
func IdentityMatrix() TransformationMatrix {
	return TransformationMatrix{RotationScale: [2][2]float32{{1, 0}, {0, 1}}}
}

// Translate returns a translation matrix.
func Translate(p Point[ScaledPixels]) TransformationMatrix {
	return TransformationMatrix{
		RotationScale: [2][2]float32{{1, 0}, {0, 1}},
		Translation:   [2]float32{p.X.AsF32(), p.Y.AsF32()},
	}
}

// Rotate returns a clockwise rotation matrix.
func Rotate(angle Radians) TransformationMatrix {
	c := float32(math.Cos(float64(angle)))
	s := float32(math.Sin(float64(angle)))
	return TransformationMatrix{RotationScale: [2][2]float32{{c, -s}, {s, c}}}
}

// ScaleMatrix returns a scale matrix.
func ScaleMatrix(s Size[float32]) TransformationMatrix {
	return TransformationMatrix{RotationScale: [2][2]float32{{s.Width, 0}, {0, s.Height}}}
}

// Compose returns self × other; other is applied first, then self.
func (m TransformationMatrix) Compose(other TransformationMatrix) TransformationMatrix {
	if other == IdentityMatrix() {
		return m
	}
	var out TransformationMatrix
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			out.RotationScale[i][j] =
				m.RotationScale[i][0]*other.RotationScale[0][j] +
					m.RotationScale[i][1]*other.RotationScale[1][j]
		}
	}
	out.Translation[0] = m.Translation[0] +
		m.RotationScale[0][0]*other.Translation[0] +
		m.RotationScale[0][1]*other.Translation[1]
	out.Translation[1] = m.Translation[1] +
		m.RotationScale[1][0]*other.Translation[0] +
		m.RotationScale[1][1]*other.Translation[1]
	return out
}

// TransformBounds transforms all four corners and takes the min/max envelope.
func (m TransformationMatrix) TransformBounds(b Bounds[ScaledPixels]) Bounds[ScaledPixels] {
	corners := []Point[float32]{
		{X: b.Origin.X.AsF32(), Y: b.Origin.Y.AsF32()},
		{X: b.Origin.X.AsF32() + b.Size.Width.AsF32(), Y: b.Origin.Y.AsF32()},
		{X: b.Origin.X.AsF32(), Y: b.Origin.Y.AsF32() + b.Size.Height.AsF32()},
		{X: b.Origin.X.AsF32() + b.Size.Width.AsF32(), Y: b.Origin.Y.AsF32() + b.Size.Height.AsF32()},
	}
	minX, minY := float32(math.MaxFloat32), float32(math.MaxFloat32)
	maxX, maxY := float32(-math.MaxFloat32), float32(-math.MaxFloat32)
	for _, c := range corners {
		x := m.RotationScale[0][0]*c.X + m.RotationScale[0][1]*c.Y + m.Translation[0]
		y := m.RotationScale[1][0]*c.X + m.RotationScale[1][1]*c.Y + m.Translation[1]
		if x < minX {
			minX = x
		}
		if y < minY {
			minY = y
		}
		if x > maxX {
			maxX = x
		}
		if y > maxY {
			maxY = y
		}
	}
	return Bounds[ScaledPixels]{
		Origin: Point[ScaledPixels]{X: ScaledPx(minX), Y: ScaledPx(minY)},
		Size:   Size[ScaledPixels]{Width: ScaledPx(maxX - minX), Height: ScaledPx(maxY - minY)},
	}
}

// Apply transforms a logical point.
func (m TransformationMatrix) Apply(p Point[Pixels]) Point[Pixels] {
	x := p.X.AsF32()
	y := p.Y.AsF32()
	return Point[Pixels]{
		X: Px(m.RotationScale[0][0]*x + m.RotationScale[0][1]*y + m.Translation[0]),
		Y: Px(m.RotationScale[1][0]*x + m.RotationScale[1][1]*y + m.Translation[1]),
	}
}

// ---------------------------------------------------------------------------
// Rounding helpers (port of gpui's util.rs)
// ---------------------------------------------------------------------------

// RoundHalfTowardZero rounds ties toward zero (0.5->0, 1.5->1, -0.5->0).
func RoundHalfTowardZero(v float32) float32 {
	s := float32(1)
	if v < 0 {
		s = -1
	}
	av := v * s
	return s * float32(math.Ceil(float64(av)-0.5))
}

// RoundToDevicePixel snaps a logical value to the nearest device pixel and
// returns it in logical units (upstream round_to_device_pixels): the result
// lands on the device-pixel grid after scaling. Layout and hit-testing work
// in logical pixels, so this keeps them aligned with painted device pixels.
func RoundToDevicePixel(logical, scaleFactor float32) float32 {
	if scaleFactor == 0 || scaleFactor == 1 {
		return RoundHalfTowardZero(logical)
	}
	return RoundHalfTowardZero(logical*scaleFactor) / scaleFactor
}

// RoundStrokeToDevicePixel snaps stroke widths the same way, clamping
// positive values below one device pixel up to one device pixel (returned
// in logical units).
func RoundStrokeToDevicePixel(logical, scaleFactor float32) float32 {
	if logical == 0 {
		return 0
	}
	if scaleFactor <= 0 {
		return logical
	}
	if logical < 0 {
		return 0
	}
	v := RoundToDevicePixel(logical, scaleFactor)
	if v <= 0 {
		return 1 / scaleFactor
	}
	return v
}

// FloorToDevicePixel floors a logical value into device space.
func FloorToDevicePixel(logical, scaleFactor float32) float32 {
	return float32(math.Floor(float64(logical * scaleFactor)))
}

// CeilToDevicePixel ceils a logical value into device space.
func CeilToDevicePixel(logical, scaleFactor float32) float32 {
	return float32(math.Ceil(float64(logical * scaleFactor)))
}

// AsF32 returns the raw rem value.
func (r Rems) AsF32() float32 { return float32(r) }

// AsF32 returns the raw radians value.
func (r Radians) AsF32() float32 { return float32(r) }
