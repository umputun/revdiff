package diff

import (
	"errors"
	"strings"
	"testing"
)

func TestMultiFileStdinReader_TwoFiles(t *testing.T) {
	content := `diff --git a/file1.go b/file1.go
index abc..def
--- a/file1.go
+++ b/file1.go
@@ -1,1 +1,2 @@
 line1
+line2

diff --git a/file2.go b/file2.go
index ghi..jkl
--- a/file2.go
+++ b/file2.go
@@ -1,1 +1,1 @@
-old
+new
`

	r, err := NewMultiFileStdinReader(content)
	if err != nil {
		t.Fatalf("NewMultiFileStdinReader() error = %v", err)
	}

	// test ChangedFiles
	files, err := r.ChangedFiles("", false)
	if err != nil {
		t.Fatalf("ChangedFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Errorf("ChangedFiles() returned %d files, want 2", len(files))
	}
	if files[0].Path != "file1.go" {
		t.Errorf("files[0].Path = %q, want %q", files[0].Path, "file1.go")
	}
	if files[1].Path != "file2.go" {
		t.Errorf("files[1].Path = %q, want %q", files[1].Path, "file2.go")
	}

	// test FileDiff for file1
	lines1, err := r.FileDiff("", "file1.go", false, 0)
	if err != nil {
		t.Fatalf("FileDiff(file1.go) error = %v", err)
	}
	if len(lines1) == 0 {
		t.Error("FileDiff(file1.go) returned no lines")
	}

	// test FileDiff for file2
	lines2, err := r.FileDiff("", "file2.go", false, 0)
	if err != nil {
		t.Fatalf("FileDiff(file2.go) error = %v", err)
	}
	if len(lines2) == 0 {
		t.Error("FileDiff(file2.go) returned no lines")
	}

	// test FileDiff for non-existent file
	linesNone, err := r.FileDiff("", "nonexistent.go", false, 0)
	if err != nil {
		t.Fatalf("FileDiff(nonexistent.go) error = %v", err)
	}
	if linesNone != nil {
		t.Error("FileDiff(nonexistent.go) should return nil")
	}
}

func TestMultiFileStdinReader_BinaryFile(t *testing.T) {
	content := `diff --git a/text.go b/text.go
index abc..def
--- a/text.go
+++ b/text.go
@@ -1,1 +1,1 @@
-old
+new

diff --git a/image.png b/image.png
new file mode 100644
index 0000000..mno7890
Binary files /dev/null and b/image.png differ
`

	r, err := NewMultiFileStdinReader(content)
	if err != nil {
		t.Fatalf("NewMultiFileStdinReader() error = %v", err)
	}

	files, err := r.ChangedFiles("", false)
	if err != nil {
		t.Fatalf("ChangedFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Errorf("ChangedFiles() returned %d files, want 2", len(files))
	}

	// check that binary file was parsed
	lines, err := r.FileDiff("", "image.png", false, 0)
	if err != nil {
		t.Fatalf("FileDiff(image.png) error = %v", err)
	}
	if len(lines) == 0 {
		t.Error("FileDiff(image.png) returned no lines")
	}
	if len(lines) > 0 && !lines[0].IsBinary {
		t.Error("FileDiff(image.png) should have IsBinary=true")
	}
}

func TestMultiFileStdinReader_NewDeletedFiles(t *testing.T) {
	content := `diff --git a/new.go b/new.go
new file mode 100644
index 0000000..abc1234
--- /dev/null
+++ b/new.go
@@ -0,0 +1,1 @@
+new content

diff --git a/deleted.go b/deleted.go
deleted file mode 100644
index jkl..0000000
--- a/deleted.go
+++ /dev/null
@@ -1,1 +0,0 @@
-deleted content
`

	r, err := NewMultiFileStdinReader(content)
	if err != nil {
		t.Fatalf("NewMultiFileStdinReader() error = %v", err)
	}

	files, err := r.ChangedFiles("", false)
	if err != nil {
		t.Fatalf("ChangedFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Errorf("ChangedFiles() returned %d files, want 2", len(files))
	}

	// check status
	if files[0].Status != FileAdded {
		t.Errorf("files[0].Status = %v, want FileAdded", files[0].Status)
	}
	if files[1].Status != FileDeleted {
		t.Errorf("files[1].Status = %v, want FileDeleted", files[1].Status)
	}
}

func TestMultiFileStdinReader_RenamedFile(t *testing.T) {
	content := `diff --git a/old.go b/new.go
similarity index 100%
rename from old.go
rename to new.go
`

	r, err := NewMultiFileStdinReader(content)
	if err != nil {
		t.Fatalf("NewMultiFileStdinReader() error = %v", err)
	}

	files, err := r.ChangedFiles("", false)
	if err != nil {
		t.Fatalf("ChangedFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Errorf("ChangedFiles() returned %d files, want 1", len(files))
	}

	// check that renamed file uses new name
	if files[0].Path != "new.go" {
		t.Errorf("files[0].Path = %q, want %q", files[0].Path, "new.go")
	}
	if files[0].Status != FileRenamed {
		t.Errorf("files[0].Status = %v, want FileRenamed", files[0].Status)
	}
}

func TestMultiFileStdinReader_EmptyInput(t *testing.T) {
	_, err := NewMultiFileStdinReader("")
	if !errors.Is(err, ErrNotUnifiedDiff) {
		t.Errorf("NewMultiFileStdinReader(\"\") error = %v, want ErrNotUnifiedDiff", err)
	}
}

func TestMultiFileStdinReader_NotUnifiedDiffSentinel(t *testing.T) {
	// plain text returns the sentinel so the caller can silently fall back
	_, err := NewMultiFileStdinReader("just some plain text\nnothing diff-like here\n")
	if !errors.Is(err, ErrNotUnifiedDiff) {
		t.Errorf("plain text NewMultiFileStdinReader error = %v, want ErrNotUnifiedDiff", err)
	}
}

func TestMultiFileStdinReader_ProseMentioningMarker(t *testing.T) {
	// the marker is referenced inside a sentence (not at line start) — must NOT sniff true
	content := "Documentation: the header `diff --git a/foo b/foo` separates file sections.\n" +
		"It is followed by `@@ -1,1 +1,1 @@` and the hunk body.\n"
	_, err := NewMultiFileStdinReader(content)
	if !errors.Is(err, ErrNotUnifiedDiff) {
		t.Errorf("prose mention NewMultiFileStdinReader error = %v, want ErrNotUnifiedDiff", err)
	}
}

func TestMultiFileStdinReader_HunkOnlyNoDiffGit(t *testing.T) {
	// hunk header without a "diff --git" boundary cannot be sectioned; reject sniff
	content := "--- a/file.go\n+++ b/file.go\n@@ -1,1 +1,1 @@\n-old\n+new\n"
	_, err := NewMultiFileStdinReader(content)
	if !errors.Is(err, ErrNotUnifiedDiff) {
		t.Errorf("hunk-only NewMultiFileStdinReader error = %v, want ErrNotUnifiedDiff", err)
	}
}

func TestMultiFileStdinReader_MalformedHunkHeaderFails(t *testing.T) {
	// hunk header start exceeds int64 range — matches the regex but Atoi fails,
	// triggering parseUnifiedDiff's only practical error path. The whole call
	// must fail so the caller falls back to raw-text mode rather than silently
	// dropping the bad section.
	content := `diff --git a/bad.go b/bad.go
index abc..def
--- a/bad.go
+++ b/bad.go
@@ -99999999999999999999999,1 +1,1 @@
-old
+new
`
	_, err := NewMultiFileStdinReader(content)
	if err == nil {
		t.Fatal("malformed hunk header NewMultiFileStdinReader should return error")
	}
	if errors.Is(err, ErrNotUnifiedDiff) {
		t.Errorf("malformed hunk header should NOT return ErrNotUnifiedDiff, got %v", err)
	}
}

func TestMultiFileStdinReader_PartialFailureFailsWhole(t *testing.T) {
	// first section parses cleanly, second section has a malformed hunk;
	// reader must fail so the caller falls back rather than rendering a
	// tree with one file silently dropped.
	content := `diff --git a/good.go b/good.go
index abc..def
--- a/good.go
+++ b/good.go
@@ -1,1 +1,1 @@
-old
+new

diff --git a/bad.go b/bad.go
index def..ghi
--- a/bad.go
+++ b/bad.go
@@ -99999999999999999999999,1 +1,1 @@
-old
+new
`
	_, err := NewMultiFileStdinReader(content)
	if err == nil {
		t.Fatal("partial failure NewMultiFileStdinReader should return error")
	}
	if !strings.Contains(err.Error(), "bad.go") {
		t.Errorf("error %q should reference the failing section path", err)
	}
}

func TestMultiFileStdinReader_RenameTargetWithSpaces(t *testing.T) {
	content := `diff --git "a/old name.go" "b/new name.go"
similarity index 100%
rename from old name.go
rename to new name.go
`
	r, err := NewMultiFileStdinReader(content)
	if err != nil {
		t.Fatalf("NewMultiFileStdinReader() error = %v", err)
	}
	files, err := r.ChangedFiles("", false)
	if err != nil {
		t.Fatalf("ChangedFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("ChangedFiles() returned %d files, want 1", len(files))
	}
	if files[0].Path != "new name.go" {
		t.Errorf("files[0].Path = %q, want %q", files[0].Path, "new name.go")
	}
	if files[0].Status != FileRenamed {
		t.Errorf("files[0].Status = %v, want FileRenamed", files[0].Status)
	}
}

func TestMultiFileStdinReader_PreservesOrder(t *testing.T) {
	content := `diff --git a/z.go b/z.go
index abc..def
--- a/z.go
+++ b/z.go
@@ -1,1 +1,1 @@
-old
+new

diff --git a/a.go b/a.go
index abc..def
--- a/a.go
+++ b/a.go
@@ -1,1 +1,1 @@
-old
+new

diff --git a/m.go b/m.go
index abc..def
--- a/m.go
+++ b/m.go
@@ -1,1 +1,1 @@
-old
+new
`

	r, err := NewMultiFileStdinReader(content)
	if err != nil {
		t.Fatalf("NewMultiFileStdinReader() error = %v", err)
	}

	files, err := r.ChangedFiles("", false)
	if err != nil {
		t.Fatalf("ChangedFiles() error = %v", err)
	}

	// files should be in diff order, not alphabetical
	if len(files) != 3 {
		t.Fatalf("ChangedFiles() returned %d files, want 3", len(files))
	}
	if files[0].Path != "z.go" {
		t.Errorf("files[0].Path = %q, want %q", files[0].Path, "z.go")
	}
	if files[1].Path != "a.go" {
		t.Errorf("files[1].Path = %q, want %q", files[1].Path, "a.go")
	}
	if files[2].Path != "m.go" {
		t.Errorf("files[2].Path = %q, want %q", files[2].Path, "m.go")
	}
}

