// Package style defines the CSS-like style system of go-gpui: the resolved
// [Style], its partial form [StyleRefinement], the text style and the
// Tailwind-inspired builder API that is used to configure elements.
//
// # Style and StyleRefinement
//
// [Style] is a complete description of how one element is laid out and
// painted. Almost nothing creates a Style by hand; instead elements carry a
// [StyleRefinement], a mirror of Style in which every property is optional
// (a nil pointer means "not set"). Refinements are cheap to build, merge and
// stack, which is what makes the style cascade work:
//
//  1. Start from [DefaultStyle] (display flex, direction row, auto sizes, ...).
//  2. Apply the element's own refinement with [Style.Refine].
//  3. While an interaction state is active (hover, pressed, focused) apply
//     the state's refinement on top, again with [Style.Refine].
//
// Refine only overwrites properties that are set, and composite properties
// (size, margin, padding, border widths, corner radii, insets, gap, overflow
// and text) override per component: setting the width leaves the height alone.
// [StyleRefinement.Merge] combines two refinements into one without touching
// a base style, and [StyleRefinement.Clone] makes a mutable copy.
//
// Text properties are inherited: [TextStyle] and [TextStyleRefinement] hold
// them, and a child resolves its text style starting from its parent's.
//
// # The builder API
//
// All builder methods live on *[StyleRefinement], mutate it in place and
// return it so calls chain. They follow Tailwind CSS naming:
//
//   - Hand-written methods take typed lengths or enums: W, H, P, M, SetGap,
//     Top, Rounded, Border, Bg, TextColor, FlexCol, ItemsCenter, JustifyBetween,
//     OverflowHidden, CursorPointer and so on (see styled.go).
//   - Generated scale methods take no arguments and encode the value in the
//     name: W_4, P_2, GapX_1p5, M_Neg_2, Rounded_Md, ShadowLg,
//     TextXs. They are produced by cmd/genstyles into styled_gen.go; edit the
//     generator, not the generated file.
//
// The numeric scale is Tailwind's: one unit is 0.25rem, so P_4 is 1rem,
// W_64 is 16rem. The suffix "px" means one pixel, "full" means 100% of the
// parent, N_M suffixes such as W_1_2 or W_2_3 are fractions of the parent,
// "auto" is where CSS allows it, and "Neg" marks negative margins and insets
// (M_Neg_4 is -1rem).
//
// Interaction states (hover, active, focus) are not stored on StyleRefinement
// itself: the elements package keeps one refinement per state and applies it
// on top of the base style when the state is active.
//
// # Lengths
//
// Sizes use the length types of the geometry package: Length (px, rem,
// fraction or auto) for sizes, margins and insets; DefiniteLength (no auto)
// for padding, gap and line height; AbsoluteLength (px or rem only) for
// border widths, corner radii, font size and scrollbar width. Rem lengths
// resolve against the window's rem size, 16 logical pixels by default.
//
// # Example
//
//	r := style.NewStyleRefinement().
//		Flex().FlexCol().
//		W_64().P_4().Gap_2().
//		BgColor(color.White()).
//		Rounded_Md().ShadowSm().
//		TextColor(color.Black()).TextSm()
//
//	// Resolve for painting or layout:
//	s := style.DefaultStyle()
//	s.Refine(r)
package style
