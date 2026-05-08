package width

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCharWidthASCII(t *testing.T) {
	assert.Equal(t, 1, CharWidth('A'))
	assert.Equal(t, 1, CharWidth(' '))
}

func TestCharWidthCJK(t *testing.T) {
	assert.Equal(t, 2, CharWidth('한'))
	assert.Equal(t, 2, CharWidth('漢'))
	assert.Equal(t, 2, CharWidth('あ'))
}

func TestStrWidthASCII(t *testing.T) {
	assert.Equal(t, 5, StrWidth("Hello"))
}

func TestStrWidthCJK(t *testing.T) {
	assert.Equal(t, 4, StrWidth("한글"))
	assert.Equal(t, 4, StrWidth("漢字"))
}

func TestStrWidthMixed(t *testing.T) {
	assert.Equal(t, 4, StrWidth("A한B"))
}

func TestPadToWidthASCII(t *testing.T) {
	assert.Equal(t, "Hi   ", PadToWidth("Hi", 5))
}

func TestPadToWidthCJK(t *testing.T) {
	assert.Equal(t, "한   ", PadToWidth("한", 5))
}

func TestPadToWidthAlreadyWider(t *testing.T) {
	assert.Equal(t, "Hello", PadToWidth("Hello", 3))
}
