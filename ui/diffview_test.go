package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"

	"github.com/umputun/revdiff/diff"
)

func TestModel_LineNumGutter(t *testing.T) {
	m := testModel(nil, nil)
	m.lineNumbers = true
	m.lineNumWidth = 3

	tests := []struct {
		name string
		dl   diff.DiffLine
		want string // plain text content (ANSI stripped)
	}{
		{
			name: "context line",
			dl:   diff.DiffLine{OldNum: 25, NewNum: 32, ChangeType: diff.ChangeContext},
			want: "  25  32", // " " + " 25" + " " + " 32"
		},
		{
			name: "add line",
			dl:   diff.DiffLine{OldNum: 0, NewNum: 40, ChangeType: diff.ChangeAdd},
			want: "      40", // " " + "   " + " " + " 40"
		},
		{
			name: "remove line",
			dl:   diff.DiffLine{OldNum: 40, NewNum: 0, ChangeType: diff.ChangeRemove},
			want: "  40    ", // " " + " 40" + " " + "   "
		},
		{
			name: "divider",
			dl:   diff.DiffLine{ChangeType: diff.ChangeDivider},
			want: "        ", // " " + "   " + " " + "   "
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.lineNumGutter(tt.dl)
			stripped := ansi.Strip(got)
			assert.Equal(t, tt.want, stripped)
		})
	}
}

func TestModel_LineNumGutterWidth(t *testing.T) {
	m := testModel(nil, nil)
	m.lineNumWidth = 3
	// width = 1 (leading space) + 3 (old) + 1 (space) + 3 (new) = 8
	assert.Equal(t, 8, m.lineNumGutterWidth())

	m.lineNumWidth = 1
	// width = 1 + 1 + 1 + 1 = 4
	assert.Equal(t, 4, m.lineNumGutterWidth())
}
