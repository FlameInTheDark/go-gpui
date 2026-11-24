package text

import (
	"reflect"
	"testing"
)

// TestWrapWithIndentKeepsBlockIndent verifies that wrapped block-indented
// lines keep their leading indent on every continuation line (upstream
// IndentAdjustment::SameIndent), and that the indent counts toward the row
// width like upstream adds indent_width to the row width after each wrap.
func TestWrapWithIndentKeepsBlockIndent(t *testing.T) {
	got := WrapWithIndent("    hello world foo bar", 20, 4)
	want := []string{"    hello world foo", "    bar"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WrapWithIndent = %q, want %q", got, want)
	}

	// Tab indent measured in indentWidth columns: tab(4) + "foo bar" (7)
	// fits width 12, " baz" would exceed it.
	got = WrapWithIndent("\tfoo bar baz", 12, 4)
	want = []string{"\tfoo bar", "\tbaz"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tab indent = %q, want %q", got, want)
	}
}

// TestWrapWithIndentShortLines verifies short lines produce no continuation.
func TestWrapWithIndentShortLines(t *testing.T) {
	if got := WrapWithIndent("hi", 20, 4); !reflect.DeepEqual(got, []string{"hi"}) {
		t.Fatalf("short line = %q, want [hi]", got)
	}
	if got := WrapWithIndent("", 20, 4); !reflect.DeepEqual(got, []string{""}) {
		t.Fatalf("empty text = %q, want one empty row", got)
	}
	// Exactly at the width: no wrap.
	if got := WrapWithIndent("12345", 5, 4); !reflect.DeepEqual(got, []string{"12345"}) {
		t.Fatalf("exact fit = %q, want no continuation", got)
	}
}

// TestWrapWithIndentUnindentsBlankishRow verifies the up/down adjustment
// (upstream base_indent = first_non_whitespace_ix - last_wrap_ix): when the
// leading indent fills a whole row, the first visual row is blank-ish and the
// continuation unindents by one level (one indentWidth), so wrapped rows do
// not degrade into cascading whitespace-only rows.
func TestWrapWithIndentUnindentsBlankishRow(t *testing.T) {
	// 8-space indent fills width 8 entirely: first row is the blank indent
	// row, continuations unindent one level (4 columns).
	got := WrapWithIndent("        deep text", 8, 4)
	want := []string{"        ", "    deep", "    text"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("blank-ish row = %q, want %q", got, want)
	}

	// Two tabs (8 columns) at width 8, level 4: the blank first row keeps
	// the visible tabs, continuations unindent one tab (4 columns).
	got = WrapWithIndent("\t\tdeep text", 8, 4)
	want = []string{"\t\t", "\tdeep", "\ttext"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tab blank-ish row = %q, want %q", got, want)
	}

	// All-whitespace line: single row, no continuation (upstream wraps
	// all-whitespace lines with indent 0).
	if got := WrapWithIndent("      ", 8, 4); !reflect.DeepEqual(got, []string{"      "}) {
		t.Fatalf("all-whitespace = %q, want single row", got)
	}
}

// TestWrapWithIndentMultiLine wraps each physical line independently.
func TestWrapWithIndentMultiLine(t *testing.T) {
	got := WrapWithIndent("aa bb\n  cc dd", 5, 4)
	want := []string{"aa bb", "  cc", "  dd"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("multiline = %q, want %q", got, want)
	}
}

// TestWrapWithIndentHardSplit verifies oversized words are hard-split at the
// column budget (upstream keeps oversized graphemes intact rather than
// dropping them).
func TestWrapWithIndentHardSplit(t *testing.T) {
	got := WrapWithIndent("aaaaaaaaaa bb", 5, 4)
	want := []string{"aaaaa", "aaaaa", "bb"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hard split = %q, want %q", got, want)
	}
}
