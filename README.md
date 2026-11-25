# go-gpui

go-gpui is a pure-Go desktop UI toolkit inspired by Rust GPUI. Build your interface from Go functions, flexbox containers and event handlers. No cgo required.

**Requires Go 1.23+.** Native windows run on Windows (Direct3D 11 with a software fallback) and Linux (X11 or Wayland, software rendering). A headless backend is available for tests. This is not a drop-in port of Rust GPUI or a collection of ready-made desktop controls.

## Quick start

In a new folder, run `go mod init example.com/hello` and then:

```sh
go get github.com/FlameInTheDark/go-gpui
```

Save this as `main.go` and run `go run .`:

```go
package main

import (
	"log"

	"github.com/FlameInTheDark/go-gpui"
)

func main() {
	err := gpui.Run(gpui.WindowOptions{Title: "Hello"}, func(_ *gpui.Window) gpui.Element {
		return gpui.Column(gpui.Text("Hello, go-gpui!")).
			Size_Full().ItemsCenter().JustifyCenter().
			BgColor(gpui.RGB(0x18212f)).TextColor(gpui.RGB(0xffffff))
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

## Next steps

- **[Documentation index](docs/index.md):** find features, guides, and package API references.
- **[Tutorial](docs/tutorial.md):** add clicks, manage state, and test without a display.
- **[State, events, and goroutines](docs/reactivity.md):** typed signals, observers, and safely returning worker results to the UI.
- **[Counter](examples/counter):** run `go run ./examples/counter` from a checkout for a small interactive app.
- **[More examples](docs/examples.md):** task list, gallery, and virtual lists.
- **[Flint](https://github.com/FlameInTheDark/flint):** the former editor example, now a separate project built with go-gpui.
- **[Architecture](docs/architecture.md) · [Go API](https://pkg.go.dev/github.com/FlameInTheDark/go-gpui) · [Releases](docs/releases.md)**

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
