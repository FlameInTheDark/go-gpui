// Package keymap ports gpui's keystroke and keymap machinery: Keystroke
// parsing, key contexts, bindings and multi-stroke matching.
package keymap

import (
	"fmt"
	"strings"
)

// Modifiers is the state of the five modifier keys.
type Modifiers struct {
	Control  bool
	Alt      bool
	Shift    bool
	Platform bool // cmd on macOS, super/win elsewhere
	Function bool
}

// NoModifiers is the empty modifier set.
func NoModifiers() Modifiers { return Modifiers{} }

// Command is the platform (cmd/super/win) modifier.
func Command() Modifiers { return Modifiers{Platform: true} }

// Secondary is cmd on macOS, ctrl elsewhere.
func Secondary(isMac bool) Modifiers {
	if isMac {
		return Modifiers{Platform: true}
	}
	return Modifiers{Control: true}
}

// Modified reports whether any modifier is set.
func (m Modifiers) Modified() bool { return m.NumberOfModifiers() > 0 }

// NumberOfModifiers counts set modifiers.
func (m Modifiers) NumberOfModifiers() int {
	n := 0
	if m.Control {
		n++
	}
	if m.Alt {
		n++
	}
	if m.Shift {
		n++
	}
	if m.Platform {
		n++
	}
	if m.Function {
		n++
	}
	return n
}

// IsSubsetOf reports whether m's set bits are a subset of o's.
func (m Modifiers) IsSubsetOf(o Modifiers) bool {
	return (!m.Control || o.Control) &&
		(!m.Alt || o.Alt) &&
		(!m.Shift || o.Shift) &&
		(!m.Platform || o.Platform) &&
		(!m.Function || o.Function)
}

// Equal compares two modifier sets.
func (m Modifiers) Equal(o Modifiers) bool {
	return m == o
}

// String renders like "ctrl-shift".
func (m Modifiers) String() string {
	var parts []string
	if m.Control {
		parts = append(parts, "control")
	}
	if m.Alt {
		parts = append(parts, "alt")
	}
	if m.Shift {
		parts = append(parts, "shift")
	}
	if m.Platform {
		parts = append(parts, "platform")
	}
	if m.Function {
		parts = append(parts, "function")
	}
	return strings.Join(parts, "-")
}

// OS selects platform-specific keystroke behavior.
type OS int

const (
	// OSMac is macOS.
	OSMac OS = iota
	// OSLinux is Linux/BSD.
	OSLinux
	// OSWindows is Windows.
	OSWindows
)

// Capslock is the caps-lock state.
type Capslock struct{ On bool }

// Keystroke is a key event as seen by the keymap: the key name plus the
// modifiers held and, if any, the character the key would produce.
type Keystroke struct {
	Modifiers Modifiers
	// Key is the (lowercase) key name: a single character, or a named key
	// like "space", "backspace", "left".
	Key string
	// KeyChar is the character this keystroke would type, if any.
	KeyChar string
}

// IsModifierKey reports whether the key itself is a modifier.
func (k Keystroke) IsModifierKey() bool {
	switch k.Key {
	case "shift", "control", "alt", "platform", "function":
		return true
	}
	return false
}

// IsIMEInProgress reports whether the keystroke is being routed through an
// IME (printable key, no character, no relevant modifiers).
func (k Keystroke) IsIMEInProgress() bool {
	return k.KeyChar == "" && (IsPrintableKey(k.Key) || k.Key == "") &&
		!(k.Modifiers.Platform || k.Modifiers.Control || k.Modifiers.Function || k.Modifiers.Alt)
}

// WithSimulatedIME fills KeyChar for printable keys (test dispatch).
func (k Keystroke) WithSimulatedIME() Keystroke {
	if k.KeyChar != "" {
		return k
	}
	if k.Modifiers.Platform || k.Modifiers.Control || k.Modifiers.Function || k.Modifiers.Alt {
		return k
	}
	switch k.Key {
	case "space":
		k.KeyChar = " "
	case "tab":
		k.KeyChar = "\t"
	case "enter":
		k.KeyChar = "\n"
	default:
		if len(k.Key) == 1 {
			c := k.Key
			if k.Modifiers.Shift {
				c = strings.ToUpper(c)
			}
			k.KeyChar = c
		}
	}
	return k
}

// FillSpaceChar guarantees the space bar types a space. The space bar's
// key name is "space", so backends that resolve KeyChar only for
// single-character key names (win32's `len(key) == 1` guard, X11, Wayland)
// delivered key-downs with an empty KeyChar — and every text surface that
// inserts `if KeyChar != ""` silently dropped spaces. Platforms that do
// resolve the character keep it (early return).
func (k Keystroke) FillSpaceChar() Keystroke {
	if k.KeyChar != "" || k.Key != "space" {
		return k
	}
	if k.Modifiers.Platform || k.Modifiers.Control || k.Modifiers.Alt {
		return k
	}
	k.KeyChar = " "
	return k
}

// shouldMatch reports whether a typed keystroke matches a binding keystroke,
// with the IME-character relaxation from upstream (non-Windows): a typed
// keystroke with a key_char matches bindings on the character with only
// control/platform held.
func (k Keystroke) shouldMatch(target Keystroke, os OS) bool {
	if os != OSWindows && k.KeyChar != "" && k.KeyChar != k.Key {
		imeMods := Modifiers{Control: k.Modifiers.Control, Platform: k.Modifiers.Platform}
		if target.Key == k.KeyChar && target.Modifiers.Equal(imeMods) {
			return true
		}
	}
	if os == OSWindows && k.KeyChar != "" {
		if target.Key == k.KeyChar && target.Modifiers.Equal(NoModifiers()) {
			return true
		}
	}
	return target.Modifiers.Equal(k.Modifiers) && target.Key == k.Key
}

