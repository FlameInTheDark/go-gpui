// Package platform defines the platform abstraction layer: input events,
// window options and the Platform / PlatformWindow interfaces that OS
// backends implement (headless, X11, Wayland, Windows).
package platform

import (
	"image"
	"time"

	"github.com/FlameInTheDark/go-gpui/color"
	"github.com/FlameInTheDark/go-gpui/geometry"
	"github.com/FlameInTheDark/go-gpui/keymap"
	"github.com/FlameInTheDark/go-gpui/render"
)

// ---------------------------------------------------------------------------
// Input events (port of PlatformInput)
// ---------------------------------------------------------------------------

// MouseButton identifies a mouse button.
type MouseButton uint8

const (
	// ButtonLeft is the primary button.
	ButtonLeft MouseButton = 0
	// ButtonRight is the secondary button.
	ButtonRight MouseButton = 1
	// ButtonMiddle is the tertiary button.
	ButtonMiddle MouseButton = 2
	// ButtonNavigate is back/forward buttons.
	ButtonNavigate MouseButton = 3
)

// NavigationDirection is back or forward.
type NavigationDirection uint8

const (
	// NavigateBack is the back button.
	NavigateBack NavigationDirection = 0
	// NavigateForward is the forward button.
	NavigateForward NavigationDirection = 1
)

// MouseDownEvent is a button press.
type MouseDownEvent struct {
	Button     MouseButton
	Position   geometry.Point[geometry.Pixels]
	Modifiers  keymap.Modifiers
	ClickCount int
	FirstMouse bool
}

// IsFocusing reports whether the press should focus.
func (e MouseDownEvent) IsFocusing() bool { return e.Button == ButtonLeft }

// MouseUpEvent is a button release.
type MouseUpEvent struct {
	Button     MouseButton
	Position   geometry.Point[geometry.Pixels]
	Modifiers  keymap.Modifiers
	ClickCount int
}

// MouseMoveEvent is pointer motion.
type MouseMoveEvent struct {
	Position      geometry.Point[geometry.Pixels]
	PressedButton *MouseButton
	Modifiers     keymap.Modifiers
}

// Dragging reports whether the left button is held.
func (e MouseMoveEvent) Dragging() bool {
	return e.PressedButton != nil && *e.PressedButton == ButtonLeft
}

// MouseExitEvent is pointer leaving the window.
type MouseExitEvent struct {
	Position      geometry.Point[geometry.Pixels]
	PressedButton *MouseButton
	Modifiers     keymap.Modifiers
}

// ScrollDelta is scroll motion in pixels or lines.
type ScrollDelta struct {
	kind   uint8 // 0 = pixels, 1 = lines
	Pixels geometry.Point[geometry.Pixels]
	Lines  geometry.Point[float32]
}

// ScrollDeltaPixels constructs pixel delta.
func ScrollDeltaPixels(x, y geometry.Pixels) ScrollDelta {
	return ScrollDelta{kind: 0, Pixels: geometry.Point[geometry.Pixels]{X: x, Y: y}}
}

// ScrollDeltaLines constructs line delta.
func ScrollDeltaLines(x, y float32) ScrollDelta {
	return ScrollDelta{kind: 1, Lines: geometry.Point[float32]{X: x, Y: y}}
}

// Precise reports pixel-precision scrolling (trackpads).
func (d ScrollDelta) Precise() bool { return d.kind == 0 }

// PixelDelta converts to pixels at the given line height.
func (d ScrollDelta) PixelDelta(lineHeight geometry.Pixels) geometry.Point[geometry.Pixels] {
	if d.kind == 0 {
		return d.Pixels
	}
	return geometry.Point[geometry.Pixels]{
		X: geometry.Px(d.Lines.X * lineHeight.AsF32()),
		Y: geometry.Px(d.Lines.Y * lineHeight.AsF32()),
	}
}

// Coalesce merges same-sign deltas (sums) or replaces on sign change.
func (d ScrollDelta) Coalesce(o ScrollDelta) ScrollDelta {
	if d.kind != o.kind {
		return o
	}
	if d.kind == 0 {
		return ScrollDeltaPixels(
			coalescePx(d.Pixels.X, o.Pixels.X),
			coalescePx(d.Pixels.Y, o.Pixels.Y),
		)
	}
	return ScrollDeltaLines(
		coalesceF(d.Lines.X, o.Lines.X),
		coalesceF(d.Lines.Y, o.Lines.Y),
	)
}

