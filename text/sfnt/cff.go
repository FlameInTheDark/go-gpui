package sfnt

import (
	"encoding/binary"
)

// CFF is a parsed Compact Font Format table (metrics + Type2 charstrings).
type CFF struct {
	header      []byte
	nameIndex   []byte
	topIndex    []byte
	stringIndex []byte
	gsubrIndex  []byte
	// Per-font data (first font only).
	topDICT                      []byte
	charStrings                  []byte
	privateDict                  []byte
	subrIndex                    []byte
	defaultWidthX, nominalWidthX float64
}

func parseCFF(data []byte) (*CFF, error) {
	if len(data) < 4 {
		return nil, errCFF("short")
	}
	hdrSize := int(data[2])
	pos := hdrSize
	nameIndex, n, err := readIndex(data, pos)
	if err != nil {
		return nil, err
	}
	pos = n
	topIndex, n, err := readIndex(data, pos)
	if err != nil {
		return nil, err
	}
	pos = n
	stringIndex, n, err := readIndex(data, pos)
	if err != nil {
		return nil, err
	}
	pos = n
	gsubrIndex, n, err := readIndex(data, pos)
	if err != nil {
		return nil, err
	}
	pos = n
	_ = pos

	// First font's top DICT.
	topData, err := indexAt(topIndex, 0)
	if err != nil {
		return nil, err
	}
	cff := &CFF{
		header:      data[:hdrSize],
		nameIndex:   nameIndex,
		topIndex:    topIndex,
		stringIndex: stringIndex,
		gsubrIndex:  gsubrIndex,
		topDICT:     topData,
	}
	// Parse the top DICT operands we need.
	var charStringsOff, privateOff, privateLen float64
	operands := []float64{}
	for i := 0; i < len(topData); {
		op, size, vals, err := readDICTOperator(topData, i)
		if err != nil {
			break
		}
		i += size
		_ = op
		operands = append(operands[:0], vals...)
		if len(vals) == 0 {
			continue
		}
	}
	// Simpler: scan operators.
	operands = operands[:0]
	posD := 0
	for posD < len(topData) {
		b := topData[posD]
		if b <= 21 { // operator
			var vals []float64
			vals, posD = dictOperandsBefore(topData, posD)
			switch b {
			case 17: // CharStrings
				if len(vals) > 0 {
					charStringsOff = vals[len(vals)-1]
				}
			case 18: // Private
				if len(vals) >= 2 {
					privateLen = vals[0]
					privateOff = vals[1]
				}
			}
			if b == 12 { // escaped operator
				posD++
			}
			posD++
		} else {
			// Skip operand.
			_, n, err := readDICTOperand(topData, posD)
			if err != nil {
				break
			}
			posD = n
		}
	}
	if int(charStringsOff) < len(data) {
		cs, _, err := readIndex(data, int(charStringsOff))
		if err == nil {
			cff.charStrings = cs
		}
	}
	if int(privateOff) < len(data) && int(privateOff)+int(privateLen) <= len(data) {
		cff.privateDict = data[int(privateOff) : int(privateOff)+int(privateLen)]
		// Parse Subrs offset (operator 19) and widths.
		p := 0
		var subrsOff float64 = -1
		for p < len(cff.privateDict) {
			bb := cff.privateDict[p]
			if bb <= 21 {
				vals, np := dictOperandsBefore(cff.privateDict, p)
				switch bb {
				case 19:
					if len(vals) > 0 {
						subrsOff = vals[len(vals)-1]
					}
				case 20:
					if len(vals) > 0 {
						cff.defaultWidthX = vals[len(vals)-1]
					}
				case 21:
					if len(vals) > 0 {
						cff.nominalWidthX = vals[len(vals)-1]
					}
				}
				if bb == 12 {
					np++
				}
				p = np + 1
			} else {
				_, n, err := readDICTOperand(cff.privateDict, p)
				if err != nil {
					break
				}
				p = n
			}
		}
		if subrsOff >= 0 && int(privateOff)+int(subrsOff) < len(data) {
			subr, _, err := readIndex(data, int(privateOff)+int(subrsOff))
			if err == nil {
				cff.subrIndex = subr
			}
		}
	}
	return cff, nil
}

