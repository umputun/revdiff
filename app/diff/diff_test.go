package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseUnifiedDiff_SimpleAdd(t *testing.T) {
	raw := readFixture(t, "simple_add.diff")
	lines, err := parseUnifiedDiff(raw)
	require.NoError(t, err)

	// expected: context, blank, context, add, add, add, context, context
	require.Len(t, lines, 8)

	assert.Equal(t, ChangeContext, lines[0].ChangeType)
	assert.Equal(t, "package main", lines[0].Content)
	assert.Equal(t, 1, lines[0].OldNum)
	assert.Equal(t, 1, lines[0].NewNum)
	assert.False(t, lines[0].IsBinary, "text lines should not have IsBinary set")

	// blank line (empty context)
	assert.Equal(t, ChangeContext, lines[1].ChangeType)
	assert.Empty(t, lines[1].Content)

	assert.Equal(t, ChangeContext, lines[2].ChangeType)
	assert.Equal(t, "func handle() {", lines[2].Content)

	// three added lines
	assert.Equal(t, ChangeAdd, lines[3].ChangeType)
	assert.Equal(t, "    if err != nil {", lines[3].Content)
	assert.Equal(t, 0, lines[3].OldNum)
	assert.Equal(t, 4, lines[3].NewNum)

	assert.Equal(t, ChangeAdd, lines[4].ChangeType)
	assert.Equal(t, "        return", lines[4].Content)

	assert.Equal(t, ChangeAdd, lines[5].ChangeType)
	assert.Equal(t, "    }", lines[5].Content)

	assert.Equal(t, ChangeContext, lines[6].ChangeType)
	assert.Equal(t, "    fmt.Println(\"done\")", lines[6].Content)
	assert.Equal(t, 4, lines[6].OldNum)
	assert.Equal(t, 7, lines[6].NewNum)

	assert.Equal(t, ChangeContext, lines[7].ChangeType)
	assert.Equal(t, "}", lines[7].Content)
}

func TestParseUnifiedDiff_SimpleRemove(t *testing.T) {
	raw := readFixture(t, "simple_remove.diff")
	lines, err := parseUnifiedDiff(raw)
	require.NoError(t, err)
	require.NotEmpty(t, lines, "expected non-empty result")

	// verify we have removed lines
	var removals, additions int
	for _, l := range lines {
		switch l.ChangeType {
		case ChangeRemove:
			removals++
		case ChangeAdd:
			additions++
		case ChangeContext, ChangeDivider:
		}
	}
	assert.Equal(t, 3, removals, "expected 3 removed lines")
	assert.Equal(t, 1, additions, "expected 1 added line")
}

func TestParseUnifiedDiff_MultiHunk(t *testing.T) {
	raw := readFixture(t, "multi_hunk.diff")
	lines, err := parseUnifiedDiff(raw)
	require.NoError(t, err)

	// verify divider exists between hunks
	var dividers int
	for _, l := range lines {
		if l.ChangeType == ChangeDivider {
			dividers++
		}
	}
	assert.Equal(t, 1, dividers, "expected 1 divider between two hunks")

	// verify additions in both hunks
	var additions []string
	for _, l := range lines {
		if l.ChangeType == ChangeAdd {
			additions = append(additions, l.Content)
		}
	}
	assert.Equal(t, []string{`import "os"`, "    os.Exit(0)"}, additions)
}

func TestParseUnifiedDiff_MixedChanges(t *testing.T) {
	raw := readFixture(t, "mixed_changes.diff")
	lines, err := parseUnifiedDiff(raw)
	require.NoError(t, err)

	types := make([]ChangeType, 0, len(lines))
	for _, l := range lines {
		types = append(types, l.ChangeType)
	}

	// expected sequence: ctx, blank, ctx, remove, remove, add, add, ctx, ctx, blank
	assert.Equal(t, []ChangeType{
		ChangeContext,
		ChangeContext, // blank
		ChangeContext,
		ChangeRemove,
		ChangeRemove,
		ChangeAdd,
		ChangeAdd,
		ChangeContext,
		ChangeContext,
		ChangeContext, // blank trailing
	}, types)
}

