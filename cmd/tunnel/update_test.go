package main

import "testing"

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v0.1.3", "v0.1.4", true},
		{"v0.1.3", "v0.1.3", false},
		{"v0.1.4", "v0.1.3", false},
		{"v0.9.0", "v0.10.0", true}, // multi-digit component
		{"dev", "v0.1.3", false},    // dev builds never update
		{"v0.1.3", "", false},       // empty latest
	}
	for _, c := range cases {
		got := isNewerVersion(c.current, c.latest)
		if got != c.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}