type errCFF string

func (e errCFF) Error() string { return "cff: " + string(e) }

// readIndex reads a CFF INDEX at pos, returning its bytes and the end
// position.
func readIndex(data []byte, pos int) ([]byte, int, error) {
	if pos+2 > len(data) {
		return nil, 0, errCFF("index oob")
	}
	count := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	if count == 0 {
		return data[pos : pos+2], pos + 2, nil
	}
	if pos+3 > len(data) {
		return nil, 0, errCFF("index oob")
	}
	offSize := int(data[pos+2])
	if offSize < 1 || offSize > 4 {
		return nil, 0, errCFF("bad offSize")
	}
	offBase := pos + 3
	numOffsets := count + 1
	if offBase+offSize*numOffsets > len(data) {
		return nil, 0, errCFF("index oob")
	}
	readOff := func(i int) int {
		v := 0
		for j := 0; j < offSize; j++ {
			v = v<<8 | int(data[offBase+offSize*i+j])
		}
		return v
	}
	start := offBase + offSize*numOffsets - 1 + readOff(0)
	end := offBase + offSize*numOffsets - 1 + readOff(count)
	if end > len(data) || start > end {
		return nil, 0, errCFF("index data oob")
	}
	// Return the entire INDEX (header + data) so indexAt can re-read the
	// offset table; endIndex advances past the data.
	return data[pos:end], end, nil
}

func indexAt(index []byte, i int) ([]byte, error) {
	if len(index) < 2 {
		return nil, errCFF("empty index")
	}
	count := int(binary.BigEndian.Uint16(index[0:2]))
	if count == 0 || i >= count {
		return nil, errCFF("index range")
	}
	offSize := int(index[2])
	offBase := 3
	readOff := func(k int) int {
		v := 0
		for j := 0; j < offSize; j++ {
			v = v<<8 | int(index[offBase+offSize*k+j])
		}
		return v
	}
	start := offBase + offSize*(count+1) - 1 + readOff(i)
	end := offBase + offSize*(count+1) - 1 + readOff(i+1)
	if end > len(index) || start > end {
		return nil, errCFF("index data range")
	}
	return index[start:end], nil
}

func readDICTOperand(b []byte, pos int) (float64, int, error) {
	if pos >= len(b) {
		return 0, 0, errCFF("dict oob")
	}
	v := b[pos]
	switch {
	case v <= 21:
		return 0, pos, nil // operator handled by caller
	case v == 28:
		if pos+3 > len(b) {
			return 0, 0, errCFF("dict oob")
		}
		return float64(int16(binary.BigEndian.Uint16(b[pos+1:]))), pos + 3, nil
	case v == 29:
		if pos+5 > len(b) {
			return 0, 0, errCFF("dict oob")
		}
		return float64(int32(binary.BigEndian.Uint32(b[pos+1:]))), pos + 5, nil
	case v == 30: // real
		// Parse a real number; we only need integral values here.
		var mant float64
		var frac float64
		var fracDiv = float64(1)
		neg := false
		p := pos + 1
		done := false
		for p < len(b) && !done {
			nib := b[p]
			for _, half := range []byte{nib >> 4, nib & 0xF} {
				switch {
				case half <= 9:
					if fracDiv == 1 {
						mant = mant*10 + float64(half)
					} else {
						frac += float64(half) * fracDiv
						fracDiv /= 10
					}
				case half == 0xa: // .
					fracDiv = 0.1
				case half == 0xb: // E
				case half == 0xc: // E-
				case half == 0xe: // -
					neg = !neg
				case half == 0xf:
					done = true
				}
			}
			p++
		}
		val := mant + frac
		if neg {
			val = -val
		}
		return val, p, nil
	case v >= 32 && v <= 246:
		return float64(int32(v) - 139), pos + 1, nil
	case v >= 247 && v <= 250:
		if pos+2 > len(b) {
			return 0, 0, errCFF("dict oob")
		}
		return float64(int32(v-247)*256 + int32(b[pos+1]) + 108), pos + 2, nil
	case v >= 251 && v <= 254:
		if pos+2 > len(b) {
			return 0, 0, errCFF("dict oob")
		}
		return float64(-(int32(v-251)*256 + int32(b[pos+1]) + 108)), pos + 2, nil
	}
	return 0, 0, errCFF("bad operand")
}

