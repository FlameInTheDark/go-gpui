package keymap

import (
	"fmt"
	"strings"
)

// ContextEntry is one identifier or key=value pair in a KeyContext.
type ContextEntry struct {
	Key   string
	Value string // empty means identifier-only
	IsKV  bool
}

// KeyContext is an ordered bag of identifiers and key=value pairs, e.g.
// "Editor mode=insert os=linux".
type KeyContext struct {
	Entries []ContextEntry
}

// NewContext builds a context from "identifier" / "key = value" tokens.
func NewContext(source string) (KeyContext, error) {
	var ctx KeyContext
	tokens := strings.Fields(source)
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		// support "key=value" attached and "key = value" split across tokens
		if i+2 < len(tokens)+1 && i+1 < len(tokens) && tokens[i+1] == "=" {
			if i+2 >= len(tokens) {
				return KeyContext{}, fmt.Errorf("invalid key context: %q", source)
			}
			if !ctx.containsKV(tok, tokens[i+2]) {
				ctx.Entries = append(ctx.Entries, ContextEntry{Key: tok, Value: tokens[i+2], IsKV: true})
			}
			i += 2
			continue
		}
		if eq := strings.Index(tok, "="); eq > 0 {
			k, v := tok[:eq], tok[eq+1:]
			if !ctx.containsKV(k, v) {
				ctx.Entries = append(ctx.Entries, ContextEntry{Key: k, Value: v, IsKV: true})
			}
			continue
		}
		if tok == "=" || tok == "" {
			continue
		}
		if !ctx.containsIdentifier(tok) {
			ctx.Entries = append(ctx.Entries, ContextEntry{Key: tok})
		}
	}
	return ctx, nil
}

// ContextWithDefaults seeds os and the given parts.
func ContextWithDefaults(osName string, parts ...string) KeyContext {
	ctx, _ := NewContext("os=" + osName)
	for _, p := range parts {
		if c, err := NewContext(p); err == nil {
			ctx.Extend(c)
		}
	}
	return ctx
}

func (c KeyContext) containsIdentifier(name string) bool {
	for _, e := range c.Entries {
		if !e.IsKV && e.Key == name {
			return true
		}
	}
	return false
}

func (c KeyContext) containsKV(k, v string) bool {
	for _, e := range c.Entries {
		if e.IsKV && e.Key == k && e.Value == v {
			return true
		}
	}
	return false
}

// Contains reports whether the context has the identifier or exact pair.
func (c KeyContext) Contains(entry ContextEntry) bool {
	if entry.IsKV {
		return c.containsKV(entry.Key, entry.Value)
	}
	return c.containsIdentifier(entry.Key)
}

// Extend appends entries from another context, ignoring duplicates.
func (c *KeyContext) Extend(o KeyContext) {
	for _, e := range o.Entries {
		if !c.Contains(e) {
			c.Entries = append(c.Entries, e)
		}
	}
}

// String renders the context.
func (c KeyContext) String() string {
	var parts []string
	for _, e := range c.Entries {
		if e.IsKV {
			parts = append(parts, e.Key+"="+e.Value)
		} else {
			parts = append(parts, e.Key)
		}
	}
	return strings.Join(parts, " ")
}

// Predicate is a key-context predicate tree.
type Predicate interface {
	// evalInner evaluates against a context stack; only the last (deepest)
	// context is examined for leaf predicates.
	evalInner(contexts []KeyContext) bool
	fmt.Stringer
}

type predIdentifier struct{ name string }

func (p predIdentifier) evalInner(contexts []KeyContext) bool {
	if len(contexts) == 0 {
		return false
	}
	last := contexts[len(contexts)-1]
	return last.containsIdentifier(p.name)
}
func (p predIdentifier) String() string { return p.name }

type predEqual struct{ k, v string }

func (p predEqual) evalInner(contexts []KeyContext) bool {
	if len(contexts) == 0 {
		return false
	}
	last := contexts[len(contexts)-1]
	return last.containsKV(p.k, p.v)
}
func (p predEqual) String() string { return p.k + " == " + p.v }

type predNotEqual struct{ k, v string }

func (p predNotEqual) evalInner(contexts []KeyContext) bool {
	if len(contexts) == 0 {
		return false
	}
	last := contexts[len(contexts)-1]
	// NotEqual matches absent keys too.
	return !last.containsKV(p.k, p.v)
}
func (p predNotEqual) String() string { return p.k + " != " + p.v }

type predNot struct{ inner Predicate }

func (p predNot) evalInner(contexts []KeyContext) bool {
	// Not(p) is true iff p fails for EVERY prefix of the context stack.
	for i := range contexts {
		if p.inner.evalInner(contexts[:i+1]) {
			return false
		}
	}
	return true
}
func (p predNot) String() string {
	if _, ok := p.inner.(predIdentifier); ok {
		return "!" + p.inner.String()
	}
	return "!(" + p.inner.String() + ")"
}

type predAnd struct{ l, r Predicate }

func (p predAnd) evalInner(contexts []KeyContext) bool {
	return p.l.evalInner(contexts) && p.r.evalInner(contexts)
}
func (p predAnd) String() string { return p.l.String() + " && " + p.r.String() }

type predOr struct{ l, r Predicate }

func (p predOr) evalInner(contexts []KeyContext) bool {
	return p.l.evalInner(contexts) || p.r.evalInner(contexts)
}
func (p predOr) String() string { return p.l.String() + " || " + p.r.String() }

type predDescendant struct{ parent, child Predicate }

func (p predDescendant) evalInner(contexts []KeyContext) bool {
	for i := 0; i < len(contexts); i++ {
		if p.parent.evalInner(contexts[:i+1]) && p.child.evalInner(contexts[i+1:]) {
			return true
		}
	}
	return false
}
func (p predDescendant) String() string { return p.parent.String() + " > " + p.child.String() }

