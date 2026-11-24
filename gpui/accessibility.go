package gpui

import (
	"github.com/FlameInTheDark/go-gpui/geometry"
)

// Accessibility tree (upstream window/a11y.rs + accesskit). Upstream
// builds an accesskit tree and feeds it to the OS screen reader; this
// port records the same structure (role, label, bounds, states) during
// prepaint so tests and platform bridges can read the semantic tree.
//
// Elements opt in through their style or explicit annotation:
//
//	w.WithAccessibility(AccessibilityNode{
//	    Role:    AccessibilityButton,
//	    Label:   "Save file",
//	}, func() { ... children ... })
//
// The node's bounds come from the enclosing element's layout bounds, and
// children nest inside the closure.

// AccessibilityRole classifies an element for assistive technology
// (upstream accesskit::Role; the subset this port records).
type AccessibilityRole uint8

const (
	// AccessibilityUnknown is the default (not recorded).
	AccessibilityUnknown AccessibilityRole = 0
	// AccessibilityWindow is the root window.
	AccessibilityWindow AccessibilityRole = 1
	// AccessibilityContainer is a generic grouping.
	AccessibilityContainer AccessibilityRole = 2
	// AccessibilityButton is an actionable control.
	AccessibilityButton AccessibilityRole = 3
	// AccessibilityCheckBox is a toggle.
	AccessibilityCheckBox AccessibilityRole = 4
	// AccessibilityRadioButton is a one-of selector.
	AccessibilityRadioButton AccessibilityRole = 5
	// AccessibilityTextInput is an editable text field.
	AccessibilityTextInput AccessibilityRole = 6
	// AccessibilityStaticText is non-editable text (labels).
	AccessibilityStaticText AccessibilityRole = 7
	// AccessibilityImage is a picture.
	AccessibilityImage AccessibilityRole = 8
	// AccessibilityList is a list container.
	AccessibilityList AccessibilityRole = 9
	// AccessibilityListItem is one row of a list.
	AccessibilityListItem AccessibilityRole = 10
	// AccessibilityTab is a tab selector.
	AccessibilityTab AccessibilityRole = 11
	// AccessibilityMenu is a menu container.
	AccessibilityMenu AccessibilityRole = 12
	// AccessibilityMenuItem is one menu entry.
	AccessibilityMenuItem AccessibilityRole = 13
)

// String names the role (stable identifiers for platform bridges).
func (r AccessibilityRole) String() string {
	switch r {
	case AccessibilityWindow:
		return "window"
	case AccessibilityContainer:
		return "container"
	case AccessibilityButton:
		return "button"
	case AccessibilityCheckBox:
		return "checkBox"
	case AccessibilityRadioButton:
		return "radioButton"
	case AccessibilityTextInput:
		return "textInput"
	case AccessibilityStaticText:
		return "staticText"
	case AccessibilityImage:
		return "image"
	case AccessibilityList:
		return "list"
	case AccessibilityListItem:
		return "listItem"
	case AccessibilityTab:
		return "tab"
	case AccessibilityMenu:
		return "menu"
	case AccessibilityMenuItem:
		return "menuItem"
	default:
		return "unknown"
	}
}

// AccessibilityNode is one recorded semantic node (upstream
// accesskit::Node).
type AccessibilityNode struct {
	// Role classifies the element.
	Role AccessibilityRole
	// Label is the accessible name.
	Label string
	// Value is the current value (text inputs).
	Value string
	// Bounds is the element's window bounds at record time (logical
	// pixels).
	Bounds geometry.Bounds[geometry.Pixels]
	// Focused reports focus containment.
	Focused bool
	// Disabled greys the control out.
	Disabled bool
	// Children indexes into the frame's node slice (assigned by the
	// window).
	Children []int
}

// AccessibilityTree is one frame's recorded semantic tree.
type AccessibilityTree struct {
	// Nodes are the recorded nodes in paint order; 0 is the root.
	Nodes []AccessibilityNode
}

// NodeAt returns the node at index (upstream tree access).
func (t *AccessibilityTree) NodeAt(index int) (AccessibilityNode, bool) {
	if t == nil || index < 0 || index >= len(t.Nodes) {
		return AccessibilityNode{}, false
	}
	return t.Nodes[index], true
}

