package annotation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_Add(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "use errors.Is()"})

	anns := s.Get("handler.go")
	require.Len(t, anns, 1)
	assert.Equal(t, Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "use errors.Is()"}, anns[0])
}

func TestStore_AddReplacesExisting(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "old comment"})
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "new comment"})

	anns := s.Get("handler.go")
	require.Len(t, anns, 1)
	assert.Equal(t, "new comment", anns[0].Comment)
}

func TestStore_AddReplacesExistingPreservesEndLine(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "old comment", EndLine: 0})
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "refactor this hunk", EndLine: 67})

	anns := s.Get("handler.go")
	require.Len(t, anns, 1)
	assert.Equal(t, "refactor this hunk", anns[0].Comment)
	assert.Equal(t, 67, anns[0].EndLine, "EndLine should be updated on replacement")
}

func TestStore_AddReplacesExistingClearsEndLine(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "refactor this hunk", EndLine: 67})
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "just a note", EndLine: 0})

	anns := s.Get("handler.go")
	require.Len(t, anns, 1)
	assert.Equal(t, "just a note", anns[0].Comment)
	assert.Equal(t, 0, anns[0].EndLine, "EndLine should be cleared when replacement has no range")
}

func TestStore_AddMultipleLines(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "first"})
	s.Add(Annotation{File: "handler.go", Line: 10, Type: "-", Comment: "second"})

	anns := s.Get("handler.go")
	require.Len(t, anns, 2)
	// sorted by line number
	assert.Equal(t, 10, anns[0].Line)
	assert.Equal(t, 43, anns[1].Line)
}

func TestStore_AddMultipleFiles(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "first"})
	s.Add(Annotation{File: "store.go", Line: 18, Type: "-", Comment: "second"})

	assert.Len(t, s.Get("handler.go"), 1)
	assert.Len(t, s.Get("store.go"), 1)
}

func TestStore_Delete(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "comment"})

	ok := s.Delete("handler.go", 43, "+")
	assert.True(t, ok)
	assert.Empty(t, s.Get("handler.go"))
}

func TestStore_DeleteCleansUpEmptyFile(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "comment"})
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
	s.Add(Annotation{File: "handler.go", Line: 10, Type: "+", Comment: "keep"})
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "-", Comment: "remove"})

	s.Delete("handler.go", 43, "-")
	anns := s.Get("handler.go")
	require.Len(t, anns, 1)
	assert.Equal(t, 10, anns[0].Line)
}

func TestStore_DeleteMatchesByType(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 5, Type: "+", Comment: "added line"})
	s.Add(Annotation{File: "handler.go", Line: 5, Type: "-", Comment: "removed line"})

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
	s.Add(Annotation{File: "handler.go", Line: 5, Type: "+", Comment: "added line"})
	s.Add(Annotation{File: "handler.go", Line: 5, Type: "-", Comment: "removed line"})

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
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "comment"})

	anns := s.Get("handler.go")
	anns[0].Comment = "modified"

	original := s.Get("handler.go")
	assert.Equal(t, "comment", original[0].Comment, "Get should return a copy")
}

func TestStore_All(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "first"})
	s.Add(Annotation{File: "store.go", Line: 18, Type: "-", Comment: "second"})

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
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "comment"})

	all := s.All()
	all["handler.go"][0].Comment = "modified"

	original := s.All()
	assert.Equal(t, "comment", original["handler.go"][0].Comment, "All should return a copy")
}

func TestStore_Files(t *testing.T) {
	s := NewStore()
	assert.Empty(t, s.Files())

	s.Add(Annotation{File: "z_file.go", Line: 1, Type: "+", Comment: "comment"})
	s.Add(Annotation{File: "a_file.go", Line: 2, Type: "-", Comment: "comment"})
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
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "use errors.Is() instead of direct comparison"})

	expected := "## handler.go:43 (+)\nuse errors.Is() instead of direct comparison\n"
	assert.Equal(t, expected, s.FormatOutput())
}