// predicate precedences (low to high)
const (
	precChild = 1
	precOr    = 2
	precAnd   = 3
	precEq    = 4
	precNot   = 5
)

type predParser struct {
	toks []string
	pos  int
}

func isIdentifierChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '_' || c == '-'
}

func tokenizePredicate(s string) []string {
	var toks []string
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n':
			i++
		case c == '(' || c == ')':
			toks = append(toks, string(c))
			i++
		case c == '>' && i+1 < len(s) && s[i+1] == '=':
			toks = append(toks, ">=")
			i += 2
		case c == '<' && i+1 < len(s) && s[i+1] == '=':
			toks = append(toks, "<=")
			i += 2
		case c == '!' && i+1 < len(s) && s[i+1] == '=':
			toks = append(toks, "!=")
			i += 2
		case c == '=' && i+1 < len(s) && s[i+1] == '=':
			toks = append(toks, "==")
			i += 2
		case c == '&' && i+1 < len(s) && s[i+1] == '&':
			toks = append(toks, "&&")
			i += 2
		case c == '|' && i+1 < len(s) && s[i+1] == '|':
			toks = append(toks, "||")
			i += 2
		case c == '>' || c == '<':
			toks = append(toks, string(c))
			i++
		case c == '!':
			toks = append(toks, "!")
			i++
		case isIdentifierChar(c):
			j := i
			for j < len(s) && isIdentifierChar(s[j]) {
				j++
			}
			toks = append(toks, s[i:j])
			i = j
		default:
			// single-char vim operator-ish identifier
			toks = append(toks, string(c))
			i++
		}
	}
	return toks
}

// ParsePredicate parses a key binding context predicate like
// "Editor && mode == insert", "!(MenuBar > Open)", "A > B".
func ParsePredicate(source string) (Predicate, error) {
	p := &predParser{toks: tokenizePredicate(source)}
	pred, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.toks) {
		return nil, fmt.Errorf("unexpected token %q in predicate %q", p.toks[p.pos], source)
	}
	return pred, nil
}

func (p *predParser) peek() (string, bool) {
	if p.pos < len(p.toks) {
		return p.toks[p.pos], true
	}
	return "", false
}

func (p *predParser) parseExpr(minPrecedence int) (Predicate, error) {
	left, err := p.parseAtom()
	if err != nil {
		return nil, err
	}
	for {
		tok, ok := p.peek()
		if !ok {
			return left, nil
		}
		var prec int
		var op string
		switch tok {
		case ">":
			prec, op = precChild, ">"
		case "||":
			prec, op = precOr, "||"
		case "&&":
			prec, op = precAnd, "&&"
		case "==":
			prec, op = precEq, "=="
		case "!=":
			prec, op = precEq, "!="
		default:
			return left, nil
		}
		if prec < minPrecedence {
			return left, nil
		}
		p.pos++
		right, err := p.parseExpr(prec + 1)
		if err != nil {
			return nil, err
		}
		switch op {
		case ">":
			left = predDescendant{parent: left, child: right}
		case "||":
			left = predOr{l: left, r: right}
		case "&&":
			left = predAnd{l: left, r: right}
		case "==":
			li, lok := left.(predIdentifier)
			ri, rok := right.(predIdentifier)
			if !lok || !rok {
				return nil, fmt.Errorf("== operands must be identifiers")
			}
			left = predEqual{k: li.name, v: ri.name}
		case "!=":
			li, lok := left.(predIdentifier)
			ri, rok := right.(predIdentifier)
			if !lok || !rok {
				return nil, fmt.Errorf("!= operands must be identifiers")
			}
			left = predNotEqual{k: li.name, v: ri.name}
		}
	}
}

func (p *predParser) parseAtom() (Predicate, error) {
	tok, ok := p.peek()
	if !ok {
		return nil, fmt.Errorf("unexpected end of predicate")
	}
	switch tok {
	case "!":
		p.pos++
		inner, err := p.parseExpr(precNot)
		if err != nil {
			return nil, err
		}
		return predNot{inner: inner}, nil
	case "(":
		p.pos++
		inner, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if t, ok := p.peek(); !ok || t != ")" {
			return nil, fmt.Errorf("expected )")
		}
		p.pos++
		return inner, nil
	default:
		p.pos++
		return predIdentifier{name: tok}, nil
	}
}

// DepthOf returns the deepest suffix length of contexts for which the
// predicate holds; 0 if it never holds. Used for binding precedence.
func DepthOf(p Predicate, contexts []KeyContext) int {
	for depth := len(contexts); depth >= 0; depth-- {
		if p.evalInner(contexts[:depth]) {
			return depth
		}
	}
	return 0
}

// PredicateEnabled reports whether a binding's predicate (if any) matches
// the context stack.
func PredicateEnabled(p Predicate, contexts []KeyContext) bool {
	if p == nil {
		return true
	}
	return p.evalInner(contexts)
}

// IsSuperset reports whether p covers everything o covers (used to decide
// whether a NoAction/Unbind entry dominates a binding).
func IsSuperset(p, o Predicate) bool {
	if p == nil {
		return true
	}
	if o == nil {
		return false
	}
	if p.String() == o.String() {
		return true
	}
	switch pp := p.(type) {
	case predOr:
		return IsSuperset(pp.l, o) || IsSuperset(pp.r, o)
	case predAnd:
		return IsSuperset(pp.l, o) || IsSuperset(pp.r, o)
	case predDescendant:
		return IsSuperset(pp.child, o)
	}
	return false
}
