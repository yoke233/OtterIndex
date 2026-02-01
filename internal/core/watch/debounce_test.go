package watch

import (
	"testing"
	"time"
)

func TestDebouncerDelayFor_Adaptive(t *testing.T) {
	d := NewDebouncer(200 * time.Millisecond)
	d.SetDelayFunc(func(count int) time.Duration {
		switch {
		case count <= 10:
			return 50 * time.Millisecond
		case count <= 100:
			return 100 * time.Millisecond
		default:
			return 200 * time.Millisecond
		}
	})

	if got := d.DelayFor(5); got != 50*time.Millisecond {
		t.Fatalf("delay for 5: %v", got)
	}
	if got := d.DelayFor(50); got != 100*time.Millisecond {
		t.Fatalf("delay for 50: %v", got)
	}
	if got := d.DelayFor(500); got != 200*time.Millisecond {
		t.Fatalf("delay for 500: %v", got)
	}
}
