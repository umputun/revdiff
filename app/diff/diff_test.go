package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestGit_CommitLogRange(t *testing.T) {
	g := NewGit("")
	tests := []struct {
		name, ref, want string
	}{
		{"single ref maps to range ending at HEAD", "main", "main..HEAD"},
		{"explicit range passes through", "main..feature", "main..feature"},
		{"explicit range with ref that contains dots not a range", "v1.2.3", "v1.2.3..HEAD"},
		{"three-dot syntax treated as range", "main...feature", "main...feature"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, g.commitLogRange(tt.ref))
		})
	}
}

func TestGit_ParseCommitLog(t *testing.T) {
	g := NewGit("")

	t.Run("empty output returns nil", func(t *testing.T) {
		assert.Nil(t, g.parseCommitLog(readFixture(t, "gitlog_empty.txt")))
	})

	t.Run("single record", func(t *testing.T) {
		got := g.parseCommitLog(readFixture(t, "gitlog_single.txt"))
		require.Len(t, got, 1)
		assert.Equal(t, "abc123def456789012345678901234567890abcd", got[0].Hash)
		assert.Equal(t, "Eugene <umputun@gmail.com>", got[0].Author)
		assert.Equal(t, "Add commit info popup", got[0].Subject)
		assert.Equal(t, "This popup shows the list of commits\nin the current ref-based diff range.\nUseful for PR reviews.", got[0].Body)
		assert.Equal(t, 2026, got[0].Date.Year())
		assert.Equal(t, time.April, got[0].Date.Month())
		assert.Equal(t, 10, got[0].Date.Day())
	})

	t.Run("many records preserve order and separate bodies", func(t *testing.T) {
		got := g.parseCommitLog(readFixture(t, "gitlog_many.txt"))
		require.Len(t, got, 3)
		assert.Equal(t, "First commit", got[0].Subject)
		assert.Equal(t, "Body of first commit.", got[0].Body)
		assert.Equal(t, "Second commit", got[1].Subject)
		assert.Empty(t, got[1].Body)
		assert.Equal(t, "Third commit", got[2].Subject)
		assert.Equal(t, "Multi-line\nbody here.", got[2].Body)
	})

	t.Run("empty body produces empty Body field", func(t *testing.T) {
		got := g.parseCommitLog(readFixture(t, "gitlog_nobody.txt"))
		require.Len(t, got, 1)
		assert.Equal(t, "Fix typo", got[0].Subject)
		assert.Empty(t, got[0].Body)
	})

	t.Run("tricky content: CJK, ANSI escapes, tabs in body and subject", func(t *testing.T) {
		got := g.parseCommitLog(readFixture(t, "gitlog_tricky.txt"))
		require.Len(t, got, 4)

		// CJK chars preserved as UTF-8
		assert.Equal(t, "李雷 <lilei@example.com>", got[0].Author)
		assert.Equal(t, "添加中文支持", got[0].Subject)
		assert.Equal(t, "修复 CJK 字符宽度。", got[0].Body)

		// ANSI escapes stripped from both subject and body
		assert.Equal(t, "testred", got[1].Subject)
		assert.NotContains(t, got[1].Subject, "\x1b")
		assert.Equal(t, "payloadbold-green end", got[1].Body)
		assert.NotContains(t, got[1].Body, "\x1b")

		// tabs within the body are preserved (field separator is \x1f, not \t)
		assert.Equal(t, "tabs-in-body", got[2].Subject)
		assert.Equal(t, "col1\tcol2\tcol3", got[2].Body)

		// tabs within the subject are preserved too — subject and body share one
		// field split on the first newline, so tabs pass through verbatim
		assert.Equal(t, "sub\twith\ttabs", got[3].Subject)
		assert.Equal(t, "body with just one line", got[3].Body)
	})

	t.Run("malformed record (fewer than 4 fields) is skipped", func(t *testing.T) {
		// three fields only — no desc (subject/body)
		raw := "hash\x1fauthor\x1f2026-04-10T12:00:00-04:00\x00"
		assert.Empty(t, g.parseCommitLog(raw))
	})

	t.Run("records cap at MaxCommits", func(t *testing.T) {
		var b strings.Builder
		for range MaxCommits + 50 {
			b.WriteString("hash\x1fauthor\x1f2026-04-10T12:00:00-04:00\x1fsubject\x00")
		}
		got := g.parseCommitLog(b.String())
		assert.Len(t, got, MaxCommits)
	})

	t.Run("malformed date leaves zero-value Date", func(t *testing.T) {
		raw := "hash\x1fauthor\x1fnot-a-date\x1fsubject\nbody\x00"
		got := g.parseCommitLog(raw)
		require.Len(t, got, 1)
		assert.True(t, got[0].Date.IsZero())
	})

	t.Run("subject containing \\x1f absorbs into final field and US byte is sanitized", func(t *testing.T) {
		// crafted subject with US byte embedded — SplitN(4) absorbs all trailing
		// \x1f into the last field; sanitizeCommitText then drops the US byte so
		// the rendered subject has no framing artifacts
		raw := "hash\x1fauthor\x1f2026-04-10T12:00:00-04:00\x1fsubject\x1fwith-us\nactual body\x00"
		got := g.parseCommitLog(raw)
		require.Len(t, got, 1)
		assert.Equal(t, "subjectwith-us", got[0].Subject)
		assert.NotContains(t, got[0].Subject, "\x1f")
		assert.Equal(t, "actual body", got[0].Body)
	})

	t.Run("author ANSI escape is stripped", func(t *testing.T) {
		raw := "hash\x1fEvil\x1b[31mRed\x1b[0m <e@x>\x1f2026-04-10T12:00:00-04:00\x1fsubject\nbody\x00"
		got := g.parseCommitLog(raw)
		require.Len(t, got, 1)
		assert.Equal(t, "EvilRed <e@x>", got[0].Author)
		assert.NotContains(t, got[0].Author, "\x1b")
	})

	t.Run("author BEL and C1 single-byte CSI stripped", func(t *testing.T) {
		// BEL would ring the terminal bell; 0x9b is 8-bit CSI which some terminals
		// interpret the same as ESC [ and would trigger styling on the popup
		raw := "hash\x1fAlice\x07\u009b31m <a@x>\x1f2026-04-10T12:00:00-04:00\x1fsubject\nbody\x00"
		got := g.parseCommitLog(raw)
		require.Len(t, got, 1)
		assert.Equal(t, "Alice31m <a@x>", got[0].Author)
		assert.NotContains(t, got[0].Author, "\x07")
		assert.NotContains(t, got[0].Author, "\u009b")
	})

	t.Run("author with crafted US byte collapses into shifted fields but stays sanitized", func(t *testing.T) {
		// delimiter injection in author shifts downstream fields. sanitizeCommitText
		// still strips any ESC/BEL/C1 bytes in whatever content ends up in each
		// parsed slot, so no terminal-control sequence reaches the overlay
		raw := "hash\x1fEvil\x1fname\x1fwith\x1b[31mred\x1b[0m <e@x>\nbody\x00"
		got := g.parseCommitLog(raw)
		require.Len(t, got, 1)
		assert.NotContains(t, got[0].Author, "\x1f")
		assert.NotContains(t, got[0].Subject, "\x1b")
		assert.NotContains(t, got[0].Body, "\x1b")
		// date parse fails because the injected author shifted the date slot;
		// parser preserves the commit with a zero Date rather than dropping it
		assert.True(t, got[0].Date.IsZero())
	})
}

