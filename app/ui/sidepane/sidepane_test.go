package sidepane

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureVisible(t *testing.T) {
	tests := []struct {
		name       string
		cursor     int
		offset     int
		count      int
		height     int
		wantCursor int
		wantOffset int
	}{
		{name: "cursor above viewport", cursor: 2, offset: 5, count: 20, height: 10, wantCursor: 2, wantOffset: 2},
		{name: "cursor below viewport", cursor: 18, offset: 5, count: 20, height: 10, wantCursor: 18, wantOffset: 9},
		{name: "cursor in range", cursor: 7, offset: 5, count: 20, height: 10, wantCursor: 7, wantOffset: 5},
		{name: "empty count", cursor: 0, offset: 0, count: 0, height: 10, wantCursor: 0, wantOffset: 0},
		{name: "count smaller than height", cursor: 3, offset: 0, count: 5, height: 10, wantCursor: 3, wantOffset: 0},
		{name: "negative height is no-op", cursor: 5, offset: 3, count: 20, height: -1, wantCursor: 5, wantOffset: 3},
		{name: "zero height is no-op", cursor: 5, offset: 3, count: 20, height: 0, wantCursor: 5, wantOffset: 3},
		{name: "cursor at start", cursor: 0, offset: 0, count: 10, height: 5, wantCursor: 0, wantOffset: 0},
		{name: "cursor at end", cursor: 9, offset: 0, count: 10, height: 5, wantCursor: 9, wantOffset: 5},
		{name: "offset clamped to max", cursor: 3, offset: 15, count: 10, height: 5, wantCursor: 3, wantOffset: 3},
		{name: "negative offset clamped to zero", cursor: 0, offset: -3, count: 10, height: 5, wantCursor: 0, wantOffset: 0},
		{name: "height equals count", cursor: 4, offset: 0, count: 5, height: 5, wantCursor: 4, wantOffset: 0},
		{name: "height of one", cursor: 5, offset: 0, count: 10, height: 1, wantCursor: 5, wantOffset: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cursor, offset := tt.cursor, tt.offset
			ensureVisible(&cursor, &offset, tt.count, tt.height)
			assert.Equal(t, tt.wantCursor, cursor, "cursor")
			assert.Equal(t, tt.wantOffset, offset, "offset")
		})
	}
}

func TestMotionValues_Exhaustive(t *testing.T) {
	require.NotEmpty(t, MotionValues, "MotionValues should not be empty")
	for _, m := range MotionValues {
		assert.NotEmpty(t, m.String(), "Motion.String() should be non-empty for %v", m)
	}
}

func TestDirectionValues_Exhaustive(t *testing.T) {
	require.NotEmpty(t, DirectionValues, "DirectionValues should not be empty")
	for _, d := range DirectionValues {
		assert.NotEmpty(t, d.String(), "Direction.String() should be non-empty for %v", d)
	}
}

func TestMotionValues_ExpectedCount(t *testing.T) {
	// 7 values: Unknown, Up, Down, PageUp, PageDown, First, Last
	assert.Len(t, MotionValues, 7, "expected 7 motion values")
}

func TestDirectionValues_ExpectedCount(t *testing.T) {
	// 3 values: Unknown, Next, Prev
	assert.Len(t, DirectionValues, 3, "expected 3 direction values")
}
