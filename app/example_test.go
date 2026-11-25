package app_test

import (
	"fmt"

	"github.com/FlameInTheDark/go-gpui/app"
)

func ExampleSubscribe() {
	type counter struct{ value int }
	type changed struct{ value int }

	a := app.NewTestApp()
	e := app.NewEntity(a, func(*app.Context[counter]) counter { return counter{} })
	listener := app.NewEntity(a, func(*app.Context[struct{}]) struct{} { return struct{}{} })
	defer e.Release()
	defer listener.Release()

	// Registration is activated when this update's effects are flushed.
	listener.Update(a, func(_ *struct{}, cx *app.Context[struct{}]) any {
		app.Subscribe(cx, e, func(event *changed, _ app.Entity[counter], _ *app.Context[struct{}]) {
			fmt.Println(event.value)
		})
		return nil
	})

	e.Update(a, func(state *counter, cx *app.Context[counter]) any {
		state.value++
		cx.Notify() // State observers and views can react to the change.
		cx.Emit(changed{value: state.value})
		return nil
	})
	// Output: 1
}