func TestParseUnifiedDiff_Empty(t *testing.T) {
	lines, err := parseUnifiedDiff("")
	require.NoError(t, err)
	assert.Empty(t, lines)
}

func TestParseUnifiedDiff_Binary(t *testing.T) {
	raw := readFixture(t, "binary.diff")
	lines, err := parseUnifiedDiff(raw)
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, BinaryPlaceholder, lines[0].Content)
	assert.Equal(t, ChangeContext, lines[0].ChangeType)
	assert.Equal(t, 1, lines[0].OldNum)
	assert.Equal(t, 1, lines[0].NewNum)
	assert.True(t, lines[0].IsBinary, "binary placeholder should have IsBinary set")
}

func TestParseUnifiedDiff_BinaryNewFile(t *testing.T) {
	raw := "diff --git a/new.bin b/new.bin\nnew file mode 100644\nindex 0000000..dd12d3a\nBinary files /dev/null and b/new.bin differ\n"
	lines, err := parseUnifiedDiff(raw)
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "(new binary file)", lines[0].Content)
	assert.True(t, lines[0].IsBinary)
}

func TestParseUnifiedDiff_BinaryDeleted(t *testing.T) {
	raw := "diff --git a/old.bin b/old.bin\ndeleted file mode 100644\nindex 2dfe7e4..0000000\nBinary files a/old.bin and /dev/null differ\n"
	lines, err := parseUnifiedDiff(raw)
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "(deleted binary file)", lines[0].Content)
	assert.True(t, lines[0].IsBinary)
}

func TestParseUnifiedDiff_LineNumbers(t *testing.T) {
	raw := readFixture(t, "simple_add.diff")
	lines, err := parseUnifiedDiff(raw)
	require.NoError(t, err)

	// additions should have OldNum=0
	for _, l := range lines {
		if l.ChangeType == ChangeAdd {
			assert.Equal(t, 0, l.OldNum, "added lines should have OldNum=0")
			assert.Positive(t, l.NewNum, "added lines should have positive NewNum")
		}
	}

	// context lines should have both nums > 0
	for _, l := range lines {
		if l.ChangeType == ChangeContext {
			assert.Positive(t, l.OldNum, "context lines should have positive OldNum")
			assert.Positive(t, l.NewNum, "context lines should have positive NewNum")
		}
	}
}

func TestParseUnifiedDiff_RemoveLineNumbers(t *testing.T) {
	raw := readFixture(t, "simple_remove.diff")
	lines, err := parseUnifiedDiff(raw)
	require.NoError(t, err)

	for _, l := range lines {
		if l.ChangeType == ChangeRemove {
			assert.Positive(t, l.OldNum, "removed lines should have positive OldNum")
			assert.Equal(t, 0, l.NewNum, "removed lines should have NewNum=0")
		}
	}
}

func TestGit_ChangedFiles(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	// create and modify a file
	writeFile(t, dir, "hello.go", "package main\n\nfunc main() {}\n")
	gitCmd(t, dir, "add", "hello.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "hello.go", "package main\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n")

	entries, err := g.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "hello.go", Status: FileModified}}, entries)
}

func TestGit_ChangedFiles_Staged(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "a.go", "package a\n")
	gitCmd(t, dir, "add", "a.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "a.go", "package a\n\nvar x = 1\n")
	gitCmd(t, dir, "add", "a.go")

	entries, err := g.ChangedFiles("", true)
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "a.go", Status: FileModified}}, entries)
}

func TestGit_ChangedFiles_WithRef(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "b.go", "package b\n")
	gitCmd(t, dir, "add", "b.go")
	gitCmd(t, dir, "commit", "-m", "first")

	writeFile(t, dir, "b.go", "package b\n\nvar y = 2\n")
	gitCmd(t, dir, "add", "b.go")
	gitCmd(t, dir, "commit", "-m", "second")

	entries, err := g.ChangedFiles("HEAD~1", false)
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "b.go", Status: FileModified}}, entries)
}

