//go:build linux || freebsd || openbsd || netbsd

package auto

import (
	"errors"
	"os"

	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/wayland"
	"github.com/FlameInTheDark/go-gpui/platform/x11"
)

// New returns the native Unix platform. X11 is preferred (XWayland covers
// Wayland desktops); a Wayland-only session falls back to the native
// Wayland backend.
func New() (platform.Platform, error) {
	if os.Getenv("DISPLAY") != "" {
		return x11.NewPlatform()
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return wayland.NewPlatform()
	}
	return nil, errors.New("auto: no display server found (neither $DISPLAY nor $WAYLAND_DISPLAY is set); " +
		"run under Xvfb/Wayland or use --headless to render to PNGs")
}
