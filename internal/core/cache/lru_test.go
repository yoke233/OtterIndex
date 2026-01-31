package cache

import "testing"

func TestLRU_EvictsOldest(t *testing.T) {
	c := NewLRU(2)
	c.Put("a", 1)
	c.Put("b", 2)
	_, _ = c.Get("a") // a becomes most-recent
	c.Put("c", 3)     // should evict b

	if _, ok := c.Get("b"); ok {
		t.Fatal("expected b evicted")
	}
	if v, ok := c.Get("a"); !ok || v.(int) != 1 {
		t.Fatalf("expected a present, got %v ok=%v", v, ok)
	}
}

