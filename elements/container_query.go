package elements

import (
	"image"

	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/gpui"
	"github.com/FlameInTheDark/go-gpui/style"
)

// ContainerQuery is a container query element, in the spirit of CSS
// container queries (upstream elements/container_query.rs): the element's
// own size is determined solely by its style and the space offered by its
// parent, and the render callback receives that size.
//
// By default the element fills its parent (equivalent to SizeFull);
// use the Styled methods to size it differently. Because the contents
// don't exist until after layout, they cannot influence the element's
// size.
//
//	container_query.New(func(size geometry.Size[geometry.Pixels], w *gpui.Window) gpui.Element {
//	        if size.Width < geometry.Px(240) {
//	                return elements.Div().Child(elements.Text("Narrow layout"))
//	        }
//	        return elements.Div().Child(elements.Text("Wide layout"))
//	})
type ContainerQuery struct {
	render func(size geometry.Size[geometry.Pixels], w *gpui.Window) gpui.Element
	style  style.StyleRefinement

	// child is created during prepaint and painted during paint.
	child gpui.Element
}

// NewContainerQuery constructs a container query element with the given
// render callback (upstream container_query()). The callback receives the
// size the element was assigned during layout and returns the contents to
// display within it.
func NewContainerQuery(render func(size geometry.Size[geometry.Pixels], w *gpui.Window) gpui.Element) *ContainerQuery {
	cq := &ContainerQuery{render: render}
	// Default style: fill the parent (upstream sets relative(1.) on both
	// axes).
	cq.style.W(geometry.LengthFromDefinite(geometry.RelativeDefinite(1)))
	cq.style.H(geometry.LengthFromDefinite(geometry.RelativeDefinite(1)))
	return cq
}

// StyleRef implements Styled.
func (c *ContainerQuery) StyleRef() *style.StyleRefinement { return &c.style }

// ElementID implements Element.
func (c *ContainerQuery) ElementID() gpui.ElementId { return gpui.ElementId{} }

// RequestLayout implements Element.
func (c *ContainerQuery) RequestLayout(w *gpui.Window) {
	st := style.DefaultStyle()
	st.Refine(&c.style)
	w.RequestLayout(st, nil)
}

// Prepaint implements Element: builds the contents with the assigned size
// and lays them out as a root at this element's bounds (upstream prepaint).
func (c *ContainerQuery) Prepaint(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {
	if c.render == nil {
		return
	}
	child := c.render(bounds.Size, w)
	if child == nil {
		c.child = nil
		return
	}
	c.child = child
	// Layout the child as a root within the definite assigned size, then
	// prepaint it at this element's origin (upstream layout_as_root +
	// prepaint_at).
	node := w.ComputeElementLayout(child, bounds.Size)
	w.PrepaintElementAt(child, node, bounds.Origin)
}

// Paint implements Element.
func (c *ContainerQuery) Paint(w *gpui.Window) {
	if c.child != nil {
		c.child.Paint(w)
	}
}

// Surface is a surface element (upstream elements/surface.rs): it paints a
// platform surface — a hardware video frame — into the scene. Upstream
// sources CoreVideo pixel buffers on macOS; this port accepts a decoded
// image (the platform-agnostic equivalent) and paints it through the
// image path with the same object-fit semantics.
type Surface struct {
	source    image.Image
	objectFit style.ObjectFit
	style     style.StyleRefinement
	grayscale bool
}

// NewSurface creates a new surface element showing src (upstream
// surface()).
func NewSurface(src image.Image) *Surface {
	return &Surface{source: src, objectFit: style.ObjectFitContain}
}

// ObjectFit sets how the surface content is fitted into the element
// bounds (upstream Surface::object_fit).
func (s *Surface) ObjectFit(fit style.ObjectFit) *Surface {
	s.objectFit = fit
	return s
}

// Grayscale renders the surface without color (parity with Img).
func (s *Surface) Grayscale(v bool) *Surface {
	s.grayscale = v
	return s
}

// StyleRef implements Styled.
func (s *Surface) StyleRef() *style.StyleRefinement { return &s.style }

// ElementID implements Element.
func (s *Surface) ElementID() gpui.ElementId { return gpui.ElementId{} }

// RequestLayout implements Element.
func (s *Surface) RequestLayout(w *gpui.Window) {
	st := style.DefaultStyle()
	st.Refine(&s.style)
	w.RequestLayout(st, nil)
}

// Prepaint implements Element.
func (s *Surface) Prepaint(w *gpui.Window, bounds geometry.Bounds[geometry.Pixels]) {}

// Paint implements Element: fits the surface content into the bounds and
// paints it (upstream paint via window.paint_surface).
func (s *Surface) Paint(w *gpui.Window) {
	if s.source == nil {
		return
	}
	bounds := w.ElementBounds(s)
	srcSize := s.source.Bounds().Size()
	fitBounds := objectFitBounds(s.objectFit, bounds, srcSize.X, srcSize.Y)
	w.PaintImage(fitBounds, geometry.ZeroCornersAbsolute().ToPixels(w.RemSize()), s.source, s.grayscale)
}
