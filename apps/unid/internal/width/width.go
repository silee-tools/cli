package width

import "github.com/mattn/go-runewidth"

func CharWidth(ch rune) int {
	return runewidth.RuneWidth(ch)
}

func StrWidth(s string) int {
	return runewidth.StringWidth(s)
}

func PadToWidth(s string, width int) string {
	current := StrWidth(s)
	if current >= width {
		return s
	}
	pad := make([]byte, width-current)
	for i := range pad {
		pad[i] = ' '
	}
	return s + string(pad)
}
