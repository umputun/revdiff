package annotation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_Add(t *testing.T) {
	s := NewStore()
	s.Add("handler.go", 43, "+", "use errors.Is()")

	anns := s.Get("handler.go")
	require.Len(t, anns, 1)
	assert.Equal(t, Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "use errors.Is()"}, anns[0])
}

func TestStore_AddReplacesExisting(t *testing.T) {
	s := NewStore()
	s.Add("handler.go", 43, "+", "old comment")
	s.Add("handler.go", 43, "+", "new comment")

	anns := s.Get("handler.go")
	require.Len(t, anns, 1)
	assert.Equal(t, "new comment", anns[0].Comment)
}

func TestStore_AddMultipleLines(t *testing.T) {
	s := NewStore()
	s.Add("handler.go", 43, "+", "first")
	s.Add("handler.go", 10, "-", "second")

	anns := s.Get("handler.go")
	require.Len(t, anns, 2)
	// sorted by line number
	assert.Equal(t, 10, anns[0].Line)
	assert.Equal(t, 43, anns[1].Line)
}

func TestStore_AddMultipleFiles(t *testing.T) {
	s := NewStore()
	s.Add("handler.go", 43, "+", "first")
	s.Add("store.go", 18, "-", "second")

	assert.Len(t, s.Get("handler.go"), 1)
	assert.Len(t, s.Get("store.go"), 1)
}

func TestStore_Delete(t *testing.T) {
	s := NewStore()
	s.Add("handler.go", 43, "+", "comment")

	ok := s.Delete("handler.go", 43, "+")
	assert.True(t, ok)
	assert.Empty(t, s.Get("handler.go"))
}

func TestStore_DeleteCleansUpEmptyFile(t *testing.T) {
	s := NewStore()
	s.Add("handler.go", 43, "+", "comment")
	s.Delete("handler.go", 43, "+")

	all := s.All()
	_, exists := all["handler.go"]
	assert.False(t, exists, "file entry should be removed when last annotation deleted")
}

func TestStore_DeleteNonExistent(t *testing.T) {
	s := NewStore()
	ok := s.Delete("handler.go", 43, "+")
	assert.False(t, ok)
}

func TestStore_DeletePreservesOthers(t *testing.T) {
	s := NewStore()
	s.Add("handler.go", 10, "+", "keep")
	s.Add("handler.go", 43, "-", "remove")

	s.Delete("handler.go", 43, "-")
	anns := s.Get("handler.go")
	require.Len(t, anns, 1)
	assert.Equal(t, 10, anns[0].Line)
}

func TestStore_DeleteMatchesByType(t *testing.T) {
	s := NewStore()
	s.Add("handler.go", 5, "+", "added line")
	s.Add("handler.go", 5, "-", "removed line")

	// deleting the add should leave the remove
	ok := s.Delete("handler.go", 5, "+")
	assert.True(t, ok)
	anns := s.Get("handler.go")
	require.Len(t, anns, 1)
	assert.Equal(t, "-", anns[0].Type)
	assert.Equal(t, "removed line", anns[0].Comment)
}

func TestStore_AddSameLineDifferentTypes(t *testing.T) {
	s := NewStore()
	s.Add("handler.go", 5, "+", "added line")
	s.Add("handler.go", 5, "-", "removed line")

	anns := s.Get("handler.go")
	require.Len(t, anns, 2, "same line number with different types should be separate annotations")
}

func TestStore_GetEmpty(t *testing.T) {
	s := NewStore()
	anns := s.Get("nonexistent.go")
	assert.Empty(t, anns)
}

func TestStore_GetReturnsCopy(t *testing.T) {
	s := NewStore()
	s.Add("handler.go", 43, "+", "comment")

	anns := s.Get("handler.go")
	anns[0].Comment = "modified"

	original := s.Get("handler.go")
	assert.Equal(t, "comment", original[0].Comment, "Get should return a copy")
}

func TestStore_All(t *testing.T) {
	s := NewStore()
	s.Add("handler.go", 43, "+", "first")
	s.Add("store.go", 18, "-", "second")

	all := s.All()
	assert.Len(t, all, 2)
	assert.Len(t, all["handler.go"], 1)
	assert.Len(t, all["store.go"], 1)
}

func TestStore_AllEmpty(t *testing.T) {
	s := NewStore()
	all := s.All()
	assert.Empty(t, all)
}

func TestStore_AllReturnsCopy(t *testing.T) {
	s := NewStore()
	s.Add("handler.go", 43, "+", "comment")

	all := s.All()
	all["handler.go"][0].Comment = "modified"

	original := s.All()
	assert.Equal(t, "comment", original["handler.go"][0].Comment, "All should return a copy")
}

func TestStore_Files(t *testing.T) {
	s := NewStore()
	assert.Empty(t, s.Files())

	s.Add("z_file.go", 1, "+", "comment")
	s.Add("a_file.go", 2, "-", "comment")
	files := s.Files()
	require.Len(t, files, 2)
	assert.Equal(t, "a_file.go", files[0])
	assert.Equal(t, "z_file.go", files[1])
}

func TestStore_FormatOutputEmpty(t *testing.T) {
	s := NewStore()
	assert.Empty(t, s.FormatOutput())
}

func TestStore_FormatOutputSingle(t *testing.T) {
	s := NewStore()
	s.Add("handler.go", 43, "+", "use errors.Is() instead of direct comparison")

	expected := "## handler.go:43 (+)\nuse errors.Is() instead of direct comparison\n"
	assert.Equal(t, expected, s.FormatOutput())
}

func TestStore_FormatOutputMultiFile(t *testing.T) {
	s := NewStore()
	s.Add("handler.go", 43, "+", "use errors.Is() instead of direct comparison")
	s.Add("store.go", 18, "-", "don't remove this validation")

	expected := "## handler.go:43 (+)\n" +
		"use errors.Is() instead of direct comparison\n" +
		"\n" +
		"## store.go:18 (-)\n" +
		"don't remove this validation\n"
	assert.Equal(t, expected, s.FormatOutput())
}

func TestStore_FormatOutputMultiLinesSameFile(t *testing.T) {
	s := NewStore()
	s.Add("handler.go", 43, "+", "fix this")
	s.Add("handler.go", 10, " ", "add docs")

	expected := "## handler.go:10 ( )\n" +
		"add docs\n" +
		"\n" +
		"## handler.go:43 (+)\n" +
		"fix this\n"
	assert.Equal(t, expected, s.FormatOutput())
}

func TestStore_FormatOutputSortedByFilename(t *testing.T) {
	s := NewStore()
	s.Add("z_file.go", 1, "+", "last")
	s.Add("a_file.go", 1, "-", "first")

	out := s.FormatOutput()
	aIdx := strings.Index(out, "## a_file.go")
	zIdx := strings.Index(out, "## z_file.go")
	require.GreaterOrEqual(t, aIdx, 0, "a_file should be in output")
	require.GreaterOrEqual(t, zIdx, 0, "z_file should be in output")
	assert.Less(t, aIdx, zIdx, "a_file should come before z_file")
	assert.Contains(t, out, "## a_file.go:1 (-)\nfirst\n")
	assert.Contains(t, out, "## z_file.go:1 (+)\nlast\n")
}
