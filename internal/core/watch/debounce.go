package watch

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type Debouncer struct {
	delay time.Duration

	mu     sync.Mutex
	timer  *time.Timer
	queued map[string]struct{}
	onFire func(paths []string)
}

func NewDebouncer(delay time.Duration) *Debouncer {
	if delay <= 0 {
		delay = 200 * time.Millisecond
	}
	return &Debouncer{
		delay:  delay,
		queued: map[string]struct{}{},
	}
}

func (d *Debouncer) OnFire(fn func(paths []string)) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.onFire = fn
	d.mu.Unlock()
}

func (d *Debouncer) Push(path string) {
	if d == nil {
		return
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}

	d.mu.Lock()
	d.queued[path] = struct{}{}
	if d.timer != nil {
		_ = d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.delay, d.fire)
	d.mu.Unlock()
}

func (d *Debouncer) fire() {
	d.mu.Lock()
	queued := d.queued
	d.queued = map[string]struct{}{}
	fn := d.onFire
	d.mu.Unlock()

	if fn == nil || len(queued) == 0 {
		return
	}

	paths := make([]string, 0, len(queued))
	for p := range queued {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	fn(paths)
}

