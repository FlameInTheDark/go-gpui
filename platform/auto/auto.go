// Package auto selects the native platform backend for the current OS at
// build time: win32 on Windows, X11 (or native Wayland) on Linux and the
// BSDs. Applications call auto.New() once and drive the returned
// platform.Platform; examples accept a --headless flag that swaps in the
// headless backend for PNG-rendering and self-verification instead.
package auto
