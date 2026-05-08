package renderer

import (
	"testing"

	"github.com/silee-tools/unid/internal/canvas"
	"github.com/silee-tools/unid/internal/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDrawRectLight(t *testing.T) {
	cv := canvas.New(7, 3)
	r := New(cv, false)
	rect := object.NewRect(0, 0, 5, 1)
	require.NoError(t, r.Draw(rect))
	out := r.Render()
	assert.Contains(t, out, "┌─────┐")
	assert.Contains(t, out, "│     │")
	assert.Contains(t, out, "└─────┘")
}

func TestDrawRectWithContent(t *testing.T) {
	cv := canvas.New(7, 3)
	r := New(cv, false)
	rect := object.NewRect(0, 0, 5, 1)
	rect.Content = "Hi"
	rect.HasContent = true
	require.NoError(t, r.Draw(rect))
	assert.Contains(t, r.Render(), "Hi")
}

func TestDrawHLine(t *testing.T) {
	cv := canvas.New(5, 1)
	r := New(cv, false)
	require.NoError(t, r.Draw(object.NewHLine(0, 0, 5)))
	assert.Equal(t, "─────", r.Render())
}

func TestDrawVLine(t *testing.T) {
	cv := canvas.New(1, 3)
	r := New(cv, false)
	require.NoError(t, r.Draw(object.NewVLine(0, 0, 3)))
	assert.Equal(t, "│\n│\n│", r.Render())
}

func TestDrawText(t *testing.T) {
	cv := canvas.New(10, 1)
	r := New(cv, false)
	require.NoError(t, r.Draw(object.NewText(0, 0, "Hello")))
	assert.Equal(t, "Hello", r.Render())
}

func TestCollisionDetected(t *testing.T) {
	cv := canvas.New(10, 3)
	r := New(cv, true)
	require.NoError(t, r.Draw(object.NewRect(0, 0, 5, 1)))
	err := r.Draw(object.NewRect(0, 0, 5, 1))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "collision")
}

func TestEllipsisTruncate(t *testing.T) {
	result := ellipsisTruncate("LongServiceName", 15, 8, object.AlignLeft)
	assert.Equal(t, "Long..11", result) // prefix "Long" + ".." + "11" (truncated 11 cols)
}

func TestHiddenTruncate(t *testing.T) {
	result := hiddenTruncate("LongServiceName", 9, object.AlignLeft)
	assert.Equal(t, "LongServi", result)
}

func TestHiddenTruncateRight(t *testing.T) {
	result := hiddenTruncate("LongServiceName", 9, object.AlignRight)
	assert.Equal(t, "rviceName", result)
}

func TestDrawAllTwoPass(t *testing.T) {
	cv := canvas.New(10, 3)
	r := New(cv, false)
	rect := object.NewRect(0, 0, 8, 1)
	rect.Content = "Test"
	rect.HasContent = true
	text := object.NewText(1, 1, "Over")
	require.NoError(t, r.DrawAll([]object.DrawObject{rect, text}))
	out := r.Render()
	// text overwrites rect border in pass 2
	assert.Contains(t, out, "Over")
}
