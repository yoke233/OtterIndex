package watch

import (
	"testing"
	"time"
)

func TestDebounce_Coalesces(t *testing.T) {
	d := NewDebouncer(200 * time.Millisecond)
	got := 0
	d.OnFire(func(paths []string) { got = len(paths) })

	d.Push("a.go")
	d.Push("a.go")
	time.Sleep(350 * time.Millisecond)

	if got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
}

