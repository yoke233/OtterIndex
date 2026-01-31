package search

import "testing"

func TestFindInText(t *testing.T) {
	ms := FindInText("x\nhello\nz\n", "hello", false)
	if len(ms) != 1 || ms[0].Line != 2 || ms[0].Col != 1 {
		t.Fatalf("matches=%v", ms)
	}
}

func TestFindInText_EmptyKeyword(t *testing.T) {
	ms := FindInText("x\nhello\nz\n", "", false)
	if len(ms) != 0 {
		t.Fatalf("matches=%v", ms)
	}
}

func TestFindInText_CaseInsensitive(t *testing.T) {
	ms := FindInText("x\nHeLLo\nz\n", "hello", true)
	if len(ms) != 1 || ms[0].Line != 2 || ms[0].Col != 1 {
		t.Fatalf("matches=%v", ms)
	}
}

func TestFindInText_ReportsColumnAndText(t *testing.T) {
	ms := FindInText("abc hello\n", "hello", false)
	if len(ms) != 1 || ms[0].Line != 1 || ms[0].Col != 5 || ms[0].Text != "abc hello" {
		t.Fatalf("matches=%v", ms)
	}
}