func coalescePx(a, b geometry.Pixels) geometry.Pixels {
	if a == 0 || (a > 0) == (b > 0) {
		return a.Add(b)
	}
	return b
}

func coalesceF(a, b float32) float32 {
	if a == 0 || (a > 0) == (b > 0) {
		return a + b
	}
	return b
}

// TouchPhase is the lifecycle of a touch contact.
type TouchPhase uint8

const (
	TouchPhaseStarted   TouchPhase = 0
	TouchPhaseMoved     TouchPhase = 1
	TouchPhaseEnded     TouchPhase = 2
	TouchPhaseCancelled TouchPhase = 3
)

// ScrollSource distinguishes pointer wheel from touch scrolling.
type ScrollSource uint8

const (
	ScrollSourcePointer ScrollSource = 0
	ScrollSourceTouch   ScrollSource = 1
)

// ScrollEvent is a scroll wheel/trackpad event.
type ScrollEvent struct {
	Position   geometry.Point[geometry.Pixels]
	Delta      ScrollDelta
	Modifiers  keymap.Modifiers
	TouchPhase TouchPhase
	Source     ScrollSource
}

// PinchEvent is a trackpad pinch.
type PinchEvent struct {
	Position  geometry.Point[geometry.Pixels]
	Delta     float32
	Modifiers keymap.Modifiers
	Phase     TouchPhase
}

// KeyDownEvent is a key press.
type KeyDownEvent struct {
	Keystroke            keymap.Keystroke
	IsHeld               bool
	PreferCharacterInput bool
}

// KeyUpEvent is a key release.
type KeyUpEvent struct {
	Keystroke keymap.Keystroke
}

// ModifiersChangedEvent reports a modifier state change.
type ModifiersChangedEvent struct {
	Modifiers keymap.Modifiers
	Capslock  keymap.Capslock
}

// TouchID identifies a touch contact.
type TouchID uint64

// TouchEvent is a raw touch contact event.
type TouchEvent struct {
	ID                TouchID
	Phase             TouchPhase
	Position          geometry.Point[geometry.Pixels]
	PredictedPosition *geometry.Point[geometry.Pixels]
	Force             *float32
}

// FileDropEvent is an external drag-and-drop update.
type FileDropEvent struct {
	Kind     FileDropKind
	Position geometry.Point[geometry.Pixels]
	Paths    []string
}

// CompositionPhase is the lifecycle of an IME composition session
// (upstream IME input, IMEKey handling in platform backends).
type CompositionPhase uint8

const (
	// CompositionStarted means the IME began composing (upstream
	// WM_IME_STARTCOMPOSITION / mac first marked-text update).
	CompositionStarted CompositionPhase = 0
	// CompositionUpdated means the preedit string changed.
	CompositionUpdated CompositionPhase = 1
	// CompositionEnded means composing finished: MarkedText carries the
	// committed text (possibly empty when cancelled).
	CompositionEnded CompositionPhase = 2
)

// CompositionEvent is an IME composition update (upstream
// ImeComposition: marked_text, marked_range, selected_range,
// replaced_range, cursor_offset). Ranges index the CURRENT buffer in
// UTF-16 units (platform convention: upstream indexes UTF-16 code units).
type CompositionEvent struct {
	// Phase is the lifecycle stage of this update.
	Phase CompositionPhase
	// MarkedText is the preedit string; on CompositionEnded it is the
	// committed text (empty when the composition was cancelled).
	MarkedText string
	// MarkedRange is the half-open range replaced by MarkedText
	// (upstream marked_range; nil when unmarked).
	MarkedRange *[2]int
	// SelectedRange is the highlighted range inside the composition
	// (upstream selected_range, e.g. the converted clause).
	SelectedRange [2]int
	// ReplacedRange is the buffer range the composition replaces
	// (upstream replaced_range).
	ReplacedRange [2]int
	// CursorOffset is the caret offset inside MarkedText (upstream
	// cursor_offset; nil when the IME reports none).
	CursorOffset *int
}

func (CompositionEvent) isInput() {}

