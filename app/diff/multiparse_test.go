package diff

import (
	"testing"
)

func TestIsUnifiedDiff(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "empty",
			content: "",
			want:    false,
		},
		{
			name:    "raw text",
			content: "This is just plain text\nwithout any diff markers\n",
			want:    false,
		},
		{
			name:    "git diff header",
			content: "diff --git a/file.go b/file.go\nindex abc..def\n",
			want:    true,
		},
		{
			name:    "format-patch with leading mail headers",
			content: "From abc Mon Sep 17\nFrom: A\nSubject: foo\n\ndiff --git a/file.go b/file.go\n@@ -1,1 +1,1 @@\n",
			want:    true,
		},
		{
			name:    "hunk header only (no diff --git)",
			content: "--- a/file.go\n+++ b/file.go\n@@ -1,3 +1,4 @@\n",
			want:    false, // no diff --git boundary, splitter can't section it
		},
		{
			name:    "diff-like text in code, no line-start marker",
			content: "func TestDiff() {\n  // check @@ format\n  s := \"diff --git a/foo b/foo\"\n}\n",
			want:    false, // marker is mid-line inside a quoted string
		},
		{
			name:    "marker mentioned in prose, not line-anchored",
			content: "The header is `diff --git a/x b/x` and separates files.\n",
			want:    false,
		},
		{
			name:    "marker only at line start but after blank line",
			content: "\n\ndiff --git a/foo b/foo\n@@ -1,1 +1,1 @@\n",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUnifiedDiff(tt.content); got != tt.want {
				t.Errorf("isUnifiedDiff() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSplitMultiFileDiff(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int // number of sections
		wantErr bool
	}{
		{
			name:    "empty",
			raw:     "",
			wantErr: true,
		},
		{
			name: "single file",
			raw: `diff --git a/file.go b/file.go
index abc..def
--- a/file.go
+++ b/file.go
@@ -1,1 +1,2 @@
 line1
+line2
`,
			want: 1,
		},
		{
			name: "two files",
			raw: `diff --git a/file1.go b/file1.go
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
`,
			want: 2,
		},
		{
			name: "three files with new and deleted",
			raw: `diff --git a/new.go b/new.go
new file mode 100644
index 0000000..abc1234
--- /dev/null
+++ b/new.go
@@ -0,0 +1,1 @@
+new content

diff --git a/existing.go b/existing.go
index def..ghi
--- a/existing.go
+++ b/existing.go
@@ -1,1 +1,1 @@
-old
+new

diff --git a/deleted.go b/deleted.go
deleted file mode 100644
index jkl..0000000
--- a/deleted.go
+++ /dev/null
@@ -1,1 +0,0 @@
-deleted content
`,
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitMultiFileDiff(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("splitMultiFileDiff() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.want {
				t.Errorf("splitMultiFileDiff() got %d sections, want %d", len(got), tt.want)
			}
		})
	}
}

func TestParseFileHeader(t *testing.T) {
	tests := []struct {
		name       string
		section    string
		wantPath   string
		wantStatus FileStatus
	}{
		{
			name: "modified file",
			section: `diff --git a/file.go b/file.go
index abc..def
--- a/file.go
+++ b/file.go
@@ -1,1 +1,2 @@`,
			wantPath:   "file.go",
			wantStatus: FileModified,
		},
		{
			name: "new file",
			section: `diff --git a/new.go b/new.go
new file mode 100644
index 0000000..abc1234
--- /dev/null
+++ b/new.go
@@ -0,0 +1,1 @@`,
			wantPath:   "new.go",
			wantStatus: FileAdded,
		},
		{
			name: "deleted file",
			section: `diff --git a/deleted.go b/deleted.go
deleted file mode 100644
index abc..0000000
--- a/deleted.go
+++ /dev/null
@@ -1,1 +0,0 @@`,
			wantPath:   "deleted.go",
			wantStatus: FileDeleted,
		},
		{
			name: "renamed file",
			section: `diff --git a/old.go b/new.go
similarity index 100%
rename from old.go
rename to new.go`,
			wantPath:   "new.go",
			wantStatus: FileRenamed,
		},
		{
			name: "file with spaces",
			section: `diff --git "a/path with spaces/file.go" "b/path with spaces/file.go"
index abc..def
--- "a/path with spaces/file.go"
+++ "b/path with spaces/file.go"
@@ -1,1 +1,1 @@`,
			wantPath:   "path with spaces/file.go",
			wantStatus: FileModified,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, status := parseFileHeader(tt.section)
			if path != tt.wantPath {
				t.Errorf("parseFileHeader() path = %q, want %q", path, tt.wantPath)
			}
			if status != tt.wantStatus {
				t.Errorf("parseFileHeader() status = %v, want %v", status, tt.wantStatus)
			}
		})
	}
}

func TestCleanPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "b prefix",
			input: "b/path/file.go",
			want:  "path/file.go",
		},
		{
			name:  "a prefix",
			input: "a/path/file.go",
			want:  "path/file.go",
		},
		{
			name:  "quoted path",
			input: `"path/file.go"`,
			want:  "path/file.go",
		},
		{
			name:  "b prefix with quotes",
			input: `b/"path with spaces/file.go"`,
			want:  "path with spaces/file.go",
		},
		{
			name:  "no prefix",
			input: "path/file.go",
			want:  "path/file.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanPath(tt.input); got != tt.want {
				t.Errorf("cleanPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
