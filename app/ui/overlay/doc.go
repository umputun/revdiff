// Package overlay owns all layered popup UI for revdiff — help, annotation list,
// and theme selector overlays. It provides a Manager coordinator that enforces
// mutual exclusivity (only one overlay visible at a time), routes key dispatch
// to the active overlay, and composes the overlay on top of the base view via
// ANSI-aware centered compositing.
//
// Callers supply fully populated spec structs (HelpSpec, AnnotListSpec, ThemeSelectSpec)
// when opening an overlay and handle side effects by switching on the returned Outcome
// from HandleKey. The overlay package has no dependency on ui.Model, annotation store,
// theme loading, or any filesystem operation.
package overlay
