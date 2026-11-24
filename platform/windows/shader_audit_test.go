// This file intentionally has NO build constraint: gpuShaderSource (in
// shader_source.go) is pure data, so this static audit runs on every
// platform, including the Linux CI where //go:build windows files are
// excluded, like TestVTableSlots and TestGammaCorrectionRatios.
package windows

import (
	"regexp"
	"strings"
	"testing"
)

// TestShaderCallArities statically audits gpuShaderSource. D3DCompile only
// runs on Windows at runtime, so a broken call (a missing argument after a
// refactor, a typo'd helper name) is otherwise noticed only when the renderer
// logs "Direct3D 11 unavailable ... using software rendering" on the user's
// machine. This test parses the HLSL the same way a compiler front-end would:
// every call to a user-defined function must pass the declared number of
// arguments, and every called identifier must resolve to a definition or a
// known HLSL intrinsic / type constructor.
//
// It is intentionally conservative: unknown constructs fail the test rather
// than being silently accepted, so extending the shader requires keeping the
// intrinsic table below honest.
func TestShaderCallArities(t *testing.T) {
	code := stripHLSLComments(gpuShaderSource)

	// Collect user function definitions: "type name(params) {", possibly
	// multi-line and possibly preceded by "static"; pixel-shader entry
	// points carry a semantic between ")" and "{"
	// (float4 psMain(PSIn input) : SV_Target {).
	type fnDef struct {
		args      int
		line      int
		headerMin int // byte offset where the definition header starts
		headerMax int // byte offset where the definition header ends
	}
	defs := map[string]fnDef{}
	defRe := regexp.MustCompile(`(?m)^(?:static\s+)?(?:\w+\s+)+?(\w+)\s*\(([^)]*)\)\s*(?::\s*\w+)?\s*\{`)
	for _, m := range defRe.FindAllStringSubmatchIndex(code, -1) {
		name := code[m[2]:m[3]]
		switch name {
		case "if", "for", "while", "switch", "return":
			continue
		}
		args := strings.TrimSpace(code[m[4]:m[5]])
		n := 0
		if args != "" && args != "void" {
			n = len(strings.Split(args, ","))
		}
		defs[name] = fnDef{n, strings.Count(code[:m[0]], "\n") + 1, m[0], m[1]}
	}
	if len(defs) < 30 {
		t.Fatalf("shader audit parsed only %d function definitions from gpuShaderSource; the source is probably malformed", len(defs))
	}

	known := map[string]bool{
		// " : register(t0)" resource bindings look like calls.
		"register": true,
	}
	for _, intrinsic := range strings.Fields(`abs acos all any asin atan atan2 ceil clamp
                cos cosh cross degrees determinant distance dot exp exp2 faceforward floor fmod frac
                fwidth isfinite isinf isnan lerp lit log log10 log2 mad max min mul normalize pow
                radians reflect refract round rsqrt saturate sign sin sincos sinh smoothstep sqrt
                step tan tanh transpose trunc length frexp ldexp modf asfloat asint asuint
                float float2 float3 float4 int int2 int3 int4 uint uint2 uint3 uint4 bool bool2
                bool3 bool4 half half2 half3 half4 double
                if for while switch return discard break continue struct const static void
                StructuredBuffer Texture2D Texture2DArray Texture2DMS Texture3D TextureCube
                SamplerState cbuffer Buffer ByteAddressBuffer RWStructuredBuffer RWByteAddressBuffer`) {
		known[intrinsic] = true
	}

	// scanCalls walks every identifier immediately followed by "(" with a
	// balanced argument list, and recurses INTO the argument list: a plain
	// find-all would let an outer match like "if (a && f(x))" consume the
	// nested call f(x), hiding it from the audit (that blind spot is why
	// this test once passed while the shader failed to compile). After a
	// call is handled, scanning resumes after its ")" so nothing is
	// audited twice.
	var scanCalls func(segment string, base int)
	callRe := regexp.MustCompile(`(\w+)\s*\(`)
	scanCalls = func(segment string, base int) {
		pos := 0
		for pos < len(segment) {
			m := callRe.FindStringSubmatchIndex(segment[pos:])
			if m == nil {
				return
			}
			nameStart := base + pos + m[0]
			name := segment[pos+m[2] : pos+m[3]]
			// Find the balanced closing parenthesis in the full code.
			open := base + pos + m[1] - 1 // index of "("
			depth := 0
			close := -1
			for i := open; i < len(code); i++ {
				switch code[i] {
				case '(':
					depth++
				case ')':
					depth--
					if depth == 0 {
						close = i
					}
				}
				if close >= 0 {
					break
				}
			}
			if close < 0 {
				t.Errorf("shader line %d: unbalanced parentheses after %s(", strings.Count(code[:nameStart], "\n")+1, name)
				return
			}
			inner := code[open+1 : close]
			// Always audit the arguments, whatever wraps them.
			scanCalls(inner, open+1)

			if !known[name] {
				// Skip member/method-like positions (foo.bar(...)); definitions
				// preceded by a type are recognized via the header range below.
				memberOrDecl := false
				if nameStart > 0 {
					if prev := code[nameStart-1]; prev == '.' || isWordByte(prev) {
						memberOrDecl = true
					}
				}
				if !memberOrDecl {
					line := strings.Count(code[:nameStart], "\n") + 1
					got := 0
					if strings.TrimSpace(inner) != "" {
						got = countHLSLTopLevelCommas(inner) + 1
					}
					def, ok := defs[name]
					if !ok {
						t.Errorf("shader line %d: call to %q which is neither defined in gpuShaderSource nor a known HLSL intrinsic", line, name)
					} else {
						if got != def.args {
							t.Errorf("shader line %d: %s(...) passes %d argument(s) but its definition at shader line %d takes %d", line, name, got, def.line, def.args)
						}
						// The definition header itself re-matches here; anything else must
						// not call the function before its declaration (DXC rejects
						// use-before-declaration, and FXC's behavior there is untested).
						if nameStart < def.headerMin || nameStart >= def.headerMax {
							if line < def.line {
								t.Errorf("shader line %d: %s(...) is called before its definition at shader line %d; declare helpers above its first use", line, name, def.line)
							}
						}
					}
				}
			}
			pos = close + 1 - base
		}
	}
	scanCalls(code, 0)
}

func stripHLSLComments(src string) string {
	var b strings.Builder
	for i := 0; i < len(src); {
		if strings.HasPrefix(src[i:], "//") {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		if strings.HasPrefix(src[i:], "/*") {
			end := strings.Index(src[i:], "*/")
			if end < 0 {
				i = len(src)
			} else {
				// Keep newlines so line numbers stay stable.
				b.WriteString(strings.Repeat("\n", strings.Count(src[i:i+end+2], "\n")))
				i += end + 2
			}
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

func countHLSLTopLevelCommas(s string) int {
	depth := 0
	commas := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ',':
			if depth == 0 {
				commas++
			}
		}
	}
	return commas
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
