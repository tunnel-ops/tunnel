package main

import "testing"

func TestParseTargets(t *testing.T) {
	cases := []struct {
		args        []string
		wantTargets []string
		wantOpen    bool
	}{
		{[]string{"4000"}, []string{"4000"}, false},
		{[]string{"4000", "--open"}, []string{"4000"}, true},
		{[]string{"4000", "5000", "api"}, []string{"4000", "5000", "api"}, false},
		{[]string{"4000", "5000", "--open"}, []string{"4000", "5000"}, true},
		{[]string{"--open", "4000"}, []string{"4000"}, true},
	}
	for _, c := range cases {
		targets, open := parseTargets(c.args)
		if open != c.wantOpen {
			t.Errorf("parseTargets(%v): open=%v, want %v", c.args, open, c.wantOpen)
		}
		if len(targets) != len(c.wantTargets) {
			t.Errorf("parseTargets(%v): targets=%v, want %v", c.args, targets, c.wantTargets)
			continue
		}
		for i := range targets {
			if targets[i] != c.wantTargets[i] {
				t.Errorf("parseTargets(%v): targets[%d]=%q, want %q", c.args, i, targets[i], c.wantTargets[i])
			}
		}
	}
}
