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
	if opts.Limit != 20 {
		t.Fatalf("Limit=%d", opts.Limit)
	}
	if opts.Offset != 0 {
		t.Fatalf("Offset=%d", opts.Offset)
	}
	if opts.Unit != "block" {
		t.Fatalf("Unit=%q", opts.Unit)
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

func TestIncludeRepeat(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"q", "k", "-g", "*.go", "-g", "docs/*.md"})
	_, opts, err := ExecuteForTest(cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(opts.IncludeGlobs) != 2 || opts.IncludeGlobs[0] != "*.go" || opts.IncludeGlobs[1] != "docs/*.md" {
		t.Fatalf("IncludeGlobs=%v", opts.IncludeGlobs)
	}
}

func TestThemePrecedence_NoColorWins(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"q", "k", "-b", "-Z", "-z"})
	_, opts, err := ExecuteForTest(cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if opts.Theme != "none" {
		t.Fatalf("Theme=%q", opts.Theme)
	}
}

func TestUnitInvalidIsError(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"q", "k", "--unit", "wat"})
	_, _, err := ExecuteForTest(cmd)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExplainNoValueDefaultsToText(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"q", "k", "--explain"})
	_, opts, err := ExecuteForTest(cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if opts.Explain != "text" {
		t.Fatalf("Explain=%q", opts.Explain)
	}
}