func TestGit_CommitLog_EmptyRefReturnsNil(t *testing.T) {
	g := NewGit("/nonexistent/dir")
	commits, err := g.CommitLog("")
	require.NoError(t, err)
	assert.Nil(t, commits)

	commits, err = g.CommitLog("   ")
	require.NoError(t, err)
	assert.Nil(t, commits)
}

func TestGit_CommitLog_InvalidDir(t *testing.T) {
	g := NewGit("/nonexistent/dir")
	_, err := g.CommitLog("HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit log")
}

func TestGit_CommitLog_SingleRefRange(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "a.txt", "a\n")
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "first commit\n\nBody of first commit.")

	writeFile(t, dir, "a.txt", "a\nb\n")
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "second commit")

	writeFile(t, dir, "a.txt", "a\nb\nc\n")
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "third commit")

	// single ref translates to HEAD~2..HEAD, selecting the two most-recent commits
	commits, err := g.CommitLog("HEAD~2")
	require.NoError(t, err)
	require.Len(t, commits, 2)
	assert.Equal(t, "third commit", commits[0].Subject)
	assert.Equal(t, "second commit", commits[1].Subject)
}

func TestGit_CommitLog_ExplicitRangeAndBodyStripping(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "a.txt", "a\n")
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "first")

	writeFile(t, dir, "a.txt", "a\nb\n")
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "second subject\n\nbody line 1\nbody line 2")

	commits, err := g.CommitLog("HEAD~1..HEAD")
	require.NoError(t, err)
	require.Len(t, commits, 1)
	assert.Equal(t, "second subject", commits[0].Subject)
	assert.Equal(t, "body line 1\nbody line 2", commits[0].Body)
	assert.False(t, commits[0].Date.IsZero())
	assert.Regexp(t, `^[0-9a-f]{40}$`, commits[0].Hash)
	assert.Contains(t, commits[0].Author, "<test@test.com>")
}

