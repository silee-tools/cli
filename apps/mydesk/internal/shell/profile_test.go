package shell

import (
	"testing"
)

func TestFindBlock(t *testing.T) {
	tests := []struct {
		name    string
		content string
		found   bool
		start   int
		end     int
	}{
		{
			name:    "no block",
			content: "some content\n",
			found:   false,
		},
		{
			name:    "block present",
			content: "before\n" + StartMarker + "\nexport FOO=bar\n" + EndMarker + "\nafter\n",
			found:   true,
			start:   7,
			end:     7 + len(StartMarker) + len("\nexport FOO=bar\n") + len(EndMarker),
		},
		{
			name:    "only start marker",
			content: StartMarker + "\nexport FOO=bar\n",
			found:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, found := FindBlock(tt.content, StartMarker, EndMarker)
			if found != tt.found {
				t.Errorf("found = %v, want %v", found, tt.found)
			}
			if found {
				if start != tt.start {
					t.Errorf("start = %d, want %d", start, tt.start)
				}
				if end != tt.end {
					t.Errorf("end = %d, want %d", end, tt.end)
				}
			}
		})
	}
}

func TestUpsertBlock(t *testing.T) {
	tests := []struct {
		name    string
		content string
		block   string
		want    string
	}{
		{
			name:    "empty file",
			content: "",
			block:   "export FOO=bar",
			want:    StartMarker + "\nexport FOO=bar\n" + EndMarker + "\n",
		},
		{
			name:    "append to existing content",
			content: "existing line\n",
			block:   "export FOO=bar",
			want:    "existing line\n\n" + StartMarker + "\nexport FOO=bar\n" + EndMarker + "\n",
		},
		{
			name:    "append to content without trailing newline",
			content: "existing line",
			block:   "export FOO=bar",
			want:    "existing line\n\n" + StartMarker + "\nexport FOO=bar\n" + EndMarker + "\n",
		},
		{
			name:    "replace existing block",
			content: "before\n" + StartMarker + "\nold content\n" + EndMarker + "\nafter\n",
			block:   "new content",
			want:    "before\n" + StartMarker + "\nnew content\n" + EndMarker + "\nafter\n",
		},
		{
			name:    "replace block preserves content after",
			content: "line1\n" + StartMarker + "\nold\n" + EndMarker + "\nline2\nline3\n",
			block:   "new",
			want:    "line1\n" + StartMarker + "\nnew\n" + EndMarker + "\nline2\nline3\n",
		},
		{
			name:    "content ends with double newline",
			content: "existing\n\n",
			block:   "export FOO=bar",
			want:    "existing\n\n" + StartMarker + "\nexport FOO=bar\n" + EndMarker + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UpsertBlock(tt.content, StartMarker, EndMarker, tt.block)
			if got != tt.want {
				t.Errorf("UpsertBlock():\ngot:  %q\nwant: %q", got, tt.want)
			}
		})
	}
}
