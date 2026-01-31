package otidxcli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpContainsSubcommand(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "otidx") || !strings.Contains(s, "q") {
		t.Fatalf("help missing expected text: %s", s)
	}
}

