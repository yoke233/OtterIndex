package otidxcli

import "testing"

func TestParseDefaults(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"q", "hello"})
	_, opts, err := ExecuteForTest(cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if opts.ContextLines != 1 {
		t.Fatalf("ContextLines=%d", opts.ContextLines)
	}
}

func TestExcludeCSV(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"q", "k", "-x", "*.js,*.sql"})
	_, opts, err := ExecuteForTest(cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(opts.ExcludeGlobs) != 2 || opts.ExcludeGlobs[0] != "*.js" || opts.ExcludeGlobs[1] != "*.sql" {
		t.Fatalf("ExcludeGlobs=%v", opts.ExcludeGlobs)
	}
}

