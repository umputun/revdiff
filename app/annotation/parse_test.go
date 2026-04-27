package annotation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_Empty(t *testing.T) {
	got, err := Parse(strings.NewReader(""))
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestParse_OnlyWhitespace(t *testing.T) {
	got, err := Parse(strings.NewReader("\n\n  \n"))
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestParse_SingleAdd(t *testing.T) {
	in := "## handler.go:43 (+)\nuse errors.Is()\n"
	got, err := Parse(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, Annotation{File: "handler.go", Line: 43, Type: "+", Comment: "use errors.Is()"}, got[0])
}

func TestParse_SingleDel(t *testing.T) {
	in := "## store.go:18 (-)\nremoved\n"
	got, err := Parse(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, Annotation{File: "store.go", Line: 18, Type: "-", Comment: "removed"}, got[0])
}

func TestParse_SingleContext(t *testing.T) {
	in := "## plan.md:3 ( )\ncontext note\n"
	got, err := Parse(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, Annotation{File: "plan.md", Line: 3, Type: " ", Comment: "context note"}, got[0])
}

func TestParse_FileLevel(t *testing.T) {
	in := "## handler.go (file-level)\nneeds refactor\n"
	got, err := Parse(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, Annotation{File: "handler.go", Line: 0, Type: "", Comment: "needs refactor"}, got[0])
}

func TestParse_Range(t *testing.T) {
	in := "## handler.go:43-67 (+)\nrefactor hunk\n"
	got, err := Parse(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, Annotation{File: "handler.go", Line: 43, EndLine: 67, Type: "+", Comment: "refactor hunk"}, got[0])
}

func TestParse_MultilineBody(t *testing.T) {
	in := "## handler.go:43 (+)\nfirst point\nsecond point\nthird point\n"
	got, err := Parse(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "first point\nsecond point\nthird point", got[0].Comment)
}

func TestParse_EscapedHeaderInBody(t *testing.T) {
	// FormatOutput prefixes body lines starting with "## " with a single space.
	// Parse must strip exactly one leading space from such lines.
	in := "## handler.go:43 (+)\nbody mentions\n ## not a header\nstill body\n"
	got, err := Parse(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "body mentions\n## not a header\nstill body", got[0].Comment)
}

func TestParse_MixedRecords(t *testing.T) {
	in := "## a.go (file-level)\nfile note\n\n## a.go:10 (+)\nline note\n\n## b.go:5 (-)\nb note\n"
	got, err := Parse(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"}, got[0])
	assert.Equal(t, Annotation{File: "a.go", Line: 10, Type: "+", Comment: "line note"}, got[1])
	assert.Equal(t, Annotation{File: "b.go", Line: 5, Type: "-", Comment: "b note"}, got[2])
}

func TestParse_DuplicateLastWriteWins(t *testing.T) {
	// Parse returns both entries; Store.Add applies last-write-wins on insertion.
	in := "## h.go:43 (+)\nold\n\n## h.go:43 (+)\nnew\n"
	got, err := Parse(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, got, 2)

	s := NewStore()
	for _, a := range got {
		s.Add(a)
	}
	anns := s.Get("h.go")
	require.Len(t, anns, 1)
	assert.Equal(t, "new", anns[0].Comment)
}

func TestParse_FileLevelPathWithColonNumberSuffix(t *testing.T) {
	// A path that genuinely ends in `:N` (or `:N-M`) must round-trip through
	// the file-level header form. The lazy regex would otherwise consume the
	// numeric tail into the line-number group and silently rewrite the path.
	cases := []struct {
		in   string
		want string
	}{
		{"## foo:12 (file-level)\nnote\n", "foo:12"},
		{"## weird:12-34 (file-level)\nnote\n", "weird:12-34"},
	}
	for _, tc := range cases {
		got, err := Parse(strings.NewReader(tc.in))
		require.NoError(t, err, "parse %q", tc.in)
		require.Len(t, got, 1)
		assert.Equal(t, tc.want, got[0].File)
		assert.Equal(t, 0, got[0].Line)
	}
}

func TestParse_FileLevelPathWithColonRoundTrip(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "weird:12", Line: 0, Type: "", Comment: "file note"})
	parsed, err := Parse(strings.NewReader(s.FormatOutput()))
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	assert.Equal(t, "weird:12", parsed[0].File)
	assert.Equal(t, 0, parsed[0].Line)
}

func TestParse_MalformedHeader(t *testing.T) {
	cases := []string{
		"## not a real header\nbody\n",
		"## file.go:abc (+)\nbody\n",
		"## file.go:10 (?)\nbody\n",
		"## file.go:10\nbody\n",
	}
	for _, in := range cases {
		_, err := Parse(strings.NewReader(in))
		assert.Error(t, err, "expected error for %q", in)
	}
}

func TestParse_BodyBeforeAnyHeader(t *testing.T) {
	// Garbage before the first header is an error.
	_, err := Parse(strings.NewReader("garbage\n## file.go:1 (+)\nbody\n"))
	assert.Error(t, err)
}

func TestParse_RoundTrip(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "a.go", Line: 0, Type: "", Comment: "file-level\nwith ## inside"})
	s.Add(Annotation{File: "a.go", Line: 10, Type: "+", Comment: "add note"})
	s.Add(Annotation{File: "a.go", Line: 20, EndLine: 25, Type: "-", Comment: "del range"})
	s.Add(Annotation{File: "b.go", Line: 5, Type: " ", Comment: "context\nmulti\n## escaped\nbottom"})

	formatted := s.FormatOutput()

	parsed, err := Parse(strings.NewReader(formatted))
	require.NoError(t, err)

	got := NewStore()
	for _, a := range parsed {
		got.Add(a)
	}

	assert.Equal(t, s.FormatOutput(), got.FormatOutput())
}

func TestParse_UnescapedHashInBodyToleratedAfterHeader(t *testing.T) {
	// LLM- or hand-authored bodies can include lines starting with "## "
	// without the leading-space escape. After a record header is in scope,
	// such lines are folded into the body rather than rejected.
	in := "## a.go:1 (+)\nbody before\n## not actually a header\nbody after\n"
	got, err := Parse(strings.NewReader(in))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "body before\n## not actually a header\nbody after", got[0].Comment)
}

func TestStore_Load_RoundTrip(t *testing.T) {
	src := NewStore()
	src.Add(Annotation{File: "a.go", Line: 0, Type: "", Comment: "file note"})
	src.Add(Annotation{File: "a.go", Line: 5, Type: "+", Comment: "line note"})

	dst := NewStore()
	require.NoError(t, dst.Load(strings.NewReader(src.FormatOutput())))
	assert.Equal(t, src.FormatOutput(), dst.FormatOutput())
}

func TestParse_RoundTripPreservesIndentedHashHeader(t *testing.T) {
	// Body lines whose non-space prefix is "## " must round-trip without
	// losing leading whitespace, regardless of indent depth.
	s := NewStore()
	s.Add(Annotation{File: "a.go", Line: 1, Type: "+", Comment: "## zero indent"})
	s.Add(Annotation{File: "a.go", Line: 2, Type: "+", Comment: " ## one space"})
	s.Add(Annotation{File: "a.go", Line: 3, Type: "+", Comment: "  ## two spaces"})

	parsed, err := Parse(strings.NewReader(s.FormatOutput()))
	require.NoError(t, err)
	require.Len(t, parsed, 3)
	assert.Equal(t, "## zero indent", parsed[0].Comment)
	assert.Equal(t, " ## one space", parsed[1].Comment)
	assert.Equal(t, "  ## two spaces", parsed[2].Comment)
}
