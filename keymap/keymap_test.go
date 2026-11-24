package keymap

import (
	"testing"

	"github.com/FlameInTheDark/go-gpui/action"
)

type dummyAction struct {
	name string
}

func (d dummyAction) Name() string              { return d.name }
func (d dummyAction) BoxedClone() action.Action { return dummyAction{name: d.name} }
func (d dummyAction) PartialEq(o action.Action) bool {
	return o != nil && o.Name() == d.name
}

func act(name string) action.Action { return dummyAction{name: name} }

func TestParseKeystroke(t *testing.T) {
	k, err := ParseKeystroke("cmd-shift-p", OSLinux)
	if err != nil {
		t.Fatal(err)
	}
	if k.Key != "p" || !k.Modifiers.Shift || !k.Modifiers.Platform {
		t.Fatalf("cmd-shift-p: %+v", k)
	}
	// uppercase implies shift
	k, _ = ParseKeystroke("P", OSLinux)
	if k.Key != "p" || !k.Modifiers.Shift {
		t.Fatalf("P: %+v", k)
	}
	// secondary maps per OS
	k, _ = ParseKeystroke("secondary-x", OSMac)
	if !k.Modifiers.Platform {
		t.Fatal("secondary on mac is cmd")
	}
	k, _ = ParseKeystroke("secondary-x", OSLinux)
	if !k.Modifiers.Control {
		t.Fatal("secondary on linux is ctrl")
	}
	// literal dash
	k, _ = ParseKeystroke("ctrl--", OSLinux)
	if k.Key != "-" || !k.Modifiers.Control {
		t.Fatalf("ctrl--: %+v", k)
	}
	// bare modifier
	k, _ = ParseKeystroke("cmd", OSLinux)
	if k.Key != "platform" || k.Modifiers.Modified() {
		t.Fatalf("bare cmd: %+v", k)
	}
	// named keys
	k, _ = ParseKeystroke("backspace", OSLinux)
	if k.Key != "backspace" {
		t.Fatal("backspace")
	}
	// invalid
	if _, err := ParseKeystroke("ctrl-foo-bar", OSLinux); err == nil {
		t.Fatal("should reject multi-word key")
	}
	if _, err := ParseKeystroke("", OSLinux); err == nil {
		t.Fatal("should reject empty")
	}
}

func TestKeystrokeDisplayMac(t *testing.T) {
	k, _ := ParseKeystroke("cmd-shift-p", OSMac)
	if k.Display(OSMac) != "⇧⌘P" {
		t.Fatalf("mac display: %q", k.Display(OSMac))
	}
}

func TestContextParsing(t *testing.T) {
	c, err := NewContext("Editor mode = insert os=linux")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Contains(ContextEntry{Key: "Editor"}) {
		t.Fatal("identifier Editor")
	}
	if !c.Contains(ContextEntry{Key: "mode", Value: "insert", IsKV: true}) {
		t.Fatal("mode=insert")
	}
	// duplicates ignored
	c2, _ := NewContext("Editor")
	c.Extend(c2)
	count := 0
	for _, e := range c.Entries {
		if !e.IsKV && e.Key == "Editor" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("duplicate identifier: %d", count)
	}
}

func TestPredicateParsingAndEval(t *testing.T) {
	editor := mustCtx(t, "Editor mode=insert")
	other := mustCtx(t, "Other")
	stack := []KeyContext{other, editor}

	p, err := ParsePredicate("Editor && mode == insert")
	if err != nil {
		t.Fatal(err)
	}
	if !PredicateEnabled(p, stack) {
		t.Fatal("Editor && mode == insert should hold")
	}
	p, _ = ParsePredicate("mode == normal")
	if PredicateEnabled(p, stack) {
		t.Fatal("mode is insert")
	}
	p, _ = ParsePredicate("mode != normal")
	if !PredicateEnabled(p, stack) {
		t.Fatal("mode != normal holds")
	}
	p, _ = ParsePredicate("!Editor")
	if PredicateEnabled(p, stack) {
		t.Fatal("Editor present")
	}
	// descendant: parent on some prefix, child on the suffix
	p, _ = ParsePredicate("Other > Editor")
	if !PredicateEnabled(p, stack) {
		t.Fatal("Other > Editor should hold")
	}
	p, _ = ParsePredicate("Editor > Other")
	if PredicateEnabled(p, stack) {
		t.Fatal("Editor > Other should not hold")
	}
	// depth: deeper suffix match wins
	d := DepthOf(mustPred(t, "Editor"), stack)
	if d != 2 {
		t.Fatalf("depth: %d", d)
	}
	d = DepthOf(mustPred(t, "Other"), stack)
	if d != 1 {
		t.Fatalf("depth Other: %d", d)
	}
}