// FindByRole returns the indexes of all nodes with the role.
func (t *AccessibilityTree) FindByRole(role AccessibilityRole) []int {
	if t == nil {
		return nil
	}
	var out []int
	for i := range t.Nodes {
		if t.Nodes[i].Role == role {
			out = append(out, i)
		}
	}
	return out
}

// FindByLabel returns the first node whose label contains the text.
func (t *AccessibilityTree) FindByLabel(text string) (AccessibilityNode, int, bool) {
	if t == nil {
		return AccessibilityNode{}, -1, false
	}
	for i := range t.Nodes {
		if t.Nodes[i].Label == text {
			return t.Nodes[i], i, true
		}
	}
	return AccessibilityNode{}, -1, false
}

// WithAccessibility records an accessibility node around f: children
// recorded inside f nest under it (upstream
// window.update_accessibility for element subtrees). Elements that paint
// children outside a closure (div) use the Append/Push/Pop primitives.
func (w *Window) WithAccessibility(role AccessibilityRole, label string, f func()) {
	node := AccessibilityNode{Role: role, Label: label}
	// Bounds default to the whole content; the enclosing element's
	// bounds refine them at record time via the current element path.
	node.Bounds = geometry.Bounds[geometry.Pixels]{
		Origin: geometry.Point[geometry.Pixels]{},
		Size:   w.plat.ContentSize(),
	}
	idx := w.AppendA11yNode(node)
	w.PushA11yParent(idx)
	defer w.PopA11yParent(idx)
	f()
}

// AppendA11yNode records one node and returns its index.
func (w *Window) AppendA11yNode(node AccessibilityNode) int {
	w.nextFrame.a11yNodes = append(w.nextFrame.a11yNodes, node)
	return len(w.nextFrame.a11yNodes) - 1
}

// NextA11yNodes exposes the pending node slice (for open/close-style
// recorders like div).
func (w *Window) NextA11yNodes() []AccessibilityNode {
	return w.nextFrame.a11yNodes
}

// PushA11yParent opens a node so later nodes nest under it.
func (w *Window) PushA11yParent(idx int) {
	w.a11yStack = append(w.a11yStack, idx)
}

// PopA11yParent closes the node: it nests under the enclosing open node
// (upstream accesskit parenting).
func (w *Window) PopA11yParent(idx int) {
	if n := len(w.a11yStack); n > 0 {
		w.a11yStack = w.a11yStack[:n-1]
	}
	if len(w.a11yStack) > 0 {
		p := w.a11yStack[len(w.a11yStack)-1]
		w.nextFrame.a11yNodes[p].Children = append(w.nextFrame.a11yNodes[p].Children, idx)
	}
}

// SetAccessibilityValue updates the recording node's value (text inputs).
func (w *Window) SetAccessibilityValue(value string) {
	if len(w.a11yStack) > 0 {
		i := w.a11yStack[len(w.a11yStack)-1]
		w.nextFrame.a11yNodes[i].Value = value
	}
}

// SetAccessibilityBounds updates the recording node's bounds.
func (w *Window) SetAccessibilityBounds(bounds geometry.Bounds[geometry.Pixels]) {
	if len(w.a11yStack) > 0 {
		i := w.a11yStack[len(w.a11yStack)-1]
		w.nextFrame.a11yNodes[i].Bounds = bounds
	}
}

// MarkAccessibilityFocused marks the recording node focused.
func (w *Window) MarkAccessibilityFocused() {
	if len(w.a11yStack) > 0 {
		i := w.a11yStack[len(w.a11yStack)-1]
		w.nextFrame.a11yNodes[i].Focused = true
	}
}

// MarkAccessibilityDisabled marks the recording node disabled.
func (w *Window) MarkAccessibilityDisabled() {
	if len(w.a11yStack) > 0 {
		i := w.a11yStack[len(w.a11yStack)-1]
		w.nextFrame.a11yNodes[i].Disabled = true
	}
}

// RecordedAccessibility returns the finished tree of the last rendered
// frame (upstream window.accessibility_tree).
func (w *Window) RecordedAccessibility() *AccessibilityTree {
	return &AccessibilityTree{Nodes: w.renderedFrame.a11yNodes}
}
