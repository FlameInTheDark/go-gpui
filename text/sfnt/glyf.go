package sfnt

import (
	"encoding/binary"
)

// Outline is a glyph outline: contours of quadratic segments.
type Outline struct {
	// Points are contour points. OnCurve marks on-curve points; off-curve
	// points are quadratic control points.
	Points  []Point
	OnCurve []bool
	// Ends holds the end index (inclusive) of each contour.
	Ends []int
	// Bounds.
	XMin, YMin, XMax, YMax float32
}

// Point is one outline point in font units.
type Point struct{ X, Y float32 }

// GlyphOutline returns the TrueType outline of a glyph (empty for CFF or
// missing glyphs).
func (f *Font) GlyphOutline(gid uint16) Outline {
	if f.isCFF && f.cff != nil {
		return f.cff.GlyphOutline(gid)
	}
	return f.glyfOutline(gid)
}

func (f *Font) glyphRange(gid uint16) (int, int) {
	if f.loca == nil || f.glyf == nil {
		return 0, 0
	}
	var start, end uint32
	if f.IndexToLocFormat == 0 {
		off := int(gid) * 2
		if off+4 > len(f.loca) {
			return 0, 0
		}
		start = 2 * uint32(binary.BigEndian.Uint16(f.loca[off:]))
		endOff := int(gid)*2 + 2
		if endOff+2 > len(f.loca) {
			return 0, 0
		}
		end = 2 * uint32(binary.BigEndian.Uint16(f.loca[endOff:]))
	} else {
		off := int(gid) * 4
		if off+8 > len(f.loca) {
			return 0, 0
		}
		start = binary.BigEndian.Uint32(f.loca[off:])
		end = binary.BigEndian.Uint32(f.loca[off+4:])
	}
	if int(start) >= len(f.glyf) || end < start || int(end) > len(f.glyf) {
		return 0, 0
	}
	return int(start), int(end)
}