// unparse serializes back to the canonical binding string form.
func (k Keystroke) unparse(os OS) string {
	var parts []string
	m := k.Modifiers
	if m.Control {
		parts = append(parts, "ctrl")
	}
	if m.Alt {
		parts = append(parts, "alt")
	}
	if m.Shift {
		parts = append(parts, "shift")
	}
	if m.Function {
		parts = append(parts, "fn")
	}
	if m.Platform {
		name := "cmd"
		if os == OSLinux {
			name = "super"
		} else if os == OSWindows {
			name = "win"
		}
		parts = append(parts, name)
	}
	parts = append(parts, k.Key)
	return strings.Join(parts, "-")
}

// Display renders a keystroke for humans.
func (k Keystroke) Display(os OS) string {
	if os == OSMac {
		var b strings.Builder
		if k.Modifiers.Control {
			b.WriteString("^")
		}
		if k.Modifiers.Alt {
			b.WriteString("⌥")
		}
		if k.Modifiers.Shift {
			b.WriteString("⇧")
		}
		if k.Modifiers.Platform {
			b.WriteString("⌘")
		}
		if k.Modifiers.Function {
			b.WriteString("fn")
		}
		b.WriteString(displayKeyMac(k.Key))
		return b.String()
	}
	return k.unparse(os)
}

func displayKeyMac(key string) string {
	switch key {
	case "backspace":
		return "⌫"
	case "tab":
		return "⇥"
	case "escape":
		return "⎋"
	case "left":
		return "←"
	case "right":
		return "→"
	case "up":
		return "↑"
	case "down":
		return "↓"
	}
	if len(key) == 1 {
		return strings.ToUpper(key)
	}
	return key
}

// IsPrintableKey reports whether the key can produce a character.
func IsPrintableKey(key string) bool {
	switch key {
	case "f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11",
		"f12", "f13", "f14", "f15", "f16", "f17", "f18", "f19", "f20", "f21",
		"f22", "f23", "f24", "f25", "f26", "f27", "f28", "f29", "f30", "f31",
		"f32", "f33", "f34", "f35",
		"backspace", "delete", "left", "right", "up", "down",
		"pageup", "pagedown", "insert", "home", "end",
		"back", "forward", "escape":
		return false
	}
	return true
}

// ExpectedKeystrokeError is the canonical parse error message.
const ExpectedKeystrokeError = "Expected a sequence of modifiers (`ctrl`, `alt`, `shift`, `fn`, `cmd`, `super`, or `win`) followed by a key, separated by `-`."

// ParseKeystroke parses a binding keystroke string like "cmd-shift-p",
// "ctrl-`", "secondary-x", "space" or a lone "-" for a literal dash. This is
// a faithful port of Keystroke::parse_internal.
func ParseKeystroke(source string, os OS) (Keystroke, error) {
	if source == "" {
		return Keystroke{}, fmt.Errorf("invalid keystroke: %q. %s", source, ExpectedKeystrokeError)
	}
	var modifiers Modifiers
	var key string
	var keyChar string

	components := strings.Split(source, "-")
	for i := 0; i < len(components); i++ {
		component := strings.ToLower(components[i])
		switch component {
		case "ctrl":
			modifiers.Control = true
			continue
		case "alt":
			modifiers.Alt = true
			continue
		case "shift":
			modifiers.Shift = true
			continue
		case "fn":
			modifiers.Function = true
			continue
		case "secondary":
			if os == OSMac {
				modifiers.Platform = true
			} else {
				modifiers.Control = true
			}
			continue
		case "cmd", "super", "win":
			modifiers.Platform = true
			continue
		}

		// Not a modifier: the key.
		if i+1 < len(components) {
			next := components[i+1]
			if next == "" && strings.HasSuffix(source, "-") {
				// Trailing dash: a literal "-" key.
				key = "-"
				break
			}
			if len(next) > 1 && strings.HasPrefix(next, ">") {
				// "a->ß": key with an explicit key_char (test form).
				key = components[i]
				keyChar = next[1:]
				i++
				continue
			}
			return Keystroke{}, fmt.Errorf("invalid keystroke: %q. %s", source, ExpectedKeystrokeError)
		}

		k := components[i]
		if len(k) == 1 && k[0] >= 'A' && k[0] <= 'Z' {
			// Uppercase single char means shift + lowercase.
			modifiers.Shift = true
			key = strings.ToLower(k)
		} else {
			key = strings.ToLower(k)
		}
	}

	if key == "" {
		// Allow a bare modifier as the key, e.g. "cmd" alone.
		if modifiers.Platform {
			key = "platform"
			modifiers.Platform = false
		} else if modifiers.Control {
			key = "control"
			modifiers.Control = false
		} else if modifiers.Alt {
			key = "alt"
			modifiers.Alt = false
		} else if modifiers.Shift {
			key = "shift"
			modifiers.Shift = false
		} else if modifiers.Function {
			key = "function"
			modifiers.Function = false
		} else {
			return Keystroke{}, fmt.Errorf("invalid keystroke: %q. %s", source, ExpectedKeystrokeError)
		}
	}

	return Keystroke{Modifiers: modifiers, Key: key, KeyChar: keyChar}, nil
}
