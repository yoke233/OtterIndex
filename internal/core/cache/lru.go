package cache

import (
	"container/list"
	"sync"
)

type entry struct {
	key string
	val any
}

type LRU struct {
	mu  sync.Mutex
	cap int
	ll  *list.List
	m   map[string]*list.Element
}

func NewLRU(capacity int) *LRU {
	if capacity <= 0 {
		capacity = 1
	}
	return &LRU{
		cap: capacity,
		ll:  list.New(),
		m:   map[string]*list.Element{},
	}
}

func (c *LRU) Get(key string) (any, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.m[key]; ok {
		c.ll.MoveToFront(el)
		return el.Value.(*entry).val, true
	}
	return nil, false
}

func (c *LRU) Put(key string, val any) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.m[key]; ok {
		el.Value.(*entry).val = val
		c.ll.MoveToFront(el)
		return
	}

	el := c.ll.PushFront(&entry{key: key, val: val})
	c.m[key] = el

	for c.ll.Len() > c.cap {
		last := c.ll.Back()
		if last == nil {
			break
		}
		ent := last.Value.(*entry)
		delete(c.m, ent.key)
		c.ll.Remove(last)
	}
}

