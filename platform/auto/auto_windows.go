//go:build windows

package auto

import (
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/windows"
)

// New returns the native Windows platform. It uses the Win32 message loop and
// normally presents with Direct3D 11, with a software rendering fallback.
func New() (platform.Platform, error) {
	return windows.NewPlatform()
}
