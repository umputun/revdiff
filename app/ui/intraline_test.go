package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
)

func TestChangedTokenRanges_IdentifierRename(t *testing.T) {
	minus := "func oldName() {"
	plus := "func newName() {"

	minusRanges, plusRanges := changedTokenRanges(minus, plus)
	require.Len(t, minusRanges, 1)
	require.Len(t, plusRanges, 1)

	assert.Equal(t, "oldName", minus[minusRanges[0].start:minusRanges[0].end])
	assert.Equal(t, "newName", plus[plusRanges[0].start:plusRanges[0].end])
}

func TestComputeIntralineRanges_PairsSubhunkLines(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 10, Content: "count := oldValue + 1", ChangeType: diff.ChangeRemove},
		{OldNum: 11, Content: "return count", ChangeType: diff.ChangeRemove},
		{NewNum: 10, Content: "count := newValue + 1", ChangeType: diff.ChangeAdd},
		{NewNum: 11, Content: "return count", ChangeType: diff.ChangeAdd},
	}

	ranges := computeIntralineRanges(lines, "    ")
	require.NotNil(t, ranges)

	require.Contains(t, ranges, 0)
	require.Contains(t, ranges, 2)
	assert.NotContains(t, ranges, 1, "unchanged paired line should not receive emphasis ranges")
	assert.NotContains(t, ranges, 3, "unchanged paired line should not receive emphasis ranges")

	mr := ranges[0][0]
	pr := ranges[2][0]
	assert.Equal(t, "oldValue", lines[0].Content[mr.start:mr.end])
	assert.Equal(t, "newValue", lines[2].Content[pr.start:pr.end])
}

func TestModel_RenderDiffAddsIntralineEmphasis(t *testing.T) {
	m := testModel(nil, nil)
	m.diffLines = []diff.DiffLine{
		{OldNum: 1, Content: "value := oldValue", ChangeType: diff.ChangeRemove},
		{NewNum: 1, Content: "value := newValue", ChangeType: diff.ChangeAdd},
	}
	m.intralineRanges = computeIntralineRanges(m.diffLines, m.tabSpaces)

	rendered := m.renderDiff()
	assert.Contains(t, rendered, "\033[1m", "intraline emphasis should add bold marker")
	assert.Contains(t, rendered, "oldValue")
	assert.Contains(t, rendered, "newValue")
}
