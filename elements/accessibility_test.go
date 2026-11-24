package elements

import (
	"testing"

	gc "github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/gpui"
)

// TestAccessibilityTreeRecordsButtonsAndText verifies the a11y tree:
// clickable divs record button nodes (with tooltip labels as fallback),
// explicit A11y() roles are honored, and text elements record static
// text nodes with their value.
func TestAccessibilityTreeRecordsButtonsAndText(t *testing.T) {
	w := openAnchoredWindow(t, 300, 200, func(w *gpui.Window) gpui.Element {
		return NewDiv().Flex().FlexCol().
			Child(NewDiv().
				ID(gpui.NameElementId("btn")).
				A11y(gpui.AccessibilityButton, "Save file").
				OnClick(func(ev gpui.ClickEvent, w *gpui.Window) {}).
				W(pxLen(100)).H(pxLen(40)).
				BgColor(gc.Red())).
			Child(NewText("Hello accessibility")).
			Child(NewDiv().
				ID(gpui.NameElementId("check")).
				A11y(gpui.AccessibilityCheckBox, "Dark mode").
				A11yValue("on").
				W(pxLen(100)).H(pxLen(24)))
	})
	w.Draw()

	tree := w.RecordedAccessibility()
	if tree == nil {
		t.Fatal("no accessibility tree")
	}
	// The explicit button node.
	if node, _, ok := tree.FindByLabel("Save file"); !ok || node.Role != gpui.AccessibilityButton {
		t.Fatalf("button node not found: %+v ok=%v", node, ok)
	}
	// The static text node carries its value.
	if node, _, ok := tree.FindByLabel("Hello accessibility"); !ok || node.Role != gpui.AccessibilityStaticText {
		t.Fatalf("static text node not found: %+v ok=%v", node, ok)
	}
	// The checkbox carries its value.
	node, idx, ok := tree.FindByLabel("Dark mode")
	if !ok || node.Role != gpui.AccessibilityCheckBox {
		t.Fatalf("checkbox node not found: %+v ok=%v", node, ok)
	}
	if node.Value != "on" {
		t.Fatalf("checkbox value = %q, want on", node.Value)
	}
	// Nodes have real bounds.
	if node.Bounds.Size.Width.AsF32() <= 0 || node.Bounds.Size.Height.AsF32() <= 0 {
		t.Fatalf("node %d bounds not recorded: %+v", idx, node.Bounds)
	}
}

// TestAccessibilityInferredButtonRole verifies clickable divs without an
// explicit role default to button.
func TestAccessibilityInferredButtonRole(t *testing.T) {
	w := openAnchoredWindow(t, 300, 200, func(w *gpui.Window) gpui.Element {
		return NewDiv().
			ID(gpui.NameElementId("clickme")).
			Tooltip("Click me!").
			OnClick(func(ev gpui.ClickEvent, w *gpui.Window) {}).
			W(pxLen(100)).H(pxLen(40))
	})
	w.Draw()
	tree := w.RecordedAccessibility()
	nodes := tree.FindByRole(gpui.AccessibilityButton)
	if len(nodes) == 0 {
		t.Fatal("clickable div did not record a button node")
	}
	if tree.Nodes[nodes[0]].Label != "Click me!" {
		t.Fatalf("inferred label = %q, want tooltip text", tree.Nodes[nodes[0]].Label)
	}
}

// TestAccessibilityNesting verifies nodes nest: children recorded inside
// a parent's closure become its children.
func TestAccessibilityNesting(t *testing.T) {
	w := openAnchoredWindow(t, 300, 200, func(w *gpui.Window) gpui.Element {
		return NewDiv().
			A11y(gpui.AccessibilityContainer, "toolbar").
			Child(NewDiv().
				A11y(gpui.AccessibilityButton, "cut").
				OnClick(func(ev gpui.ClickEvent, w *gpui.Window) {}))
	})
	w.Draw()
	tree := w.RecordedAccessibility()
	node, _, ok := tree.FindByLabel("toolbar")
	if !ok {
		t.Fatal("toolbar container not recorded")
	}
	found := false
	for _, c := range node.Children {
		if tree.Nodes[c].Label == "cut" {
			found = true
		}
	}
	if !found {
		t.Fatalf("cut button not nested under toolbar: %+v", node)
	}
}
