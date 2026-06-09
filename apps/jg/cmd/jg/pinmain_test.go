package main

import "testing"

func TestShouldPinMain(t *testing.T) {
	cases := []struct {
		name          string
		cwd, mainPath string
		want          bool
	}{
		{"subdir pins", "/repo/sub", "/repo", true},
		{"at main root no pin", "/repo", "/repo", false},
		{"empty main no pin", "/repo/sub", "", false},
	}
	for _, tc := range cases {
		if got := shouldPinMain(tc.cwd, tc.mainPath); got != tc.want {
			t.Errorf("%s: shouldPinMain(%q, %q) = %v, want %v",
				tc.name, tc.cwd, tc.mainPath, got, tc.want)
		}
	}
}