func readDICTOperator(b []byte, pos int) (byte, int, []float64, error) {
	return 0, 0, nil, errCFF("unused")
}

// dictOperandsBefore returns the operands that precede the operator at pos
// and the operand start.
func dictOperandsBefore(b []byte, opPos int) ([]float64, int) {
	var vals []float64
	p := 0
	for p < opPos {
		if b[p] <= 21 {
			// A previous operator: reset.
			if b[p] == 12 {
				p++
			}
			p++
			vals = vals[:0]
			continue
		}
		v, n, err := readDICTOperand(b, p)
		if err != nil || n <= p {
			break
		}
		vals = append(vals, v)
		p = n
	}
	return vals, p
}

// GlyphOutline runs the Type2 charstring for a glyph.
func (c *CFF) GlyphOutline(gid uint16) Outline {
	cs, err := indexAt(c.charStrings, int(gid))
	if err != nil {
		return Outline{}
	}
	st := &type2State{cff: c, code: cs}
	st.run()
	// Close open contour.
	if len(st.points) > 0 && len(st.ends) == 0 {
		st.ends = append(st.ends, len(st.points)-1)
	}
	out := Outline{Points: st.points, OnCurve: st.onCurve, Ends: st.ends}
	if len(st.points) > 0 {
		minX, minY := st.points[0].X, st.points[0].Y
		maxX, maxY := minX, minY
		for _, pt := range st.points {
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

type type2State struct {
	cff         *CFF
	code        []byte
	pos         int
	stack       []float64
	x, y        float64
	nStems      int
	hint        bool
	open        bool
	points      []Point
	onCurve     []bool
	ends        []int
	widthParsed bool
}

func (s *type2State) subr(index []byte, bias int, idx int) []byte {
	b, err := indexAt(index, idx+bias)
	if err != nil {
		return nil
	}
	return b
}

func (s *type2State) moveTo(x, y float64) {
	if s.open {
		s.ends = append(s.ends, len(s.points)-1)
	}
	s.x, s.y = x, y
	s.points = append(s.points, Point{X: float32(x), Y: float32(y)})
	s.onCurve = append(s.onCurve, true)
	s.open = true
}

func (s *type2State) lineTo(x, y float64) {
	s.x, s.y = x, y
	s.points = append(s.points, Point{X: float32(x), Y: float32(y)})
	s.onCurve = append(s.onCurve, true)
}

func (s *type2State) curveTo(x1, y1, x2, y2, x3, y3 float64) {
	s.points = append(s.points,
		Point{X: float32(x1), Y: float32(y1)},
		Point{X: float32(x2), Y: float32(y2)},
	)
	s.onCurve = append(s.onCurve, false, false)
	s.x, s.y = x3, y3
	s.points = append(s.points, Point{X: float32(x3), Y: float32(y3)})
	s.onCurve = append(s.onCurve, true)
}

func (s *type2State) run() {
	s.runCode(s.code)
	if s.open {
		s.ends = append(s.ends, len(s.points)-1)
		s.open = false
	}
}

func (s *type2State) runCode(code []byte) {
	savedCode, savedPos := s.code, s.pos
	s.code, s.pos = code, 0
	for s.pos < len(s.code) {
		b := s.code[s.pos]
		s.pos++
		switch {
		case b >= 32 && b <= 246:
			s.stack = append(s.stack, float64(b-139))
		case b == 28:
			s.pushInt16()
		case b == 255:
			if s.pos+4 > len(s.code) {
				return
			}
			v := float64(int32(binary.BigEndian.Uint32(s.code[s.pos:]))) / 65536
			s.pos += 4
			s.stack = append(s.stack, v)
		case b >= 247 && b <= 250:
			if s.pos >= len(s.code) {
				return
			}
			s.stack = append(s.stack, float64(b-247)*256+float64(s.code[s.pos])+108)
			s.pos++
		case b >= 251 && b <= 254:
			if s.pos >= len(s.code) {
				return
			}
			s.stack = append(s.stack, -(float64(b-251)*256 + float64(s.code[s.pos]) + 108))
			s.pos++
		case b == 1, b == 3, b == 18, b == 23: // hstem vstem hstemhm vstemhm
			s.clearWidth()
			s.nStems += len(s.stack) / 2
			s.stack = s.stack[:0]
		case b == 19, b == 20: // hintmask cntrmask
			s.clearWidth()
			s.nStems += len(s.stack) / 2
			s.stack = s.stack[:0]
			s.pos += (s.nStems + 7) / 8
		case b == 10: // callsubr
			if len(s.stack) == 0 {
				return
			}
			idx := int(s.stack[len(s.stack)-1])
			s.stack = s.stack[:len(s.stack)-1]
			var subrs []byte
			bias := 32768
			if c := s.cff; c != nil {
				subrs = c.subrIndex
				if subrs != nil && len(subrs) >= 2 {
					n := int(binary.BigEndian.Uint16(subrs[0:2]))
					switch {
					case n < 1240:
						bias = 107
					case n < 33900:
						bias = 1131
					}
				}
			}
			if sub := s.subr(subrs, bias, idx); sub != nil {
				s.runCode(sub)
			}
		case b == 29: // callgsubr
			if len(s.stack) == 0 {
				return
			}
			idx := int(s.stack[len(s.stack)-1])
			s.stack = s.stack[:len(s.stack)-1]
			bias := 32768
			var subrs []byte
			if c := s.cff; c != nil && c.gsubrIndex != nil && len(c.gsubrIndex) >= 2 {
				subrs = c.gsubrIndex
				n := int(binary.BigEndian.Uint16(subrs[0:2]))
				switch {
				case n < 1240:
					bias = 107
				case n < 33900:
					bias = 1131
				}
			}
			if sub := s.subr(subrs, bias, idx); sub != nil {
				s.runCode(sub)
			}
		case b == 11: // return
			s.code, s.pos = savedCode, savedPos
			return
		case b == 14: // endchar
			s.clearWidth()
			if s.open {
				s.ends = append(s.ends, len(s.points)-1)
				s.open = false
			}
			s.code, s.pos = savedCode, savedPos
			return
		case b == 21: // rmoveto
			s.clearWidth()
			if len(s.stack) >= 2 {
				s.moveTo(s.x+s.stack[len(s.stack)-2], s.y+s.stack[len(s.stack)-1])
			}
			s.stack = s.stack[:0]
		case b == 22: // hmoveto
			s.clearWidth()
			if len(s.stack) >= 1 {
				s.moveTo(s.x+s.stack[len(s.stack)-1], s.y)
			}
			s.stack = s.stack[:0]
		case b == 4: // vmoveto
			s.clearWidth()
			if len(s.stack) >= 1 {
				s.moveTo(s.x, s.y+s.stack[len(s.stack)-1])
			}
			s.stack = s.stack[:0]
		case b == 5: // rlineto
			for i := 0; i+1 < len(s.stack); i += 2 {
				s.lineTo(s.x+s.stack[i], s.y+s.stack[i+1])
			}
			s.stack = s.stack[:0]
		case b == 6: // hlineto
			h := true
			for i := 0; i < len(s.stack); i++ {
				if h {
					s.lineTo(s.x+s.stack[i], s.y)
				} else {
					s.lineTo(s.x, s.y+s.stack[i])
				}
				h = !h
			}
			s.stack = s.stack[:0]
		case b == 7: // vlineto
			h := false
			for i := 0; i < len(s.stack); i++ {
				if h {
					s.lineTo(s.x+s.stack[i], s.y)
				} else {
					s.lineTo(s.x, s.y+s.stack[i])
				}
				h = !h
			}
			s.stack = s.stack[:0]
		case b == 8: // rrcurveto
			for i := 0; i+5 < len(s.stack); i += 6 {
				x1, y1 := s.x+s.stack[i], s.y+s.stack[i+1]
				x2, y2 := s.x+s.stack[i+2], s.y+s.stack[i+3]
				x3, y3 := s.x+s.stack[i+4], s.y+s.stack[i+5]
				s.curveTo(x1, y1, x2, y2, x3, y3)
			}
			s.stack = s.stack[:0]
		case b == 24: // rcurveline
			for i := 0; i+5 < len(s.stack)-2; i += 6 {
				x1, y1 := s.x+s.stack[i], s.y+s.stack[i+1]
				x2, y2 := s.x+s.stack[i+2], s.y+s.stack[i+3]
				x3, y3 := s.x+s.stack[i+4], s.y+s.stack[i+5]
				s.curveTo(x1, y1, x2, y2, x3, y3)
			}
			n := len(s.stack)
			if n >= 2 {
				s.lineTo(s.x+s.stack[n-2], s.y+s.stack[n-1])
			}
			s.stack = s.stack[:0]
		case b == 25: // rlinecurve
			for i := 0; i+1 < len(s.stack)-6; i += 2 {
				s.lineTo(s.x+s.stack[i], s.y+s.stack[i+1])
			}
			n := len(s.stack)
			if n >= 6 {
				i := n - 6
				s.curveTo(s.x+s.stack[i], s.y+s.stack[i+1],
					s.x+s.stack[i+2], s.y+s.stack[i+3],
					s.x+s.stack[i+4], s.y+s.stack[i+5])
			}
			s.stack = s.stack[:0]
		case b == 26: // vvcurveto
			i := 0
			var dx0 float64
			if len(s.stack)%4 == 1 {
				dx0 = s.stack[0]
				i = 1
			}
			for ; i+3 < len(s.stack); i += 4 {
				// (d1, a, b, d2): control1 = (cx+dx0, cy+d1),
				// control2 = (c1x+a, c1y+b), end = (c2x, c2y+d2).
				x1 := s.x + dx0
				y1 := s.y + s.stack[i]
				x2 := x1 + s.stack[i+1]
				y2 := y1 + s.stack[i+2]
				x3 := x2
				y3 := y2 + s.stack[i+3]
				s.curveTo(x1, y1, x2, y2, x3, y3)
				dx0 = 0
			}
			s.stack = s.stack[:0]
		case b == 27: // hhcurveto
			i := 0
			var dy0 float64
			if len(s.stack)%4 == 1 {
				dy0 = s.stack[0]
				i = 1
			}
			for ; i+3 < len(s.stack); i += 4 {
				x1 := s.x + s.stack[i]
				y1 := s.y + dy0
				x2 := x1 + s.stack[i+1]
				y2 := y1 + s.stack[i+2]
				x3 := x2 + s.stack[i+3]
				y3 := y2
				s.curveTo(x1, y1, x2, y2, x3, y3)
				dy0 = 0
			}
			s.stack = s.stack[:0]
		case b == 30, b == 31: // vhcurveto / hvcurveto
			vertical := b == 30
			i := 0
			for i+3 < len(s.stack) {
				if vertical {
					x1, y1 := s.x, s.y+s.stack[i]
					x2, y2 := s.x+s.stack[i+1], s.y+s.stack[i+2]
					x3, y3 := s.x+s.stack[i+3], s.y
					if i+4 == len(s.stack) {
						y3 = s.y + s.stack[i+3]
					}
					s.curveTo(x1, y1, x2, y2, x3, y3)
				} else {
					x1, y1 := s.x+s.stack[i], s.y
					x2, y2 := s.x+s.stack[i+1], s.y+s.stack[i+2]
					x3, y3 := s.x, s.y+s.stack[i+3]
					if i+4 == len(s.stack) {
						x3 = s.x + s.stack[i+3]
					}
					s.curveTo(x1, y1, x2, y2, x3, y3)
				}
				vertical = !vertical
				i += 4
			}
			s.stack = s.stack[:0]
		}
	}
	s.code, s.pos = savedCode, savedPos
}

func (s *type2State) pushInt16() {
	if s.pos+2 > len(s.code) {
		s.pos = len(s.code)
		return
	}
	s.stack = append(s.stack, float64(int16(binary.BigEndian.Uint16(s.code[s.pos:]))))
	s.pos += 2
}

// clearWidth consumes an optional leading width operand.
func (s *type2State) clearWidth() {
	if !s.widthParsed && len(s.stack)%2 == 1 {
		s.stack = s.stack[1:]
	}
	s.widthParsed = true
}
