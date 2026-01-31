package otidxcli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

type ExplainOptions struct {
	Format string
}

type ExplainCollector struct {
	mu      sync.Mutex
	format  string
	kv      map[string]any
	timings map[string]time.Duration
}

func NewExplainCollector(opts ExplainOptions) *ExplainCollector {
	format := strings.TrimSpace(opts.Format)
	if format == "" {
		format = "text"
	}
	return &ExplainCollector{
		format:  format,
		kv:      map[string]any{},
		timings: map[string]time.Duration{},
	}
}

func (e *ExplainCollector) KV(key string, value any) {
	if e == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	e.mu.Lock()
	e.kv[key] = value
	e.mu.Unlock()
}

func (e *ExplainCollector) Timer(name string) func() {
	if e == nil {
		return func() {}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return func() {}
	}
	start := time.Now()
	return func() {
		d := time.Since(start)
		e.mu.Lock()
		e.timings[name] += d
		e.mu.Unlock()
	}
}

func (e *ExplainCollector) Snapshot() map[string]any {
	if e == nil {
		return map[string]any{}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	out := make(map[string]any, len(e.kv)+1)
	for k, v := range e.kv {
		out[k] = v
	}
	if len(e.timings) > 0 {
		tm := make(map[string]int64, len(e.timings))
		for k, d := range e.timings {
			tm[k] = d.Milliseconds()
		}
		out["timings_ms"] = tm
	}
	return out
}

func (e *ExplainCollector) Emit(w io.Writer) error {
	if e == nil || w == nil {
		return nil
	}

	snap := e.Snapshot()

	switch e.format {
	case "json":
		b, err := json.Marshal(snap)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(b))
		return err
	default:
		_, _ = fmt.Fprintln(w, "explain:")

		keys := make([]string, 0, len(snap))
		for k := range snap {
			if k == "timings_ms" {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			_, _ = fmt.Fprintf(w, "  %s: %v\n", k, snap[k])
		}

		tm, _ := snap["timings_ms"].(map[string]int64)
		if len(tm) == 0 {
			return nil
		}
		names := make([]string, 0, len(tm))
		for k := range tm {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, name := range names {
			_, _ = fmt.Fprintf(w, "  elapsed_ms_%s: %d\n", name, tm[name])
		}
		return nil
	}
}

func (e *ExplainCollector) EmitToStringForTest() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	_ = e.Emit(&b)
	return strings.TrimRight(b.String(), "\r\n")
}

