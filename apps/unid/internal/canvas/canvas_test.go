package canvas

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCanvas(t *testing.T) {
	cv := New(5, 3)
	assert.Equal(t, 5, cv.W)
	assert.Equal(t, 3, cv.H)
	assert.Equal(t, -1, cv.OwnerAt(0, 0))
}

func TestPutCharAndRender(t *testing.T) {
	cv := New(3, 1)
	require.NoError(t, cv.PutChar(0, 0, 'A', false, 0))
	require.NoError(t, cv.PutChar(1, 0, 'B', false, 0))
	assert.Equal(t, "AB", cv.Render())
}

func TestPutStrAndRender(t *testing.T) {
	cv := New(10, 1)
	require.NoError(t, cv.PutStr(0, 0, "Hello", false, 0))
	assert.Equal(t, "Hello", cv.Render())
}

func TestPutCharCJK(t *testing.T) {
	cv := New(4, 1)
	require.NoError(t, cv.PutChar(0, 0, '한', false, 0))
	assert.Equal(t, 0, cv.OwnerAt(0, 0))
	assert.Equal(t, 0, cv.OwnerAt(1, 0)) // continuation cell
	assert.Equal(t, "한", cv.Render())
}

func TestPutCharOutOfBounds(t *testing.T) {
	cv := New(3, 3)
	err := cv.PutChar(5, 0, 'X', false, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of canvas bounds")
}

func TestCollisionDetection(t *testing.T) {
	cv := New(5, 1)
	require.NoError(t, cv.PutChar(0, 0, 'A', true, 0))
	err := cv.PutChar(0, 0, 'B', true, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "collision")
}

func TestCollisionAllowsOverwrite(t *testing.T) {
	cv := New(5, 1)
	require.NoError(t, cv.PutChar(0, 0, 'A', false, 0))
	require.NoError(t, cv.PutChar(0, 0, 'B', false, 1))
	assert.Equal(t, "B", cv.Render())
}

func TestRenderTrimsTrailingSpaces(t *testing.T) {
	cv := New(10, 2)
	require.NoError(t, cv.PutChar(0, 0, 'X', false, 0))
	// Row 1 is all spaces → should be trimmed from trailing lines
	assert.Equal(t, "X", cv.Render())
}
