package search

import "testing"

func TestFindInText(t *testing.T) {
	ms := FindInText("x\nhello\nz\n", "hello", false)
	if len(ms) != 1 || ms[0].Line != 2 || ms[0].Col != 1 {
		t.Fatalf("matches=%v", ms)
	}
}