// TouchDragEvent is a direct touch drag claimed by an element before touch
// input becomes a tap, long press, or scrolling gesture (upstream
// gestures::TouchDragEvent).
type TouchDragEvent struct {
	// Phase is the phase of the touch drag.
	Phase TouchPhase
	// StartPosition is where the touch started.
	StartPosition geometry.Point[geometry.Pixels]
	// Position is the touch's current position.
	Position geometry.Point[geometry.Pixels]
}

// LongPressEvent is a phased long-press gesture recognized from a touch
// (upstream gestures::LongPressEvent).
type LongPressEvent struct {
	// Phase is the phase of the long press.
	Phase TouchPhase
	// StartPosition is where the touch started.
	StartPosition geometry.Point[geometry.Pixels]
	// Position is the touch's current position.
	Position geometry.Point[geometry.Pixels]
}

// BackGesturePhase is the phase of a BackGestureEvent.
type BackGesturePhase uint8

const (
	// BackGestureStarted means the gesture began; claiming it here routes
	// the remaining phases to the same element.
	BackGestureStarted BackGesturePhase = 0
	// BackGestureProgressed means the gesture moved (progress changed).
	BackGestureProgressed BackGesturePhase = 1
	// BackGestureCommitted means the gesture ended and the platform
	// decided to go back.
	BackGestureCommitted BackGesturePhase = 2
	// BackGestureCancelled means the gesture ended without going back.
	BackGestureCancelled BackGesturePhase = 3
)

// BackGestureEvent is a back-navigation gesture, such as the iOS swipe
// from the leading screen edge (upstream gestures::BackGestureEvent).
type BackGestureEvent struct {
	// Phase is the gesture phase.
	Phase BackGesturePhase
	// StartPosition is where the gesture started.
	StartPosition geometry.Point[geometry.Pixels]
	// Progress is how far the gesture has travelled toward going back,
	// from 0 to 1.
	Progress float32
}

func (TouchDragEvent) isInput()   {}
func (LongPressEvent) isInput()   {}
func (BackGestureEvent) isInput() {}

// FileDropKind discriminates FileDropEvent.
type FileDropKind uint8

const (
	// FileDropEntered means a drag entered the window with files.
	FileDropEntered FileDropKind = 0
	// FileDropPending means the drag moved.
	FileDropPending FileDropKind = 1
	// FileDropSubmit means the drag was released.
	FileDropSubmit FileDropKind = 2
	// FileDropExited means the drag left the window.
	FileDropExited FileDropKind = 3
	// FileDropEnded means the drag ended entirely.
	FileDropEnded FileDropKind = 4
)

// Input is a type-erased platform input event.
type Input interface{ isInput() }

func (MouseDownEvent) isInput()        {}
func (MouseUpEvent) isInput()          {}
func (MouseMoveEvent) isInput()        {}
func (MouseExitEvent) isInput()        {}
func (ScrollEvent) isInput()           {}
func (PinchEvent) isInput()            {}
func (KeyDownEvent) isInput()          {}
func (KeyUpEvent) isInput()            {}
func (ModifiersChangedEvent) isInput() {}
func (TouchEvent) isInput()            {}
func (FileDropEvent) isInput()         {}

// DispatchEventResult is the outcome of routing an input event through the
// app.
type DispatchEventResult struct {
	Propagate        bool
	DefaultPrevented bool
}

// ---------------------------------------------------------------------------
// Window options
// ---------------------------------------------------------------------------

// WindowKind selects the windowing mode.
type WindowKind uint8

const (
	// WindowKindNormal is a standard window.
	WindowKindNormal WindowKind = 0
	// WindowKindPopUp is an override-redirect popup.
	WindowKindPopUp WindowKind = 1
	// WindowKindFloating is a floating utility window.
	WindowKindFloating WindowKind = 2
	// WindowKindDialog is a modal dialog.
	WindowKindDialog WindowKind = 3
)

// WindowBounds describes the initial window geometry.
type WindowBounds struct {
	Kind   WindowBoundsKind
	Bounds geometry.Bounds[geometry.Pixels]
}

// WindowBoundsKind selects fixed/maximized/fullscreen.
type WindowBoundsKind uint8

