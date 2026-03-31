package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/diff"
)

// startAnnotation enters annotation input mode for the current cursor line.
func (m *Model) startAnnotation() tea.Cmd {
	dl, ok := m.cursorDiffLine()
	if !ok || dl.ChangeType == diff.ChangeDivider {
		return nil
	}

	ti := textinput.New()
	ti.Placeholder = "annotation..."
	cmd := ti.Focus()
	ti.CharLimit = 500
	ti.Width = m.width - m.treeWidth - 10

	// pre-fill with existing annotation if one exists
	lineNum := m.diffLineNum(dl)
	for _, a := range m.store.Get(m.currFile) {
		if a.Line == lineNum && a.Type == dl.ChangeType {
			ti.SetValue(a.Comment)
			break
		}
	}

	m.annotateInput = ti
	m.annotating = true
	return cmd
}

// saveAnnotation saves the current text input as an annotation on the cursor line.
func (m *Model) saveAnnotation() {
	text := m.annotateInput.Value()
	if text == "" {
		m.cancelAnnotation()
		return
	}

	dl, ok := m.cursorDiffLine()
	if !ok {
		m.cancelAnnotation()
		return
	}

	lineNum := m.diffLineNum(dl)
	m.store.Add(m.currFile, lineNum, dl.ChangeType, text)
	m.annotating = false
	m.tree.refreshFilter(m.annotatedFiles())
	m.viewport.SetContent(m.renderDiff())
}

// cancelAnnotation exits annotation input mode without saving.
func (m *Model) cancelAnnotation() {
	m.annotating = false
	m.viewport.SetContent(m.renderDiff())
}

// deleteAnnotation removes the annotation on the current cursor line if one exists.
// returns a command to load the new file if the tree selection changed after filter refresh.
func (m *Model) deleteAnnotation() tea.Cmd {
	dl, ok := m.cursorDiffLine()
	if !ok || dl.ChangeType == diff.ChangeDivider {
		return nil
	}

	lineNum := m.diffLineNum(dl)
	if m.store.Delete(m.currFile, lineNum, dl.ChangeType) {
		m.tree.refreshFilter(m.annotatedFiles())

		// if filter moved cursor to a different file, load the new selection
		if newFile := m.tree.selectedFile(); newFile != "" && newFile != m.currFile {
			m.loadSeq++
			m.pendingFile = newFile
			return m.loadFileDiff(newFile)
		}

		m.viewport.SetContent(m.renderDiff())
	}
	return nil
}

// handleAnnotateKey handles key messages during annotation input mode.
func (m Model) handleAnnotateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.saveAnnotation()
		return m, nil
	case tea.KeyEsc:
		m.cancelAnnotation()
		return m, nil
	default:
		var cmd tea.Cmd
		m.annotateInput, cmd = m.annotateInput.Update(msg)
		m.viewport.SetContent(m.renderDiff()) // re-render so typed characters are visible immediately
		return m, cmd
	}
}

// diffLineNum returns the display line number for a diff line.
func (m Model) diffLineNum(dl diff.DiffLine) int {
	if dl.ChangeType == diff.ChangeRemove {
		return dl.OldNum
	}
	return dl.NewNum
}

// cursorViewportY computes the actual viewport Y position of the cursor,
// accounting for injected annotation lines.
func (m Model) cursorViewportY() int {
	if m.currFile == "" || len(m.diffLines) == 0 {
		return m.diffCursor
	}

	annotationSet := m.buildAnnotationSet()
	y := 0
	for i := 0; i < m.diffCursor && i < len(m.diffLines); i++ {
		y++ // the diff line itself
		dl := m.diffLines[i]
		if dl.ChangeType != diff.ChangeDivider {
			key := m.annotationKey(m.diffLineNum(dl), dl.ChangeType)
			if annotationSet[key] {
				y++ // the annotation line below it
			}
		}
	}
	return y
}

// buildAnnotationSet returns a set of annotation keys for the current file.
func (m Model) buildAnnotationSet() map[string]bool {
	annotations := m.store.Get(m.currFile)
	set := make(map[string]bool, len(annotations))
	for _, a := range annotations {
		set[m.annotationKey(a.Line, a.Type)] = true
	}
	return set
}

// annotationKey creates a lookup key from line number and change type.
func (m Model) annotationKey(line int, changeType string) string {
	return fmt.Sprintf("%d:%s", line, changeType)
}

// renderAnnotationSummary renders a summary of all annotations for the left pane.
func (m Model) renderAnnotationSummary(width int) string {
	all := m.store.All()
	if len(all) == 0 {
		return ""
	}

	files := make([]string, 0, len(all))
	for f := range all {
		files = append(files, f)
	}
	sort.Strings(files)

	var b strings.Builder
	b.WriteString(m.styles.DirEntry.Render("─ annotations ─") + "\n")

	for _, file := range files {
		for _, a := range all[file] {
			loc := fmt.Sprintf(" %s:%d", filepath.Base(file), a.Line)
			b.WriteString(m.styles.FileEntry.Render(loc) + "\n")
			comment := a.Comment
			maxLen := width - 4
			if maxLen > 3 && len(comment) > maxLen {
				comment = comment[:maxLen-3] + "..."
			}
			b.WriteString(m.styles.AnnotationLine.Render(" \""+comment+"\"") + "\n")
		}
	}

	return b.String()
}
