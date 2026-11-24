//go:build windows

package auto

import (
	"github.com/FlameInTheDark/go-gpui/platform"
	"github.com/FlameInTheDark/go-gpui/platform/windows"
)

// New returns the native Windows (win32) platform: real windows via
// CreateWindowExW, software-rasterized frames presented with
// SetDIBitsToDevice, and input through the standard message loop.
func New() (platform.Platform, error) {
	return windows.NewPlatform()
}
