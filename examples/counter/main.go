// Command counter shows a single-window app using the top-level package.
package main

import (
	"fmt"
	"log"

	"github.com/FlameInTheDark/go-gpui"
)

func main() {
	count := 0
	err := gpui.Run(gpui.WindowOptions{Title: "Counter"}, func(_ *gpui.Window) gpui.Element {
		return gpui.Column(
			gpui.Text(fmt.Sprintf("Count: %d", count)),
			gpui.Div(gpui.Text("Add one")).
				P_2().Rounded_Md().BgColor(gpui.RGB(0x2563eb)).CursorPointer().
				OnClick(func(_ gpui.ClickEvent, w *gpui.Window) {
					count++
					w.Refresh()
				}),
		).Size_Full().ItemsCenter().JustifyCenter().Gap_2().
			BgColor(gpui.RGB(0x18212f)).TextColor(gpui.RGB(0xffffff))
	})
	if err != nil {
		log.Fatal(err)
	}
}
