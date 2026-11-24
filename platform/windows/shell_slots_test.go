package windows

import "testing"

// TestShellVTableSlots verifies that every slot constant used by the shell
// file-dialog backend points at the method named in the audited vtable
// tables (shell_slots.go). The tables record the declaration order from
// shobjidl_core.h; the slice index IS the COM vtable slot.
//
// A wrong slot silently invokes a different method with mismatched
// arguments, so the mapping is locked here: editing either a constant or a
// table entry fails this test unless both change consistently.
func TestShellVTableSlots(t *testing.T) {
	cases := []struct {
		table []string
		slot  int
		name  string
	}{
		// IFileDialog (called through IFileOpenDialog/IFileSaveDialog
		// instances - they inherit every slot below 27).
		{iFileDialogVTable, slotDlgShow, "Show"},
		{iFileDialogVTable, slotDlgSetFileTypes, "SetFileTypes"},
		{iFileDialogVTable, slotDlgSetOptions, "SetOptions"},
		{iFileDialogVTable, slotDlgSetFileName, "SetFileName"},
		{iFileDialogVTable, slotDlgSetTitle, "SetTitle"},
		{iFileDialogVTable, slotDlgGetResult, "GetResult"},
		// IFileOpenDialog additions.
		{iFileOpenDialogVTable, slotOpenGetResults, "GetResults"},
		{iFileOpenDialogVTable, slotOpenGetSelectedItems, "GetSelectedItems"},
		// IShellItem / IShellItemArray.
		{iShellItemVTable, slotShellItemGetDisplayName, "GetDisplayName"},
		{iShellItemArrayVTable, slotShellItemArrayGetCount, "GetCount"},
		{iShellItemArrayVTable, slotShellItemArrayGetItemAt, "GetItemAt"},
	}
	for _, c := range cases {
		if c.slot < 0 || c.slot >= len(c.table) {
			t.Fatalf("slot %d out of range for %s", c.slot, c.name)
		}
		if got := c.table[c.slot]; got != c.name {
			t.Fatalf("vtable slot %d is %s, want %s", c.slot, got, c.name)
		}
	}
}

// TestShellVTableNamesUnique guards against typos in the tables (a
// duplicated name would shift every following entry).
func TestShellVTableNamesUnique(t *testing.T) {
	for _, table := range [][]string{
		iFileDialogVTable, iFileOpenDialogVTable, iFileSaveDialogVTable,
		iShellItemVTable, iShellItemArrayVTable,
	} {
		seen := map[string]bool{}
		for i, name := range table {
			if seen[name] {
				t.Fatalf("duplicate method %q in vtable (index %d)", name, i)
			}
			seen[name] = true
		}
	}
}

// TestShellVTableAnchors pins slot values cross-checkable against the
// platform SDK independently of the derivation comment: the IUnknown triad,
// IModalWindow::Show at 3 (IFileOpenDialog inherits it), and the two
// open-dialog additions at the slots right after IFileDialog's last method.
func TestShellVTableAnchors(t *testing.T) {
	anchors := []struct {
		table []string
		slot  int
		name  string
	}{
		{iFileOpenDialogVTable, 0, "QueryInterface"},
		{iFileOpenDialogVTable, 2, "Release"},
		{iFileOpenDialogVTable, 3, "Show"},
		{iFileOpenDialogVTable, 9, "SetOptions"},
		{iFileOpenDialogVTable, 15, "SetFileName"},
		{iFileOpenDialogVTable, 17, "SetTitle"},
		{iFileOpenDialogVTable, 18, "SetOkButtonLabel"},
		{iFileOpenDialogVTable, 19, "SetFileNameLabel"},
		{iFileOpenDialogVTable, 20, "GetResult"},
		{iFileOpenDialogVTable, 21, "AddPlace"},
		{iFileOpenDialogVTable, 22, "SetDefaultExtension"},
		{iFileOpenDialogVTable, 23, "Close"},
		{iFileOpenDialogVTable, 26, "SetFilter"},
		{iFileOpenDialogVTable, 27, "GetResults"},
		{iFileOpenDialogVTable, 28, "GetSelectedItems"},
		{iFileSaveDialogVTable, 20, "GetResult"},
		{iFileSaveDialogVTable, 27, "SetSaveAsItem"},
		{iFileSaveDialogVTable, 31, "ApplyProperties"},
		{iShellItemVTable, 3, "BindToHandler"},
		{iShellItemArrayVTable, 9, "EnumItems"},
	}
	for _, a := range anchors {
		if got := a.table[a.slot]; got != a.name {
			t.Fatalf("anchor slot %d is %s, want %s", a.slot, got, a.name)
		}
	}
}
