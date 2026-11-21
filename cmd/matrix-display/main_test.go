package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"-h"}, {"--help"}} {
		var out strings.Builder
		if err := run(context.Background(), args, &out); err != nil {
			t.Fatalf("run(%q): %v", args, err)
		}
		if !strings.HasPrefix(out.String(), "usage:") {
			t.Errorf("run(%q) wrote %q, want usage", args, out.String())
		}
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out strings.Builder
	if err := run(context.Background(), []string{"nope"}, &out); err == nil {
		t.Fatal("run(nope) succeeded, want an error")
	}
}
