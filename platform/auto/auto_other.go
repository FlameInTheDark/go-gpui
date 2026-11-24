//go:build !windows && !linux && !freebsd && !openbsd && !netbsd

package auto

import (
	"errors"

	"github.com/FlameInTheDark/go-gpui/platform"
)

// New returns an error on platforms without a native backend (macOS is
// not yet ported). Use the headless platform to render without a window.
func New() (platform.Platform, error) {
	return nil, errors.New("auto: no native platform backend on this OS; use --headless to render to PNGs")
}
