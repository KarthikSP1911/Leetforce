package main

import (
	"strings"
	"testing"
)

func TestRunUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		env  string
		args []string
		want string
	}{
		{"no args", "", nil, "usage"},
		{"no redis url", "", []string{"results"}, "LEETFORCE_REDIS_URL"},
		{"unknown command", "redis://127.0.0.1:1", []string{"bogus"}, "unknown command"},
		{"enqueue needs three args", "redis://127.0.0.1:1", []string{"enqueue", "p"}, "usage"},
		{"enqueue missing file", "redis://127.0.0.1:1", []string{"enqueue", "p", "python", "/no/such/file"}, "read source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LEETFORCE_REDIS_URL", tt.env)
			t.Setenv("LEETFORCE_QUEUE_PREFIX", "")
			err := run(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("run(%v) = %v, want error containing %q", tt.args, err, tt.want)
			}
		})
	}
}

func TestDestroyRefusesDefaultPrefix(t *testing.T) {
	t.Setenv("LEETFORCE_REDIS_URL", "redis://127.0.0.1:1")
	for _, prefix := range []string{"", "leetforce"} {
		t.Setenv("LEETFORCE_QUEUE_PREFIX", prefix)
		err := run([]string{"destroy"})
		if err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Fatalf("prefix %q: got %v, want refusal", prefix, err)
		}
	}
}