const (
	// WindowBoundsWindowed uses the bounds as-is.
	WindowBoundsWindowed WindowBoundsKind = 0
	// WindowBoundsMaximized maximizes the window.
	WindowBoundsMaximized WindowBoundsKind = 1
	// WindowBoundsFullscreen fullscreens the window.
	WindowBoundsFullscreen WindowBoundsKind = 2
)

// WindowedBounds constructs windowed bounds.
func WindowedBounds(b geometry.Bounds[geometry.Pixels]) WindowBounds {
	return WindowBounds{Kind: WindowBoundsWindowed, Bounds: b}
}

// TitlebarOptions configures the title bar.
type TitlebarOptions struct {
	Title              *string
	AppearsTransparent bool
}

// WindowBackgroundAppearance selects the window compositing mode.
type WindowBackgroundAppearance uint8

const (
	// BackgroundOpaque renders an opaque window.
	BackgroundOpaque WindowBackgroundAppearance = 0
	// BackgroundTransparent renders with transparency.
	BackgroundTransparent WindowBackgroundAppearance = 1
	// BackgroundBlurred renders with background blur.
	BackgroundBlurred WindowBackgroundAppearance = 2
)

// WindowDecorations selects client vs server decorations.
type WindowDecorations uint8

const (
	// DecorationsServer uses the WM's titlebar.
	DecorationsServer WindowDecorations = 0
	// DecorationsClient draws decorations in-app.
	DecorationsClient WindowDecorations = 1
)

// WindowOptions configures a new window.
type WindowOptions struct {
	WindowBounds      *WindowBounds
	Titlebar          *TitlebarOptions
	Focus             bool
	Show              bool
	Kind              WindowKind
	IsMovable         bool
	IsResizable       bool
	IsMinimizable     bool
	DisplayID         *uint32
	WindowBackground  WindowBackgroundAppearance
	WindowMinSize     *geometry.Size[geometry.Pixels]
	WindowDecorations *WindowDecorations
	AppID             string
	Title             string
	Icon              []byte
	// WindowBackgroundPx is the headless framebuffer size.
	WindowBackgroundPx *geometry.Size[geometry.Pixels]
	ScaleFactor        float32
}

// RequestFrameOptions controls frame scheduling requests.
type RequestFrameOptions struct {
	RequirePresentation bool
	ForceRender         bool
}

// RenderImage is a CPU-rendered frame (RGBA8 premultiplied, BGRA byte
// order is the renderer's choice; this port uses RGBA).
type RenderImage = render.RenderImage

// InputHandler is the IME/text-input contract.
type InputHandler interface {
	SelectedTextRange(ignoreDisabledInput bool) (start, end int, reversed bool, ok bool)
	MarkedTextRange() (start, end int, ok bool)
	TextForRange(start, end int) (string, bool)
	ReplaceTextInRange(start, end int, newText string)
	ReplaceAndMarkTextInRange(start, end int, newText string, newSelectedStart, newSelectedEnd int)
	UnmarkText()
	BoundsForRange(start, end int) (geometry.Bounds[geometry.Pixels], bool)
	AcceptsTextInput() bool
	TextLength() int
}

// ClipboardItem is clipboard content (upstream ClipboardItem: text plus
// an optional image entry).
type ClipboardItem struct {
	Text string
	// Image is the clipboard bitmap when present (CF_DIB/CF_DIBV5 on
	// Windows; nil when the clipboard holds no image).
	Image image.Image
}

// HasImage reports whether the item carries bitmap content.
func (i ClipboardItem) HasImage() bool { return i.Image != nil }

// FileDialogOptions configures native file dialogs (upstream
// PathPromptOptions).
type FileDialogOptions struct {
	// Files allows selecting files.
	Files bool
	// Directories allows selecting directories.
	Directories bool
	// Multiple allows selecting more than one entry.
	Multiple bool
	// Prompt is the dialog title (upstream prompt).
	Prompt string
	// FileName pre-fills the file name field of a save dialog (upstream
	// passes the suggested name to IFileDialog::SetFileName /
	// NSSavePanel::setNameFieldStringValue); ignored by open dialogs.
	FileName string
}

