package main

import "testing"

func TestVersionLine(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"jg", "0.3.0", "jg v0.3.0 © 2026 silee-tools\n"},
		{"jg", "dev", "jg vdev © 2026 silee-tools\n"},
	}
	for _, tt := range tests {
		if got := versionLine(tt.name, tt.version); got != tt.want {
			t.Errorf("versionLine(%q, %q) = %q, want %q", tt.name, tt.version, got, tt.want)
		}
	}
}