func TestGit_ChangedFiles_NoChanges(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "c.go", "package c\n")
	gitCmd(t, dir, "add", "c.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	entries, err := g.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestGit_ChangedFiles_Error(t *testing.T) {
	g := NewGit("/nonexistent/repo")
	_, err := g.ChangedFiles("", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get changed files")
}

func TestGit_FileDiff(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "main.go", "package main\n\nfunc main() {\n}\n")
	gitCmd(t, dir, "add", "main.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "main.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n")

	lines, err := g.FileDiff("", "main.go", false)
	require.NoError(t, err)
	require.NotEmpty(t, lines, "expected non-empty diff lines")

	// verify we have both additions and context
	var hasAdd, hasCtx bool
	for _, l := range lines {
		switch l.ChangeType {
		case ChangeAdd:
			hasAdd = true
		case ChangeContext:
			hasCtx = true
		case ChangeRemove, ChangeDivider:
		}
	}
	assert.True(t, hasAdd, "expected addition lines")
	assert.True(t, hasCtx, "expected context lines")
}

func TestGit_FileDiff_Error(t *testing.T) {
	g := NewGit("/nonexistent/repo")
	_, err := g.FileDiff("", "main.go", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get file diff")
}

func TestParseUnifiedDiff_LongLine(t *testing.T) {
	// build a diff with a line exceeding default scanner buffer (64KB)
	longContent := strings.Repeat("x", 100_000)
	raw := "diff --git a/big.js b/big.js\n--- a/big.js\n+++ b/big.js\n@@ -1,1 +1,2 @@\n context\n+" + longContent + "\n"

	lines, err := parseUnifiedDiff(raw)
	require.NoError(t, err, "should handle lines up to 1MB without error")

	var hasAdd bool
	for _, l := range lines {
		if l.ChangeType == ChangeAdd && len(l.Content) == 100_000 {
			hasAdd = true
		}
	}
	assert.True(t, hasAdd, "should parse the long added line")
}

func TestGit_FileDiff_NoChanges(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "x.go", "package x\n")
	gitCmd(t, dir, "add", "x.go")
	gitCmd(t, dir, "commit", "-m", "initial")

	// no modifications, diff should be empty
	lines, err := g.FileDiff("", "x.go", false)
	require.NoError(t, err)
	assert.Empty(t, lines)
}

func TestGit_FileDiff_BinaryFile(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	// create and commit a binary file (1 KB of random-ish data with null bytes)
	binData := make([]byte, 1024)
	for i := range binData {
		binData[i] = byte(i % 256)
	}
	err := os.WriteFile(filepath.Join(dir, "image.png"), binData, 0o600)
	require.NoError(t, err)
	gitCmd(t, dir, "add", "image.png")
	gitCmd(t, dir, "commit", "-m", "add binary")

	// modify the binary file (2 KB now)
	binData2 := make([]byte, 2048)
	for i := range binData2 {
		binData2[i] = byte((i * 3) % 256)
	}
	err = os.WriteFile(filepath.Join(dir, "image.png"), binData2, 0o600)
	require.NoError(t, err)

	lines, err := g.FileDiff("", "image.png", false)
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, ChangeContext, lines[0].ChangeType)
	// should contain size info like "(binary file: 1.0 KB → 2.0 KB)"
	assert.Contains(t, lines[0].Content, "binary file")
	assert.Contains(t, lines[0].Content, "→")
	assert.Contains(t, lines[0].Content, "KB")
	assert.True(t, lines[0].IsBinary)
}

func TestGit_FileDiff_NewBinaryFile(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	// create initial commit so HEAD exists
	writeFile(t, dir, "README", "init\n")
	gitCmd(t, dir, "add", "README")
	gitCmd(t, dir, "commit", "-m", "init")

	// stage a new binary file
	binData := make([]byte, 512)
	for i := range binData {
		binData[i] = byte(i % 256)
	}
	err := os.WriteFile(filepath.Join(dir, "new.bin"), binData, 0o600)
	require.NoError(t, err)
	gitCmd(t, dir, "add", "new.bin")

	lines, err := g.FileDiff("", "new.bin", true)
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0].Content, "new binary file")
	assert.Contains(t, lines[0].Content, "512 B")
	assert.True(t, lines[0].IsBinary)
}