func TestStore_FormatOutputMultiFile(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "use errors.Is() instead of direct comparison"})
	s.Add(Annotation{File: "store.go", Line: 18, Type: "-", Comment: "don't remove this validation"})

	expected := "## handler.go:43 (+)\n" +
		"use errors.Is() instead of direct comparison\n" +
		"\n" +
		"## store.go:18 (-)\n" +
		"don't remove this validation\n"
	assert.Equal(t, expected, s.FormatOutput())
}

func TestStore_FormatOutputMultiLinesSameFile(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "fix this"})
	s.Add(Annotation{File: "handler.go", Line: 10, Type: " ", Comment: "add docs"})

	expected := "## handler.go:10 ( )\n" +
		"add docs\n" +
		"\n" +
		"## handler.go:43 (+)\n" +
		"fix this\n"
	assert.Equal(t, expected, s.FormatOutput())
}

func TestStore_AddFileLevelAnnotation(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 0, Type: "", Comment: "this file needs refactoring"})

	anns := s.Get("handler.go")
	require.Len(t, anns, 1)
	assert.Equal(t, Annotation{File: "handler.go", Line: 0, Type: "", Comment: "this file needs refactoring"}, anns[0])
}

func TestStore_AddFileLevelReplacesExisting(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 0, Type: "", Comment: "old comment"})
	s.Add(Annotation{File: "handler.go", Line: 0, Type: "", Comment: "new comment"})

	anns := s.Get("handler.go")
	require.Len(t, anns, 1)
	assert.Equal(t, "new comment", anns[0].Comment)
}

func TestStore_DeleteFileLevelAnnotation(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 0, Type: "", Comment: "file comment"})

	ok := s.Delete("handler.go", 0, "")
	assert.True(t, ok)
	assert.Empty(t, s.Get("handler.go"))
}

func TestStore_DeleteFileLevelPreservesLineAnnotations(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 0, Type: "", Comment: "file comment"})
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "line comment"})

	s.Delete("handler.go", 0, "")
	anns := s.Get("handler.go")
	require.Len(t, anns, 1)
	assert.Equal(t, 43, anns[0].Line)
}

func TestStore_GetFileLevelWithLineAnnotations(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "line comment"})
	s.Add(Annotation{File: "handler.go", Line: 0, Type: "", Comment: "file comment"})
	s.Add(Annotation{File: "handler.go", Line: 10, Type: "-", Comment: "another line"})

	anns := s.Get("handler.go")
	require.Len(t, anns, 3)
	// sorted by line: file-level (0) first, then 10, then 43
	assert.Equal(t, 0, anns[0].Line)
	assert.Equal(t, 10, anns[1].Line)
	assert.Equal(t, 43, anns[2].Line)
}

func TestStore_FormatOutputFileLevelOnly(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 0, Type: "", Comment: "this file needs refactoring"})

	expected := "## handler.go (file-level)\nthis file needs refactoring\n"
	assert.Equal(t, expected, s.FormatOutput())
}

func TestStore_FormatOutputFileLevelBeforeLineAnnotations(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "fix this"})
	s.Add(Annotation{File: "handler.go", Line: 0, Type: "", Comment: "file needs refactoring"})

	expected := "## handler.go (file-level)\nfile needs refactoring\n" +
		"\n" +
		"## handler.go:43 (+)\nfix this\n"
	assert.Equal(t, expected, s.FormatOutput())
}

func TestStore_FormatOutputMixedFileLevelAndLineMultiFile(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "a_file.go", Line: 0, Type: "", Comment: "file-level note for a"})
	s.Add(Annotation{File: "a_file.go", Line: 10, Type: "+", Comment: "line note for a"})
	s.Add(Annotation{File: "b_file.go", Line: 5, Type: "-", Comment: "line note for b"})
	s.Add(Annotation{File: "b_file.go", Line: 0, Type: "", Comment: "file-level note for b"})

	expected := "## a_file.go (file-level)\nfile-level note for a\n" +
		"\n" +
		"## a_file.go:10 (+)\nline note for a\n" +
		"\n" +
		"## b_file.go (file-level)\nfile-level note for b\n" +
		"\n" +
		"## b_file.go:5 (-)\nline note for b\n"
	assert.Equal(t, expected, s.FormatOutput())
}

