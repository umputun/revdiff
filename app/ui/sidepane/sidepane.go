package sidepane

//go:generate go run github.com/go-pkgz/enum@v0.7.0 -type motion
//go:generate go run github.com/go-pkgz/enum@v0.7.0 -type direction

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/style"
)

// motion is the generator input for Motion (see generated motion_enum.go).
// never reference motion or its constants outside this file; use the generated
// exported Motion type and its constants instead.
type motion int

const (
	motionUnknown  motion = iota // zero value sentinel
	motionUp                     // single step up
	motionDown                   // single step down
	motionPageUp                 // page up — uses count[0] from variadic
	motionPageDown               // page down — uses count[0] from variadic
	motionFirst                  // jump to first entry
	motionLast                   // jump to last entry
)

// direction is the generator input for Direction (see generated direction_enum.go).
// never reference direction or its constants outside this file; use the generated
// exported Direction type and its constants instead.
type direction int

const (
	directionUnknown direction = iota // zero value sentinel
	directionNext                     // forward / next
	directionPrev                     // backward / previous
)

// Resolver is what sidepane needs for style lookups.
// satisfied by *style.Resolver via ui's styleResolver interface.
type Resolver interface {
	Style(k style.StyleKey) lipgloss.Style
	Color(k style.ColorKey) style.Color
}

// Renderer is what sidepane needs for compound ANSI rendering (FileTree only — TOC doesn't use it).
// satisfied by *style.Renderer via ui's styleRenderer interface.
type Renderer interface {
	FileStatusMark(status diff.FileStatus) string
	FileReviewedMark() string
	FileAnnotationMark() string
}

// FileTreeRender holds parameters for FileTree.Render.
type FileTreeRender struct {
	Width     int
	Height    int
	Annotated map[string]bool
	Resolver  Resolver
	Renderer  Renderer
}

// ScrollState reports the visible window state for a sidepane component.
type ScrollState struct {
	Total  int // total logical rows in the sidepane
	Offset int // first visible logical row
}

// TOCRender holds parameters for TOC.Render.
type TOCRender struct {
	Width    int
	Height   int
	Focused  bool
	Resolver Resolver
}

// ensureVisible adjusts offset so cursor is within the visible range of given height.
// shared by FileTree.EnsureVisible and TOC.EnsureVisible.
func ensureVisible(cursor, offset *int, count, height int) {
	if height <= 0 {
		return
	}
	switch {
	case *cursor < *offset:
		*offset = *cursor
	case *cursor >= *offset+height:
		*offset = *cursor - height + 1
	}
	if *offset < 0 {
		*offset = 0
	}
	if maxOff := max(count-height, 0); *offset > maxOff {
		*offset = maxOff
	}
}