func (f *Font) glyfOutline(gid uint16) Outline {
	start, end := f.glyphRange(gid)
	if start == end {
		return Outline{}
	}
	g := f.glyf[start:end]
	if len(g) < 10 {
		return Outline{}
	}
	numberOfContours := int(int16(binary.BigEndian.Uint16(g[0:2])))
	var out Outline
	out.XMin = float32(int16(binary.BigEndian.Uint16(g[2:4])))
	out.YMin = float32(int16(binary.BigEndian.Uint16(g[4:6])))
	out.XMax = float32(int16(binary.BigEndian.Uint16(g[6:8])))
	out.YMax = float32(int16(binary.BigEndian.Uint16(g[8:10])))

	if numberOfContours >= 0 {
		// Simple glyph.
		if len(g) < 10+2*numberOfContours {
			return Outline{}
		}
		ends := make([]int, numberOfContours)
		for i := 0; i < numberOfContours; i++ {
			ends[i] = int(binary.BigEndian.Uint16(g[10+2*i:]))
		}
		pos := 10 + 2*numberOfContours
		if pos+2 > len(g) {
			return Outline{}
		}
		// Skip the instructions.
		instructionLength := int(binary.BigEndian.Uint16(g[pos : pos+2]))
		pos += 2 + instructionLength
		nPoints := ends[len(ends)-1] + 1
		// Flags.
		flags := make([]byte, 0, nPoints)
		for len(flags) < nPoints && pos < len(g) {
			flag := g[pos]
			pos++
			flags = append(flags, flag)
			if flag&0x08 != 0 { // repeat
				if pos >= len(g) {
					return Outline{}
				}
				repeat := int(g[pos])
				pos++
				for r := 0; r < repeat && len(flags) < nPoints; r++ {
					flags = append(flags, flag)
				}
			}
		}
		// X coordinates.
		xs := make([]float32, nPoints)
		x := int32(0)
		for i := 0; i < nPoints; i++ {
			flag := flags[i]
			if flag&0x02 != 0 {
				if pos >= len(g) {
					return Outline{}
				}
				d := int32(g[pos])
				pos++
				if flag&0x10 == 0 {
					d = -d
				}
				x += d
			} else if flag&0x10 == 0 {
				if pos+2 > len(g) {
					return Outline{}
				}
				x += int32(int16(binary.BigEndian.Uint16(g[pos:])))
				pos += 2
			}
			xs[i] = float32(x)
		}
		// Y coordinates.
		ys := make([]float32, nPoints)
		y := int32(0)
		for i := 0; i < nPoints; i++ {
			flag := flags[i]
			if flag&0x04 != 0 {
				if pos >= len(g) {
					return Outline{}
				}
				d := int32(g[pos])
				pos++
				if flag&0x20 == 0 {
					d = -d
				}
				y += d
			} else if flag&0x20 == 0 {
				if pos+2 > len(g) {
					return Outline{}
				}
				y += int32(int16(binary.BigEndian.Uint16(g[pos:])))
				pos += 2
			}
			ys[i] = float32(y)
		}
		out.Points = make([]Point, nPoints)
		out.OnCurve = make([]bool, nPoints)
		for i := 0; i < nPoints; i++ {
			out.Points[i] = Point{X: xs[i], Y: ys[i]}
			out.OnCurve[i] = flags[i]&0x01 != 0
		}
		out.Ends = ends
		return out
	}

	// Composite glyph.
	var points []Point
	var onCurve []bool
	var ends []int
	for len(g) >= 6 {
		flags := int(binary.BigEndian.Uint16(g[0:2]))
		glyphIdx := binary.BigEndian.Uint16(g[2:4])
		p := 4
		var dx, dy float32
		var arg1, arg2 float32
		if flags&0x0001 != 0 { // ARG_1_AND_2_ARE_WORDS
			if p+4 > len(g) {
				break
			}
			arg1 = float32(int16(binary.BigEndian.Uint16(g[p:])))
			arg2 = float32(int16(binary.BigEndian.Uint16(g[p+2:])))
			p += 4
		} else {
			if p+2 > len(g) {
				break
			}
			arg1 = float32(int8(g[p]))
			arg2 = float32(int8(g[p+1]))
			p += 2
		}
		if flags&0x0002 != 0 { // ARGS_ARE_XY_VALUES
			dx, dy = arg1, arg2
		}
		// Point-matching components are rare; args are consumed above
		// and the matching itself is unsupported.
		var sx, sy, s01, s10 float32 = 1, 1, 0, 0
		f2dot14 := func(off int) float32 {
			if off+2 > len(g) {
				return 0
			}
			return float32(int16(binary.BigEndian.Uint16(g[off:]))) / 16384
		}
		if flags&0x0008 != 0 { // WE_HAVE_A_SCALE
			sx = f2dot14(p)
			sy = sx
			p += 2
		} else if flags&0x0040 != 0 { // WE_HAVE_AN_X_AND_Y_SCALE
			sx = f2dot14(p)
			sy = f2dot14(p + 2)
			p += 4
		} else if flags&0x0080 != 0 { // WE_HAVE_A_TWO_BY_TWO
			sx = f2dot14(p)      // xScale
			s01 = f2dot14(p + 2) // scale01
			s10 = f2dot14(p + 4) // scale10
			sy = f2dot14(p + 6)  // yScale
			p += 8
		}
		sub := f.glyfOutline(glyphIdx)
		for i, pt := range sub.Points {
			points = append(points, Point{
				X: sx*pt.X + s01*pt.Y + dx,
				Y: s10*pt.X + sy*pt.Y + dy,
			})
			onCurve = append(onCurve, sub.OnCurve[i])
		}
		for _, e := range sub.Ends {
			ends = append(ends, e+len(points)-len(sub.Points))
		}
		if flags&0x0020 == 0 { // MORE_COMPONENTS
			g = g[p:]
			continue
		}
		break
	}
	out.Points = points
	out.OnCurve = onCurve
	out.Ends = ends
	if len(points) > 0 {
		minX, minY := points[0].X, points[0].Y
		maxX, maxY := minX, minY
		for _, pt := range points {
			if pt.X < minX {
				minX = pt.X
			}
			if pt.Y < minY {
				minY = pt.Y
			}
			if pt.X > maxX {
				maxX = pt.X
			}
			if pt.Y > maxY {
				maxY = pt.Y
			}
		}
		out.XMin, out.YMin, out.XMax, out.YMax = minX, minY, maxX, maxY
	}
	return out
}
