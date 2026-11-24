package gpui

// Menu types (upstream gpui::Menu / gpui::MenuItem from platform.rs,
// re-exported at the crate root). Upstream builds native menus on macOS;
// the other platforms render an in-app menu bar (elements.MenuBar reads
// the window's app menu). Items dispatch typed actions through the
// window's action pipeline or a direct handler.

// MenuItem is one entry of a menu (upstream MenuItem: Action /
// Separator / Submenu).
type MenuItem struct {
	// Label is the displayed text ("" for separators).
	Label string
	// Action is a typed action value dispatched via DispatchAction on
	// click (upstream MenuItem::Action(Box<dyn Action>)).
	Action any
	// Handler runs directly on click when Action is nil (the portable
	// alternative for non-registered actions).
	Handler func(w *Window)
	// Keystroke is the binding hint displayed right-aligned (e.g.
	// "ctrl+s"; upstream reads it from the keymap through the action
	// name).
	Keystroke string
	// Separator renders a horizontal rule instead of an entry.
	Separator bool
	// Disabled greys the entry out and blocks dispatch.
	Disabled bool
}

// MenuAction builds an action item.
func MenuAction(label string, action any) MenuItem {
	return MenuItem{Label: label, Action: action}
}

// MenuHandler builds a handler item.
func MenuHandler(label string, handler func(w *Window)) MenuItem {
	return MenuItem{Label: label, Handler: handler}
}

// MenuKeystroke sets the binding hint of the item (builder).
func (i MenuItem) MenuKeystroke(keystroke string) MenuItem {
	i.Keystroke = keystroke
	return i
}

// MenuDisabled marks the item disabled (builder).
func (i MenuItem) MenuDisabled() MenuItem {
	i.Disabled = true
	return i
}

// MenuSeparator builds a separator entry.
func MenuSeparator() MenuItem {
	return MenuItem{Separator: true}
}

// Menu is one top-level menu (upstream Menu { name, items }).
type Menu struct {
	// Name is the menu-bar title.
	Name string
	// Items are the dropdown entries.
	Items []MenuItem
}

// NewMenu builds a menu from its name and items.
func NewMenu(name string, items ...MenuItem) Menu {
	return Menu{Name: name, Items: items}
}

// SetAppMenu installs the window's application menu (upstream
// App::menubar / cx.set_menus for the in-app bar on Windows/Linux).
func (w *Window) SetAppMenu(menus []Menu) {
	w.appMenu = menus
	w.dirty = true
	if w.drawPhase == PhaseNone {
		w.ScheduleFrame()
	}
}

// AppMenu returns the installed application menu.
func (w *Window) AppMenu() []Menu { return w.appMenu }

// DispatchMenuItem runs a menu item: a typed action dispatches through
// the action pipeline, else the handler runs. Disabled items do nothing.
func (w *Window) DispatchMenuItem(item MenuItem) {
	if item.Disabled || item.Separator {
		return
	}
	if item.Action != nil {
		w.DispatchAction(item.Action)
		return
	}
	if item.Handler != nil {
		item.Handler(w)
	}
}
