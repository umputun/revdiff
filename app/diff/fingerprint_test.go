package diff

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileFingerprintSemanticDiff(t *testing.T) {
	entry := FileEntry{Path: "main.go", Status: FileModified}
	base := []DiffLine{
		{OldNum: 1, NewNum: 1, Content: "before", ChangeType: ChangeContext},
		{OldNum: 2, Content: "old", ChangeType: ChangeRemove},
		{NewNum: 2, Content: "new", ChangeType: ChangeAdd},
		{OldNum: 3, NewNum: 3, Content: "after", ChangeType: ChangeContext},
	}

	t.Run("ignores line numbers context and dividers", func(t *testing.T) {
		shifted := []DiffLine{
			{ChangeType: ChangeDivider, Content: "different divider label"},
			{OldNum: 100, NewNum: 100, Content: "different context", ChangeType: ChangeContext},
			{OldNum: 101, Content: "old", ChangeType: ChangeRemove},
			{NewNum: 101, Content: "new", ChangeType: ChangeAdd},
		}
		assert.Equal(t, FileFingerprint(entry, base), FileFingerprint(entry, shifted))
	})

	t.Run("detects changed patch content", func(t *testing.T) {
		changed := append([]DiffLine(nil), base...)
		changed[2].Content = "newer"
		assert.NotEqual(t, FileFingerprint(entry, base), FileFingerprint(entry, changed))
	})

	t.Run("detects changed patch ordering", func(t *testing.T) {
		reordered := append([]DiffLine(nil), base...)
		reordered[1], reordered[2] = reordered[2], reordered[1]
		assert.NotEqual(t, FileFingerprint(entry, base), FileFingerprint(entry, reordered))
	})

	t.Run("detects metadata-only changes", func(t *testing.T) {
		rename := FileEntry{Path: "main.go", OldPath: "old.go", Status: FileRenamed}
		assert.NotEqual(t, FileFingerprint(entry, nil), FileFingerprint(rename, nil))
	})
}

func TestFileFingerprintContextOnlySource(t *testing.T) {
	entry := FileEntry{Path: "README.md"}
	first := make([]DiffLine, 1, 2)
	first[0] = DiffLine{NewNum: 1, Content: "one", ChangeType: ChangeContext}
	second := []DiffLine{{NewNum: 1, Content: "two", ChangeType: ChangeContext}}

	assert.NotEqual(t, FileFingerprint(entry, first), FileFingerprint(entry, second))
	assert.Equal(t, FileFingerprint(entry, first), FileFingerprint(entry, append(first, DiffLine{ChangeType: ChangeDivider, Content: "ignored"})))
}