func mustCtx(t *testing.T, s string) KeyContext {
	t.Helper()
	c, err := NewContext(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mustPred(t *testing.T, s string) Predicate {
	t.Helper()
	p, err := ParsePredicate(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBindingMatching(t *testing.T) {
	km := New(OSLinux)
	b1, _ := NewBinding("ctrl-p", act("palette::Open"), "", OSLinux)
	b2, _ := NewBinding("ctrl-k ctrl-t", act("theme::Toggle"), "", OSLinux)
	km.AddBindings(b1, b2)

	ctx := []KeyContext{mustCtx(t, "Editor")}
	ctrlP, _ := ParseKeystroke("ctrl-p", OSLinux)
	matched, pending := km.BindingsForInput([]Keystroke{ctrlP}, ctx)
	if pending || len(matched) != 1 || matched[0].Action.Name() != "palette::Open" {
		t.Fatalf("ctrl-p: %+v pending=%v", matched, pending)
	}

	// multi-stroke prefix is pending
	ctrlK, _ := ParseKeystroke("ctrl-k", OSLinux)
	matched, pending = km.BindingsForInput([]Keystroke{ctrlK}, ctx)
	if !pending || len(matched) != 0 {
		t.Fatalf("ctrl-k pending: matched=%v pending=%v", matched, pending)
	}

	// full multi-stroke
	ctrlT, _ := ParseKeystroke("ctrl-t", OSLinux)
	matched, pending = km.BindingsForInput([]Keystroke{ctrlK, ctrlT}, ctx)
	if pending || len(matched) != 1 || matched[0].Action.Name() != "theme::Toggle" {
		t.Fatalf("ctrl-k ctrl-t: %+v", matched)
	}
}

func TestBindingPrecedence(t *testing.T) {
	km := New(OSLinux)
	// default binding first (weaker)
	d, _ := NewBinding("ctrl-p", act("a::Default"), "", OSLinux)
	d.Meta = 2
	// user binding later (stronger)
	u, _ := NewBinding("ctrl-p", act("a::User"), "", OSLinux)
	u.Meta = 0
	km.AddBindings(d, u)

	ctx := []KeyContext{mustCtx(t, "Editor")}
	ctrlP, _ := ParseKeystroke("ctrl-p", OSLinux)
	matched, _ := km.BindingsForInput([]Keystroke{ctrlP}, ctx)
	// Like upstream, both matches are returned in precedence order (the
	// user binding first); the dispatch loop stops at the first unless
	// propagation continues.
	if len(matched) != 2 || matched[0].Action.Name() != "a::User" || matched[1].Action.Name() != "a::Default" {
		t.Fatalf("later binding wins: %+v", matched)
	}

	// deeper context beats later insertion
	km2 := New(OSLinux)
	plain, _ := NewBinding("ctrl-x", act("a::Plain"), "", OSLinux)
	nested, _ := NewBinding("ctrl-x", act("a::Nested"), "Editor", OSLinux)
	km2.AddBindings(plain, nested)
	ctrlX, _ := ParseKeystroke("ctrl-x", OSLinux)
	matched, _ = km2.BindingsForInput([]Keystroke{ctrlX}, ctx)
	// Deeper context sorts first; both are returned in precedence order.
	if len(matched) != 2 || matched[0].Action.Name() != "a::Nested" || matched[1].Action.Name() != "a::Plain" {
		t.Fatalf("context depth wins: %+v", matched)
	}

	// outside Editor context the plain binding fires
	matched, _ = km2.BindingsForInput([]Keystroke{ctrlX}, nil)
	if len(matched) != 1 || matched[0].Action.Name() != "a::Plain" {
		t.Fatalf("no context: %+v", matched)
	}
}

func TestNoActionAndUnbind(t *testing.T) {
	km := New(OSLinux)
	def, _ := NewBinding("ctrl-b", act("a::Bold"), "", OSLinux)
	def.Meta = 2
	noact, _ := NewBinding("ctrl-b", action.NoAction{}, "", OSLinux)
	noact.Meta = 2 // same tier: suppresses
	km.AddBindings(def, noact)

	ctrlB, _ := ParseKeystroke("ctrl-b", OSLinux)
	matched, _ := km.BindingsForInput([]Keystroke{ctrlB}, nil)
	if len(matched) != 0 {
		t.Fatalf("NoAction should suppress same-tier binding: %+v", matched)
	}

	km2 := New(OSLinux)
	b, _ := NewBinding("ctrl-i", act("a::Italic"), "", OSLinux)
	b.Meta = 2
	unb, _ := NewBinding("ctrl-i", action.Unbind{Target: "a::Italic"}, "", OSLinux)
	km2.AddBindings(b, unb)
	ctrlI, _ := ParseKeystroke("ctrl-i", OSLinux)
	matched, _ = km2.BindingsForInput([]Keystroke{ctrlI}, nil)
	if len(matched) != 0 {
		t.Fatalf("Unbind should remove: %+v", matched)
	}
	if got := km2.BindingsForAction(act("a::Italic")); len(got) != 0 {
		t.Fatalf("BindingsForAction should be empty: %+v", got)
	}
}

func TestModifiers(t *testing.T) {
	m := Modifiers{Control: true, Shift: true}
	if !m.Modified() || m.NumberOfModifiers() != 2 {
		t.Fatal("modified count")
	}
	ctrl := Modifiers{Control: true}
	if !ctrl.IsSubsetOf(m) {
		t.Fatal("subset")
	}
	alt := Modifiers{Alt: true}
	if alt.IsSubsetOf(m) {
		t.Fatal("not subset")
	}
	secMac := Secondary(true)
	plat := Modifiers{Platform: true}
	if secMac != plat {
		t.Fatal("secondary mac")
	}
}
