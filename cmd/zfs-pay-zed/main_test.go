package main

import "testing"

func TestParseArgs(t *testing.T) {
	tests := []struct {
		args    []string
		mode    string
		dryRun  bool
		wantErr bool
	}{
		{args: []string{"event"}, mode: "event"},
		{args: []string{"reconcile", "--dry-run"}, mode: "reconcile", dryRun: true},
		{args: []string{"cleanup"}, mode: "cleanup"},
		{args: []string{"status"}, wantErr: true},
		{args: []string{"event", "--force"}, wantErr: true},
	}
	for _, test := range tests {
		mode, dryRun, err := parseArgs(test.args)
		if (err != nil) != test.wantErr || mode != test.mode || dryRun != test.dryRun {
			t.Fatalf("parseArgs(%q) = %q, %v, %v", test.args, mode, dryRun, err)
		}
	}
}

func TestCommandTimeout(t *testing.T) {
	lookup := func(value string) func(string) string {
		return func(string) string { return value }
	}
	if got := commandTimeout(lookup("3s")); got.String() != "3s" {
		t.Fatalf("timeout = %s", got)
	}
	if got := commandTimeout(lookup("invalid")); got.String() != "20s" {
		t.Fatalf("fallback timeout = %s", got)
	}
}