func TestGit_FileDiff_ModifiedEmptyBinaryFile(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, ".gitattributes", "*.bin binary\n")
	gitCmd(t, dir, "add", ".gitattributes")

	err := os.WriteFile(filepath.Join(dir, "empty.bin"), nil, 0o600)
	require.NoError(t, err)
	gitCmd(t, dir, "add", "empty.bin")
	gitCmd(t, dir, "commit", "-m", "add empty binary")

	err = os.WriteFile(filepath.Join(dir, "empty.bin"), []byte{0x00, 0x01, 0x02}, 0o600)
	require.NoError(t, err)

	lines, err := g.FileDiff("", "empty.bin", false)
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "(binary file: 0 B → 3 B)", lines[0].Content)
}

func TestGit_ChangedFiles_IncludesBinary(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	// commit a binary file, then modify it
	binData := make([]byte, 256)
	for i := range binData {
		binData[i] = byte(i % 256)
	}
	err := os.WriteFile(filepath.Join(dir, "data.bin"), binData, 0o600)
	require.NoError(t, err)
	gitCmd(t, dir, "add", "data.bin")
	gitCmd(t, dir, "commit", "-m", "add binary")

	binData[0] = 0xFF
	err = os.WriteFile(filepath.Join(dir, "data.bin"), binData, 0o600)
	require.NoError(t, err)

	entries, err := g.ChangedFiles("", false)
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "data.bin", Status: FileModified}}, entries)
}

func TestParseBinaryStat(t *testing.T) {
	g := NewGit("")

	tests := []struct {
		name    string
		input   string
		wantOld int64
		wantNew int64
		wantOK  bool
	}{
		{
			name:    "modified binary",
			input:   " image.png | Bin 1024 -> 2048 bytes\n 1 file changed, 0 insertions(+), 0 deletions(-)\n",
			wantOld: 1024,
			wantNew: 2048,
			wantOK:  true,
		},
		{
			name:    "new binary",
			input:   " new.bin | Bin 0 -> 512 bytes\n 1 file changed, 0 insertions(+), 0 deletions(-)\n",
			wantOld: 0,
			wantNew: 512,
			wantOK:  true,
		},
		{
			name:    "deleted binary",
			input:   " old.bin | Bin 4096 -> 0 bytes\n 1 file changed, 0 insertions(+), 0 deletions(-)\n",
			wantOld: 4096,
			wantNew: 0,
			wantOK:  true,
		},
		{
			name:    "filename cannot spoof stat",
			input:   " Bin 1 -> 2 bytes.bin | Bin 1024 -> 2048 bytes\n 1 file changed, 0 insertions(+), 0 deletions(-)\n",
			wantOld: 1024,
			wantNew: 2048,
			wantOK:  true,
		},
		{
			name:   "text file stat",
			input:  " main.go | 5 +++--\n 1 file changed, 3 insertions(+), 2 deletions(-)\n",
			wantOK: false,
		},
		{
			name:   "empty input",
			input:  "",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldSize, newSize, ok := g.parseBinaryStat(tt.input)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantOld, oldSize)
				assert.Equal(t, tt.wantNew, newSize)
			}
		})
	}
}