// PlatformWindow is a window owned by a platform backend. The gpui Window
// drives it through this interface.
type PlatformWindow interface {
	// SetTitle sets the window title.
	SetTitle(title string)
	// Title returns the window title.
	Title() string
	// Resize resizes the window.
	Resize(size geometry.Size[geometry.Pixels])
	// Bounds returns the outer bounds in logical pixels.
	Bounds() geometry.Bounds[geometry.Pixels]
	// ContentSize returns the client size in logical pixels.
	ContentSize() geometry.Size[geometry.Pixels]
	// ScaleFactor returns the HiDPI scale.
	ScaleFactor() float32
	// Appearance returns light/dark.
	Appearance() color.WindowAppearance
	// BackgroundAppearance returns the compositing mode.
	BackgroundAppearance() WindowBackgroundAppearance
	// IsMaximized reports maximized state.
	IsMaximized() bool
	// ToggleFullscreen toggles fullscreen.
	ToggleFullscreen()
	// IsFullscreen reports fullscreen state.
	IsFullscreen() bool
	// Minimize minimizes the window.
	Minimize()
	// Activate raises and focuses the window.
	Activate()
	// IsActive reports input focus.
	IsActive() bool
	// Close closes the window.
	Close()
	// IsVisible reports visibility.
	IsVisible() bool
	// Show maps/shows the window.
	Show()
	// Hide unmaps the window.
	Hide()
	// Draw presents the scene.
	Draw(scene *render.Scene)
	// SpriteAtlas returns the atlas for cache lookups.
	SpriteAtlas() *render.Atlas
	// RequestFrame schedules a frame callback.
	RequestFrame(opts RequestFrameOptions)
	// EnableCharMasking/IME hooks (simplified).
	SetInputHandler(handler InputHandler)
	TakeInputHandler() InputHandler
	// InvalidateCharacterCoordinates notifies the IME of caret movement.
	InvalidateCharacterCoordinates(bounds geometry.Bounds[geometry.Pixels])
	// Clipboard read/write.
	WriteClipboard(item ClipboardItem)
	ReadClipboard() (ClipboardItem, bool)
	// ShowWindowMenu shows the WM menu at a position.
	ShowWindowMenu(position geometry.Point[geometry.Pixels])
	// SupportsCustomDecorations reports whether this window draws
	// client-side decorations (the app's own title bar with close /
	// minimize / maximize and a draggable top strip).
	SupportsCustomDecorations() bool
	// StartWindowDrag begins a native move gesture (the custom title bar
	// calls it on a press; Windows runs the modal caption-drag loop).
	StartWindowDrag()
	// ToggleMaximize switches the maximized/restored state (the custom
	// maximize button and double-click on the title bar).
	ToggleMaximize()
}

// MouseCursorStyle selects a system cursor shape (upstream
// CursorStyle; only the shapes the port currently routes are listed).
type MouseCursorStyle uint8

const (
	// CursorDefault is the standard arrow (upstream CursorStyle::Arrow).
	CursorDefault MouseCursorStyle = 0
	// CursorPointingHand is the link pointer.
	CursorPointingHand MouseCursorStyle = 1
	// CursorText is the text input caret.
	CursorText MouseCursorStyle = 2
	// CursorResizeLeftRight is the horizontal resize cursor.
	CursorResizeLeftRight MouseCursorStyle = 3
	// CursorResizeUpDown is the vertical resize cursor.
	CursorResizeUpDown MouseCursorStyle = 4
	// CursorCrosshair is the precise cross cursor.
	CursorCrosshair MouseCursorStyle = 5
	// CursorNotAllowed is the forbidden-operation cursor.
	CursorNotAllowed MouseCursorStyle = 6
	// CursorGrab is the open hand.
	CursorGrab MouseCursorStyle = 7
	// CursorGrabbing is the closed hand.
	CursorGrabbing MouseCursorStyle = 8
)

// WindowCallbacks are the events a platform window reports back to the
// gpui Window. The backend is constructed with these.
type WindowCallbacks struct {
	OnRequestFrame func(RequestFrameOptions)
	OnInput        func(Input) DispatchEventResult
	OnActiveStatus func(bool)
	OnVisibility   func(bool)
	OnHoverStatus  func(bool)
	OnResize       func(size geometry.Size[geometry.Pixels], scaleFactor float32)
	OnMoved        func()
	OnShouldClose  func() bool
	OnClose        func()
	OnAppearance   func(color.WindowAppearance)
	// OnImePositionRequest asks the app for the current caret position
	// (logical pixels, window-relative) so the platform can place the
	// IME candidate window; nil means the origin (upstream reads it
	// via InputHandler::bounds_for_range).
	OnImePositionRequest func() geometry.Point[geometry.Pixels]
}

