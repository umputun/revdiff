// Package themes embeds the gallery of theme files for revdiff.
// Theme files live in gallery/ and are accessible via the FS variable.
package themes

import "embed"

// FS contains all theme files from the gallery/ directory.
//
//go:embed gallery/*
var FS embed.FS