func TestParseBinaryChangeKind(t *testing.T) {
	g := NewGit("")

	tests := []struct {
		name  string
		input string
		want  binaryChangeKind
	}{
		{
			name:  "modified binary",
			input: " image.png | Bin 1024 -> 2048 bytes\n 1 file changed, 0 insertions(+), 0 deletions(-)\n",
			want:  binaryChangeModified,
		},
		{
			name:  "new binary",
			input: " new.bin | Bin 0 -> 512 bytes\n create mode 100644 new.bin\n",
			want:  binaryChangeAdded,
		},
		{
			name:  "deleted binary",
			input: " old.bin | Bin 4096 -> 0 bytes\n delete mode 100644 old.bin\n",
			want:  binaryChangeDeleted,
		},
		{
			name:  "summary mixed with stat output",
			input: " image.png | Bin 1024 -> 2048 bytes\n 1 file changed, 0 insertions(+), 0 deletions(-)\n create mode 100644 image.png\n",
			want:  binaryChangeAdded,
		},
		{
			name:  "empty input defaults to modified",
			input: "",
			want:  binaryChangeModified,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, g.parseBinaryChangeKind(tt.input))
		})
	}
}

func TestFormatSize(t *testing.T) {
	g := NewGit("")

	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, g.formatSize(tt.bytes))
		})
	}
}

func TestFormatBinaryDesc(t *testing.T) {
	g := NewGit("")

	tests := []struct {
		name    string
		kind    binaryChangeKind
		oldSize int64
		newSize int64
		want    string
	}{
		{"new file", binaryChangeAdded, 0, 2048, "(new binary file, 2.0 KB)"},
		{"deleted file", binaryChangeDeleted, 4096, 0, "(deleted binary file, 4.0 KB)"},
		{"modified file", binaryChangeModified, 1024, 2048, "(binary file: 1.0 KB → 2.0 KB)"},
		{"modified empty to non-empty", binaryChangeModified, 0, 100, "(binary file: 0 B → 100 B)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, g.formatBinaryDesc(tt.kind, tt.oldSize, tt.newSize))
		})
	}
}

// helpers

func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name)) //nolint:gosec // test fixture path
	require.NoError(t, err)
	return string(data)
}

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "user.email", "test@test.com")
	gitCmd(t, dir, "config", "user.name", "Test")
	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
	require.NoError(t, err)
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // args constructed internally, not user input
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}

func TestMatchesPrefix(t *testing.T) {
	prefixes := []string{"src", "pkg/util"}

	tests := []struct {
		file string
		want bool
	}{
		{"src/app.go", true},
		{"src/lib/deep.go", true},
		{"src", true},
		{"pkg/util/helper.go", true},
		{"pkg/util", true},
		{"pkg/other.go", false},
		{"srcutil/foo.go", false},
		{"other/src/foo.go", false},
		{"vendor/lib.go", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			assert.Equal(t, tt.want, matchesPrefix(tt.file, prefixes))
		})
	}
}

func TestNormalizePrefixes(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
		wantN int
	}{
		{"trailing slashes", []string{"src/", "vendor/"}, []string{"src", "vendor"}, 2},
		{"whitespace", []string{" src ", " vendor"}, []string{"src", "vendor"}, 2},
		{"empty strings", []string{"src", "", "  ", "vendor"}, []string{"src", "vendor"}, 2},
		{"nil input", nil, []string{}, 0},
		{"all empty", []string{"", " ", "  "}, []string{}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePrefixes(tt.input)
			assert.Equal(t, tt.want, got)
			assert.Len(t, got, tt.wantN)
		})
	}
}

func TestGit_UntrackedFiles(t *testing.T) {
	dir := t.TempDir()
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "user.name", "test")
	gitCmd(t, dir, "config", "user.email", "test@test.com")
	gitCmd(t, dir, "commit", "--allow-empty", "-m", "initial")

	// create tracked and untracked files
	writeFile(t, dir, "tracked.go", "package main\n")
	gitCmd(t, dir, "add", "tracked.go")
	gitCmd(t, dir, "commit", "-m", "add tracked")
	writeFile(t, dir, "untracked.go", "package main\n")
	writeFile(t, dir, "ignored.go", "ignored\n")
	writeFile(t, dir, ".gitignore", "ignored.go\n")

	g := NewGit(dir)
	files, err := g.UntrackedFiles()
	require.NoError(t, err)
	assert.Contains(t, files, "untracked.go")
	assert.NotContains(t, files, "tracked.go")
	assert.NotContains(t, files, "ignored.go")
	// .gitignore itself is untracked since we just created it
}