func TestStore_FormatOutputSortedByFilename(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "z_file.go", Line: 1, Type: "+", Comment: "last"})
	s.Add(Annotation{File: "a_file.go", Line: 1, Type: "-", Comment: "first"})

	out := s.FormatOutput()
	aIdx := strings.Index(out, "## a_file.go")
	zIdx := strings.Index(out, "## z_file.go")
	require.GreaterOrEqual(t, aIdx, 0, "a_file should be in output")
	require.GreaterOrEqual(t, zIdx, 0, "z_file should be in output")
	assert.Less(t, aIdx, zIdx, "a_file should come before z_file")
	assert.Contains(t, out, "## a_file.go:1 (-)\nfirst\n")
	assert.Contains(t, out, "## z_file.go:1 (+)\nlast\n")
}

// verifies that annotations work correctly on context-only lines (type " "),
// which is the type used when viewing files without git changes.
func TestStore_ContextOnlyAnnotations(t *testing.T) {
	s := NewStore()

	// add annotations on context lines (type " ")
	s.Add(Annotation{File: "plan.md", Line: 3, Type: " ", Comment: "clarify this step"})
	s.Add(Annotation{File: "plan.md", Line: 7, Type: " ", Comment: "add more detail"})

	anns := s.Get("plan.md")
	require.Len(t, anns, 2)
	assert.Equal(t, 3, anns[0].Line)
	assert.Equal(t, " ", anns[0].Type)
	assert.Equal(t, "clarify this step", anns[0].Comment)
	assert.Equal(t, 7, anns[1].Line)

	// verify output format
	out := s.FormatOutput()
	assert.Contains(t, out, "## plan.md:3 ( )")
	assert.Contains(t, out, "clarify this step")
	assert.Contains(t, out, "## plan.md:7 ( )")
	assert.Contains(t, out, "add more detail")

	// verify Has works
	assert.True(t, s.Has("plan.md", 3, " "))
	assert.False(t, s.Has("plan.md", 3, "+"))

	// verify Delete works
	ok := s.Delete("plan.md", 3, " ")
	assert.True(t, ok)
	assert.Len(t, s.Get("plan.md"), 1)
}

func TestStore_FormatOutputEndLineZero(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "fix this"})

	expected := "## handler.go:43 (+)\nfix this\n"
	assert.Equal(t, expected, s.FormatOutput(), "EndLine=0 should produce single line number")
}

func TestStore_FormatOutputEndLineRange(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 43, EndLine: 67, Type: "+", Comment: "refactor this hunk"})

	expected := "## handler.go:43-67 (+)\nrefactor this hunk\n"
	assert.Equal(t, expected, s.FormatOutput(), "EndLine>0 should produce line range")
}

func TestStore_FormatOutputFileLevelIgnoresEndLine(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "handler.go", Line: 0, EndLine: 50, Type: "", Comment: "file comment"})

	expected := "## handler.go (file-level)\nfile comment\n"
	assert.Equal(t, expected, s.FormatOutput(), "file-level annotations should ignore EndLine")
}

func TestStore_Count(t *testing.T) {
	s := NewStore()
	assert.Equal(t, 0, s.Count(), "empty store should return 0")

	s.Add(Annotation{File: "a.go", Line: 1, Type: "+", Comment: "one"})
	assert.Equal(t, 1, s.Count())

	s.Add(Annotation{File: "a.go", Line: 5, Type: "-", Comment: "two"})
	assert.Equal(t, 2, s.Count())

	s.Add(Annotation{File: "b.go", Line: 1, Type: "+", Comment: "three"})
	assert.Equal(t, 3, s.Count(), "should count across multiple files")

	s.Delete("a.go", 1, "+")
	assert.Equal(t, 2, s.Count(), "count should decrease after delete")
}
