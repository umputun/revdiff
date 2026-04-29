package style

//go:generate go run github.com/go-pkgz/enum@v0.7.0 -type colorKey
//go:generate go run github.com/go-pkgz/enum@v0.7.0 -type styleKey

// colorKey is the generator input for ColorKey (see generated color_key_enum.go).
// names intentionally diverge from Colors struct field names — enum describes what the
// color is for (DiffPaneBg, AddLineBg), while Colors fields describe what they contain
// (DiffBg, AddBg). the Color(k) switch handles the mapping internally.
// never reference colorKey or its constants outside this file; use the generated
// exported ColorKey type and its constants instead.
type colorKey int

const (
	colorKeyUnknown colorKey = iota // zero value sentinel
	colorKeyAccentFg
	colorKeyMutedFg
	colorKeyAnnotationFg
	colorKeyStatusFg // internal fallback: StatusFg || MutedFg
	colorKeyTreePaneBg
	colorKeyDiffPaneBg
	colorKeyAddLineBg
	colorKeyRemoveLineBg
	colorKeyModifyLineBg
	colorKeyWordAddBg
	colorKeyWordRemoveBg
	colorKeySearchBg
	colorKeyAddLineFg
	colorKeyRemoveLineFg
	colorKeyModifyLineFg
	colorKeySearchFg
	colorKeyNormalFg
	colorKeySelectedFg
)

// styleKey is the generator input for StyleKey (see generated style_key_enum.go).
// never reference styleKey or its constants outside this file; use the generated
// exported StyleKey type and its constants instead.
type styleKey int

const (
	styleKeyUnknown styleKey = iota

	// diff line styles
	styleKeyLineAdd
	styleKeyLineRemove
	styleKeyLineContext
	styleKeyLineModify
	styleKeyLineAddHighlight
	styleKeyLineRemoveHighlight
	styleKeyLineContextHighlight
	styleKeyLineModifyHighlight
	styleKeyLineNumber

	// pane styles
	styleKeyTreePane
	styleKeyTreePaneActive
	styleKeyDiffPane
	styleKeyDiffPaneActive

	// file tree entry styles
	styleKeyDirEntry
	styleKeyFileEntry
	styleKeyFileSelected
	styleKeyAnnotationMark
	styleKeyReviewedMark

	// file status styles
	styleKeyStatusAdded
	styleKeyStatusDeleted
	styleKeyStatusUntracked
	styleKeyStatusDefault

	// chrome styles
	styleKeyStatusBar
	styleKeySearchMatch

	// overlay / popup / input styles — per D14, all inline lipgloss construction moves here
	styleKeyAnnotInputText        // ti.TextStyle / ti.PromptStyle in newAnnotationInput
	styleKeyAnnotInputPlaceholder // ti.PlaceholderStyle
	styleKeyAnnotInputCursor      // ti.Cursor.Style / ti.Cursor.TextStyle
	styleKeyAnnotListBorder       // annotation list popup border
	styleKeyHelpBox               // help overlay box
	styleKeyThemeSelectBox        // theme selector box
	styleKeyThemeSelectBoxFocused // theme selector focused state
	styleKeyInfoBox               // info overlay box (description + session metadata + commits)
)
