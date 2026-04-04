package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/diff"
)

func TestParseTOC(t *testing.T) {
	tests := []struct {
		name    string
		lines   []diff.DiffLine
		wantNil bool
		want    []tocEntry
	}{
		{name: "empty input", lines: nil, wantNil: true},
		{name: "no headers", lines: []diff.DiffLine{
			{Content: "some text", ChangeType: diff.ChangeContext},
			{Content: "more text", ChangeType: diff.ChangeContext},
		}, wantNil: true},
		{name: "single header", lines: []diff.DiffLine{
			{Content: "# Title", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{{title: "Title", level: 1, lineIdx: 0}}},
		{name: "nested headers h1-h6", lines: []diff.DiffLine{
			{Content: "# H1", ChangeType: diff.ChangeContext},
			{Content: "## H2", ChangeType: diff.ChangeContext},
			{Content: "### H3", ChangeType: diff.ChangeContext},
			{Content: "#### H4", ChangeType: diff.ChangeContext},
			{Content: "##### H5", ChangeType: diff.ChangeContext},
			{Content: "###### H6", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{
			{title: "H1", level: 1, lineIdx: 0},
			{title: "H2", level: 2, lineIdx: 1},
			{title: "H3", level: 3, lineIdx: 2},
			{title: "H4", level: 4, lineIdx: 3},
			{title: "H5", level: 5, lineIdx: 4},
			{title: "H6", level: 6, lineIdx: 5},
		}},
		{name: "headers mixed with non-header lines", lines: []diff.DiffLine{
			{Content: "intro text", ChangeType: diff.ChangeContext},
			{Content: "# First", ChangeType: diff.ChangeContext},
			{Content: "body text", ChangeType: diff.ChangeContext},
			{Content: "## Second", ChangeType: diff.ChangeContext},
			{Content: "more body", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{
			{title: "First", level: 1, lineIdx: 1},
			{title: "Second", level: 2, lineIdx: 3},
		}},
		{name: "headers inside fenced code block excluded", lines: []diff.DiffLine{
			{Content: "# Real Header", ChangeType: diff.ChangeContext},
			{Content: "```", ChangeType: diff.ChangeContext},
			{Content: "# Not A Header", ChangeType: diff.ChangeContext},
			{Content: "## Also Not", ChangeType: diff.ChangeContext},
			{Content: "```", ChangeType: diff.ChangeContext},
			{Content: "## After Code Block", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{
			{title: "Real Header", level: 1, lineIdx: 0},
			{title: "After Code Block", level: 2, lineIdx: 5},
		}},
		{name: "fenced code block with language", lines: []diff.DiffLine{
			{Content: "# Header", ChangeType: diff.ChangeContext},
			{Content: "```go", ChangeType: diff.ChangeContext},
			{Content: "# comment in code", ChangeType: diff.ChangeContext},
			{Content: "```", ChangeType: diff.ChangeContext},
		}, want: []tocEntry{{title: "Header", level: 1, lineIdx: 0}}},
		{name: "no space after hash is not a header", lines: []diff.DiffLine{
			{Content: "#nospace", ChangeType: diff.ChangeContext},
			{Content: "##also-no", ChangeType: diff.ChangeContext},
		}, wantNil: true},
		{name: "hash only without title", lines: []diff.DiffLine{
			{Content: "# ", ChangeType: diff.ChangeContext},
		}, wantNil: true},
		{name: "more than 6 hashes", lines: []diff.DiffLine{
			{Content: "####### Too Deep", ChangeType: diff.ChangeContext},
		}, wantNil: true},
		{name: "divider lines skipped", lines: []diff.DiffLine{
			{Content: "", ChangeType: diff.ChangeDivider},
			{Content: "# Title", ChangeType: diff.ChangeContext},
			{Content: "", ChangeType: diff.ChangeDivider},
		}, want: []tocEntry{{title: "Title", level: 1, lineIdx: 1}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTOC(tt.lines)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.want, got.entries)
			assert.Equal(t, 0, got.cursor)
			assert.Equal(t, 0, got.offset)
			assert.Equal(t, -1, got.activeSection)
		})
	}
}

func TestModel_IsFullContext(t *testing.T) {
	m := &Model{}
	tests := []struct {
		name  string
		lines []diff.DiffLine
		want  bool
	}{
		{name: "all context", lines: []diff.DiffLine{
			{ChangeType: diff.ChangeContext}, {ChangeType: diff.ChangeContext},
		}, want: true},
		{name: "context with dividers", lines: []diff.DiffLine{
			{ChangeType: diff.ChangeContext}, {ChangeType: diff.ChangeDivider}, {ChangeType: diff.ChangeContext},
		}, want: true},
		{name: "mixed with add", lines: []diff.DiffLine{
			{ChangeType: diff.ChangeContext}, {ChangeType: diff.ChangeAdd},
		}, want: false},
		{name: "mixed with remove", lines: []diff.DiffLine{
			{ChangeType: diff.ChangeContext}, {ChangeType: diff.ChangeRemove},
		}, want: false},
		{name: "empty", lines: nil, want: false},
		{name: "divider only", lines: []diff.DiffLine{
			{ChangeType: diff.ChangeDivider},
		}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, m.isFullContext(tt.lines))
		})
	}
}

func TestModel_IsMarkdownFile(t *testing.T) {
	m := &Model{}
	tests := []struct {
		name string
		file string
		want bool
	}{
		{name: ".md", file: "README.md", want: true},
		{name: ".markdown", file: "notes.markdown", want: true},
		{name: ".go", file: "main.go", want: false},
		{name: ".MD uppercase", file: "README.MD", want: true},
		{name: ".MARKDOWN uppercase", file: "DOC.MARKDOWN", want: true},
		{name: "no extension", file: "Makefile", want: false},
		{name: "path with .md", file: "docs/guide.md", want: true},
		{name: "empty", file: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, m.isMarkdownFile(tt.file))
		})
	}
}