func TestGit_CommitLog_ErrorOnInvalidRef(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGit(dir)

	writeFile(t, dir, "a.txt", "a\n")
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "first")

	_, err := g.CommitLog("not-a-real-ref")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit log")
}

func TestSanitizeCommitText(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"no escape bytes passes through", "plain text", "plain text"},
		{"CSI SGR sequence fully removed", "red \x1b[31mtext\x1b[0m here", "red text here"},
		{"complex SGR with params", "\x1b[1;32mbold green\x1b[0m", "bold green"},
		{"question-mark extension parsed", "start\x1b[?25lhidden\x1b[?25hend", "starthiddenend"},
		{"stray escape without CSI dropped", "foo\x1bbar", "foobar"},
		{"multiple escapes", "\x1b[31ma\x1b[32mb\x1b[0mc", "abc"},
		{"CJK content preserved", "添加 \x1b[31m中文\x1b[0m 支持", "添加 中文 支持"},
		{"BEL byte dropped", "ring\x07bell", "ringbell"},
		{"backspace byte dropped", "back\x08space", "backspace"},
		{"DEL byte dropped", "del\x7fete", "delete"},
		{"C1 single-byte CSI (U+009B) dropped so ESC-equivalent sequence is broken", "fake\u009b31mtrick\u009b0m", "fake31mtrick0m"},
		{"C1 OSC (U+009D) dropped so window-title sequence is broken", "set\u009dtitle\u009c", "settitle"},
		{"framing US byte dropped defensively", "left\x1fright", "leftright"},
		{"framing RS byte dropped defensively", "left\x1eright", "leftright"},
		{"NUL byte dropped", "a\x00b", "ab"},
		{"tab and newline preserved", "a\tb\nc", "a\tb\nc"},
		{"CR dropped so crafted author cannot overwrite hash/meta via carriage return", "line1\r\nline2", "line1\nline2"},
		{"standalone CR dropped", "start\rend", "startend"},
		{"three-byte UTF-8 with continuation in C1 range preserved", "日本語", "日本語"},
		{"emoji preserved", "shipit 🚀", "shipit 🚀"},
		{"mixed ESC and C1 stripped together", "a\x1b[31mb\u009bc", "abc"},
		{"raw 0x9b byte (invalid UTF-8) dropped so 8-bit CSI injection cannot survive", "fake\x9b31mtrick\x9b0m", "fake31mtrick0m"},
		{"raw 0x9d byte (invalid UTF-8) dropped so 8-bit OSC injection cannot survive", "set\x9dtitle\x9c", "settitle"},
		{"stray high byte 0xff dropped as invalid UTF-8", "a\xffb", "ab"},
		{"valid UTF-8 preserved adjacent to stripped raw byte", "中\x9b文", "中文"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizeCommitText(tt.in))
		})
	}
}

func TestSplitCommitDesc(t *testing.T) {
	tests := []struct {
		name, in, subject, body string
	}{
		{"subject only", "fix typo", "fix typo", ""},
		{"subject plus body", "subject\nbody line", "subject", "body line"},
		{"blank separator line stripped between subject and body", "subject\n\nbody", "subject", "body"},
		{"multi-line body", "s\na\nb\nc", "s", "a\nb\nc"},
		{"trailing newline stripped", "s\nbody\n", "s", "body"},
		{"empty string", "", "", ""},
		{"double blank separator: only one newline stripped", "s\n\n\nbody", "s", "\nbody"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, b := splitCommitDesc(tt.in)
			assert.Equal(t, tt.subject, s)
			assert.Equal(t, tt.body, b)
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