// Display describes a monitor.
type Display struct {
	ID          uint32
	Bounds      geometry.Bounds[geometry.Pixels]
	ScaleFactor float32
}

// Notification is a system notification (upstream
// platform::Notification: title + body).
type Notification struct {
	// Title is the notification heading.
	Title string
	// Body is the notification text.
	Body string
}

// Platform is the OS integration singleton.
type Platform interface {
	// Run runs the OS event loop until quit; onFinish is invoked on the main
	// thread once the loop starts.
	Run(onFinish func())
	// Quit stops the event loop.
	Quit()
	// OpenWindow creates a platform window.
	OpenWindow(options WindowOptions, callbacks WindowCallbacks) (PlatformWindow, error)
	// Displays lists monitors.
	Displays() []Display
	// PrimaryDisplay returns the primary monitor.
	PrimaryDisplay() (Display, bool)
	// Appearance reports the system appearance.
	Appearance() color.WindowAppearance
	// Clipboard accesses the system clipboard.
	ReadClipboard() (ClipboardItem, bool)
	WriteClipboard(item ClipboardItem)
	// PromptForPaths shows a native open dialog and returns the chosen
	// paths (upstream prompt_for_paths). ok=false means cancelled or
	// unavailable; the method must run on (or dispatch to) the main
	// thread and block until the dialog closes.
	PromptForPaths(options FileDialogOptions) ([]string, bool)
	// PromptForPath shows a native save dialog and returns the chosen
	// path (upstream prompt_for_new_path). ok=false means cancelled or
	// unavailable.
	PromptForPath(options FileDialogOptions) (string, bool)
	// OpenURL opens a URL with the system browser / handler (upstream
	// open_url).
	OpenURL(url string) error
	// SendNotification shows a system notification (upstream notify).
	SendNotification(notification Notification) error
	// CaptureDisplay captures a display's current pixels (upstream
	// capture_display). displayID 0 is the primary display; ok=false
	// means capture is unavailable.
	CaptureDisplay(displayID uint32) (image.Image, bool)
	// SetCursor applies a system cursor shape (upstream
	// set_cursor_style).
	SetCursor(style MouseCursorStyle)
	// DispatchOnMainThread runs a function on the main thread.
	DispatchOnMainThread(f func())
	// Now returns the platform clock (fake-able in tests).
	Now() time.Time
	// TextSystem returns the platform text system.
	TextSystem() TextSystem
}

// GestureTuningOptions mirrors the gestures package's tuning knobs without
// importing it (the platform package cannot depend on gestures because
// gestures depends on platform; the window converts between them).
type GestureTuningOptions struct {
	// TouchSlop is the pan/drag activation distance in pixels.
	TouchSlop float32
	// MultiTapInterval bounds double-tap accumulation.
	MultiTapInterval time.Duration
	// MultiTapSlop bounds double-tap distance in pixels.
	MultiTapSlop float32
	// LongPressDuration is the press-and-hold delay.
	LongPressDuration time.Duration
	// ScrollPhysicsKind selects the momentum model: "ios" (exponential
	// decay) or "android" (friction spline).
	ScrollPhysicsKind string
	// DecayPerMs is the exponential decay rate when Kind is "ios".
	DecayPerMs float32
	// MinFlingVelocity is the minimum release speed (px/s) that starts
	// momentum.
	MinFlingVelocity float32
}

// GestureTuning resolves the tuning registered for this process: the
// backend's preference when provided, otherwise nil (GPUI defaults).
func GestureTuning() *GestureTuningOptions { return processGestureTuning }

var processGestureTuning *GestureTuningOptions

// SetGestureTuning registers platform-specific gesture feel constants
// (upstream PlatformGestures::tuning); backends call it during startup.
func SetGestureTuning(o *GestureTuningOptions) { processGestureTuning = o }

// TextSystem is the shaping backend (see the text package for the
// concrete implementation).
type TextSystem interface {
	AddFonts(fontData [][]byte) error
	AllFontNames() []string
}
