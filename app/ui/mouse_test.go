package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/sidepane"
)

func TestModel_statusBarHeight(t *testing.T) {
	tests := []struct {
		name        string
		noStatusBar bool
		want        int
	}{
		{"status bar visible", false, 1},
		{"status bar hidden", true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel([]string{"a.go"}, nil)
			m.cfg.noStatusBar = tc.noStatusBar
			assert.Equal(t, tc.want, m.statusBarHeight())
		})
	}
}

func TestModel_diffTopRow(t *testing.T) {
	// diffTopRow is constant regardless of layout state — it always accounts
	// for the pane top border (row 0) and the diff header (row 1).
	m := testModel([]string{"a.go"}, nil)
	assert.Equal(t, 2, m.diffTopRow(), "diff viewport starts at row 2 (below border + header)")

	t.Run("single file mode", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.file.singleFile = true
		assert.Equal(t, 2, m.diffTopRow())
	})

	t.Run("no status bar", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.cfg.noStatusBar = true
		assert.Equal(t, 2, m.diffTopRow())
	})

	t.Run("tree hidden", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.layout.treeHidden = true
		assert.Equal(t, 2, m.diffTopRow())
	})
}

func TestModel_treeTopRow(t *testing.T) {
	// treeTopRow is constant — tree content begins at row 1 (below the top border).
	m := testModel([]string{"a.go"}, nil)
	assert.Equal(t, 1, m.treeTopRow(), "tree content starts at row 1 (below border)")

	t.Run("no status bar", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.cfg.noStatusBar = true
		assert.Equal(t, 1, m.treeTopRow())
	})
}

func TestModel_hitTest(t *testing.T) {
	// baseline layout: width=120, height=40, treeWidth=36, status bar visible.
	// tree block occupies x=[0, 37] (treeWidth+2 cols with borders),
	// diff block x=[38, 119].
	// rows: 0=top border, 1=diff header / tree row 0, 2..=content, 38=bottom border, 39=status.
	tests := []struct {
		name  string
		setup func(m *Model)
		x, y  int
		want  hitZone
	}{
		{name: "tree pane entry row", setup: func(m *Model) {}, x: 5, y: 3, want: hitTree},
		{name: "tree pane top border", setup: func(m *Model) {}, x: 5, y: 0, want: hitNone},
		{name: "tree pane first content row", setup: func(m *Model) {}, x: 5, y: 1, want: hitTree},
		{name: "diff pane viewport row", setup: func(m *Model) {}, x: 60, y: 10, want: hitDiff},
		{name: "diff pane header row", setup: func(m *Model) {}, x: 60, y: 1, want: hitHeader},
		{name: "diff pane top border", setup: func(m *Model) {}, x: 60, y: 0, want: hitHeader},
		{name: "status bar", setup: func(m *Model) {}, x: 60, y: 39, want: hitStatus},
		{name: "status bar at x=0", setup: func(m *Model) {}, x: 0, y: 39, want: hitStatus},
		{name: "tree-diff boundary: last tree column", setup: func(m *Model) {}, x: 37, y: 10, want: hitTree},
		{name: "tree-diff boundary: first diff column", setup: func(m *Model) {}, x: 38, y: 10, want: hitDiff},
		{name: "x negative", setup: func(m *Model) {}, x: -1, y: 10, want: hitNone},
		{name: "y negative", setup: func(m *Model) {}, x: 60, y: -1, want: hitNone},
		{name: "x out of bounds (= width)", setup: func(m *Model) {}, x: 120, y: 10, want: hitNone},
		{name: "y out of bounds (= height)", setup: func(m *Model) {}, x: 60, y: 40, want: hitNone},
		{
			name: "tree hidden: click in former tree area goes to diff",
			setup: func(m *Model) {
				m.layout.treeHidden = true
				m.layout.treeWidth = 0
			},
			x: 5, y: 10, want: hitDiff,
		},
		{
			name: "tree hidden: y=1 is diff header even at x=0",
			setup: func(m *Model) {
				m.layout.treeHidden = true
				m.layout.treeWidth = 0
			},
			x: 5, y: 1, want: hitHeader,
		},
		{
			name: "single file without TOC: no tree zone",
			setup: func(m *Model) {
				m.file.singleFile = true
				m.file.mdTOC = nil
				m.layout.treeWidth = 0
			},
			x: 5, y: 10, want: hitDiff,
		},
		{
			name: "single file with TOC: tree zone active",
			setup: func(m *Model) {
				m.file.singleFile = true
				m.file.mdTOC = sidepane.ParseTOC(
					[]diff.DiffLine{{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext}},
					"README.md",
				)
			},
			x: 5, y: 10, want: hitTree,
		},
		{
			name: "no status bar: last row is diff, not status",
			setup: func(m *Model) {
				m.cfg.noStatusBar = true
			},
			x: 60, y: 39, want: hitDiff,
		},
		{
			name: "no status bar: last row in tree zone is hitTree",
			setup: func(m *Model) {
				m.cfg.noStatusBar = true
			},
			x: 5, y: 39, want: hitTree,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel([]string{"a.go"}, nil)
			m.layout.width = 120
			m.layout.height = 40
			m.layout.treeWidth = 36
			tc.setup(&m)
			assert.Equal(t, tc.want, m.hitTest(tc.x, tc.y),
				"hitTest(%d, %d) with setup %q", tc.x, tc.y, tc.name)
		})
	}
}
