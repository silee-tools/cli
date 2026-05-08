package dsl

import (
	"testing"

	"github.com/silee-tools/unid/internal/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOverflow(t *testing.T) {
	cmds, err := Parse("overflow ellipsis")
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	assert.Equal(t, object.OverflowEllipsis, cmds[0].(*OverflowCmd).Mode)

	cmds, err = Parse("overflow hidden")
	require.NoError(t, err)
	assert.Equal(t, object.OverflowHidden, cmds[0].(*OverflowCmd).Mode)
}

func TestParseAlign(t *testing.T) {
	cmds, err := Parse("align center")
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	assert.Equal(t, object.AlignCenter, cmds[0].(*AlignCmd).Mode)

	cmds, err = Parse("align r")
	require.NoError(t, err)
	assert.Equal(t, object.AlignRight, cmds[0].(*AlignCmd).Mode)
}

func TestParseCollision(t *testing.T) {
	cmds, err := Parse("collision on")
	require.NoError(t, err)
	assert.True(t, cmds[0].(*CollisionCmd).On)

	cmds, err = Parse("collision off")
	require.NoError(t, err)
	assert.False(t, cmds[0].(*CollisionCmd).On)
}

func TestParseBox(t *testing.T) {
	cmds, err := Parse("box 2 3 10 5 id=mybox s=heavy c=Hello World")
	require.NoError(t, err)
	obj := cmds[0].(*ObjectCmd).Object.(*object.Rect)
	assert.Equal(t, 2, obj.Col)
	assert.Equal(t, 3, obj.Row)
	assert.Equal(t, 10, obj.Width)
	assert.Equal(t, 5, obj.Height)
	assert.Equal(t, "mybox", obj.ID)
	assert.Equal(t, object.BorderHeavy, obj.Style)
	assert.Equal(t, "Hello World", obj.Content)
}

func TestParseRectAlias(t *testing.T) {
	cmds, err := Parse("rect 0 0 5 3")
	require.NoError(t, err)
	_, ok := cmds[0].(*ObjectCmd).Object.(*object.Rect)
	assert.True(t, ok)
}

func TestParseText(t *testing.T) {
	cmds, err := Parse("text 5 10 c=Hello")
	require.NoError(t, err)
	obj := cmds[0].(*ObjectCmd).Object.(*object.Text)
	assert.Equal(t, 5, obj.Col)
	assert.Equal(t, 10, obj.Row)
	assert.Equal(t, "Hello", obj.Content)
}

func TestParseHLine(t *testing.T) {
	cmds, err := Parse("hline 0 5 20 s=dash lg=Label")
	require.NoError(t, err)
	obj := cmds[0].(*ObjectCmd).Object.(*object.HLine)
	assert.Equal(t, 20, obj.Length)
	assert.Equal(t, object.LineDash, obj.Style)
	require.NotNil(t, obj.Legend)
	assert.Equal(t, "Label", obj.Legend.Text)
}

func TestParseVLine(t *testing.T) {
	cmds, err := Parse("vline 5 0 10 s=heavy")
	require.NoError(t, err)
	obj := cmds[0].(*ObjectCmd).Object.(*object.VLine)
	assert.Equal(t, 10, obj.Length)
	assert.Equal(t, object.LineHeavy, obj.Style)
}

func TestParseArrow(t *testing.T) {
	cmds, err := Parse("arrow api.r db.t both head=▶ lg=sync")
	require.NoError(t, err)
	a := cmds[0].(*ArrowCmd)
	assert.Equal(t, "api", a.SrcID)
	assert.Equal(t, object.SideRight, a.SrcSide)
	assert.Equal(t, "db", a.DstID)
	assert.Equal(t, object.SideTop, a.DstSide)
	assert.True(t, a.Both)
	assert.True(t, a.HasHead)
	assert.Equal(t, '▶', a.Head)
	require.NotNil(t, a.Legend)
	assert.Equal(t, "sync", a.Legend.Text)
}

func TestParseArrowhead(t *testing.T) {
	cmds, err := Parse("arrowhead →")
	require.NoError(t, err)
	assert.Equal(t, '→', cmds[0].(*ArrowheadCmd).Ch)
}

func TestParseCommentsAndBlankLines(t *testing.T) {
	input := `# comment
collision on

# another comment
overflow ellipsis
`
	cmds, err := Parse(input)
	require.NoError(t, err)
	assert.Len(t, cmds, 2)
}

func TestParseErrorUnknownCommand(t *testing.T) {
	_, err := Parse("foobar 1 2 3")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command 'foobar'")
}

func TestParseErrorInvalidID(t *testing.T) {
	_, err := Parse("box 0 0 5 3 id=bad!id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}

func TestParseContentNewline(t *testing.T) {
	cmds, err := Parse(`text 0 0 c=line1<br>line2`)
	require.NoError(t, err)
	obj := cmds[0].(*ObjectCmd).Object.(*object.Text)
	assert.Equal(t, "line1\nline2", obj.Content)
}
