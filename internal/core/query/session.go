package query

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"otterindex/internal/index/sqlite"
	"otterindex/internal/model"
)

type candidateRow struct {
	Path    string
	SL      int
	EL      int
	Text    string
	Snippet string
}

type SessionOptions struct {
	TTL           time.Duration
	MinPrefixLen  int
	MaxCandidates int
}

type querySession struct {
	version       int64
	lastQ         string
	lastFetchN    int
	exhausted     bool
	workspaceRoot string
	hasFTS        bool
	ftsReason     string
	candidates    []candidateRow
	updatedAt     time.Time
}

type SessionStore struct {
	mu sync.Mutex

	ttl           time.Duration
	minPrefixLen  int
	maxCandidates int

	m map[string]*querySession
}

func NewSessionStore(opts SessionOptions) *SessionStore {
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	minPrefixLen := opts.MinPrefixLen
	if minPrefixLen <= 0 {
		minPrefixLen = 2
	}
	maxCandidates := opts.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = 2000
	}
	return &SessionStore{
		ttl:           ttl,
		minPrefixLen:  minPrefixLen,
		maxCandidates: maxCandidates,
		m:             map[string]*querySession{},
	}
}

func (s *SessionStore) ClearWorkspace(workspaceID string) {
	if s == nil {
		return
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return
	}
	prefix := fmt.Sprintf("ws=%s|", workspaceID)

	s.mu.Lock()
	for k := range s.m {
		if strings.HasPrefix(k, prefix) {
			delete(s.m, k)
		}
	}
	s.mu.Unlock()
}

func QueryWithSession(sess *SessionStore, version int64, dbPath string, workspaceID string, q string, opts Options) ([]model.ResultItem, error) {
	if sess == nil {
		return Query(dbPath, workspaceID, q, opts)
	}

	prefetchMin := 500
	if sess.maxCandidates > 0 && prefetchMin > sess.maxCandidates {
		prefetchMin = sess.maxCandidates
	}
	return queryWithSessionCommon(sess, version, dbPath, workspaceID, q, opts, func(dbPath string, workspaceID string, q string, opts Options) ([]model.ResultItem, queryInfo, error) {
		return queryWithInfo(dbPath, workspaceID, q, opts, prefetchMin)
	})
}

func queryWithSessionCommon(sess *SessionStore, version int64, dbPath string, workspaceID string, q string, opts Options, run func(dbPath string, workspaceID string, q string, opts Options) ([]model.ResultItem, queryInfo, error)) ([]model.ResultItem, error) {
	if run == nil {
		return nil, fmt.Errorf("run is nil")
	}

	dbPath = strings.TrimSpace(dbPath)
	workspaceID = strings.TrimSpace(workspaceID)
	q = strings.TrimSpace(q)
	if dbPath == "" {
		return nil, fmt.Errorf("dbPath is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspaceID is required")
	}
	if q == "" {
		return nil, fmt.Errorf("query is required")
	}

	opts.Unit = strings.TrimSpace(opts.Unit)
	if opts.Unit == "" {
		opts.Unit = "block"
	}
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}

	sessionKey := makeSessionKey(dbPath, workspaceID, opts)
	now := time.Now()

	var snapshot querySession
	var ok bool
	var narrowIn int
	var narrowOut int

	sess.mu.Lock()
	ses := sess.m[sessionKey]
	if ses != nil && now.Sub(ses.updatedAt) > sess.ttl {
		delete(sess.m, sessionKey)
		ses = nil
	}
	if ses != nil && ses.version != version {
		ses.version = version
		ses.lastQ = ""
		ses.lastFetchN = 0
		ses.exhausted = false
		ses.workspaceRoot = ""
		ses.hasFTS = false
		ses.ftsReason = ""
		ses.candidates = nil
	}

	if ses != nil && isPrefixExtension(ses.lastQ, q, sess.minPrefixLen, opts.CaseInsensitive || ses.hasFTS) && len(ses.candidates) > 0 {
		narrowIn = len(ses.candidates)

		tokens := extractQueryTerms(q)
		if len(tokens) == 0 {
			tokens = []string{q}
		}
		narrowed := narrowCandidates(ses.candidates, tokens, opts.CaseInsensitive || ses.hasFTS)

		truncated := false
		if sess.maxCandidates > 0 && len(narrowed) > sess.maxCandidates {
			narrowed = narrowed[:sess.maxCandidates]
			truncated = true
		}

		ses.lastQ = q
		ses.candidates = narrowed
		if truncated {
			ses.exhausted = false
		}
		ses.updatedAt = now

		narrowOut = len(narrowed)
		snapshot = *ses
		ok = true
	}
	sess.mu.Unlock()

	if ok {
		wantN := opts.Offset + opts.Limit
		if wantN < 0 {
			return nil, fmt.Errorf("limit+offset overflow")
		}

		if !snapshot.exhausted && narrowOut < wantN {
			ok = false
		}
	}

	if ok {
		cands := append([]candidateRow(nil), snapshot.candidates...)
		sessionItems, err := queryFromCandidates(dbPath, workspaceID, q, opts, queryInfo{
			workspaceRoot: snapshot.workspaceRoot,
			hasFTS:        snapshot.hasFTS,
			ftsReason:     snapshot.ftsReason,
			fetchN:        snapshot.lastFetchN,
			exhausted:     snapshot.exhausted,
			candidates:    cands,
		})
		if err != nil {
			return nil, err
		}

		if snapshot.exhausted || len(sessionItems) == opts.Limit {
			if ex := opts.Explain; ex != nil {
				ex.KV("cache_hit", "session")
				ex.KV("session_prefix", true)
				ex.KV("narrow_in", narrowIn)
				ex.KV("narrow_out", narrowOut)
			}
			return sessionItems, nil
		}
	}

	items, info, err := run(dbPath, workspaceID, q, opts)
	if err != nil {
		return nil, err
	}

	sess.mu.Lock()
	ses = sess.m[sessionKey]
	if ses == nil {
		ses = &querySession{}
		sess.m[sessionKey] = ses
	}
	ses.version = version
	ses.lastQ = q
	ses.lastFetchN = info.fetchN
	ses.exhausted = info.exhausted
	ses.workspaceRoot = info.workspaceRoot
	ses.hasFTS = info.hasFTS
	ses.ftsReason = info.ftsReason
	if len(info.candidates) > 0 {
		truncated := false
		if sess.maxCandidates > 0 && len(info.candidates) > sess.maxCandidates {
			info.candidates = info.candidates[:sess.maxCandidates]
			truncated = true
		}
		ses.candidates = append([]candidateRow(nil), info.candidates...)
		if truncated {
			ses.exhausted = false
		}
	} else {
		ses.candidates = nil
	}
	ses.updatedAt = now
	sess.mu.Unlock()

	return items, nil
}

func queryFromCandidates(dbPath string, workspaceID string, q string, opts Options, env queryInfo) ([]model.ResultItem, error) {
	ex := opts.Explain
	startTotal := time.Now()

	dbPath = strings.TrimSpace(dbPath)
	workspaceID = strings.TrimSpace(workspaceID)
	q = strings.TrimSpace(q)
	opts.Unit = strings.TrimSpace(opts.Unit)
	if opts.Unit == "" {
		opts.Unit = "block"
	}
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}
	if dbPath == "" {
		return nil, fmt.Errorf("dbPath is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspaceID is required")
	}
	if q == "" {
		return nil, fmt.Errorf("query is required")
	}

	if ex != nil {
		ex.KV("phase", "query")
		ex.KV("db_path", dbPath)
		ex.KV("workspace_id", workspaceID)
		ex.KV("q", q)
		ex.KV("case_insensitive", opts.CaseInsensitive)
		ex.KV("limit", opts.Limit)
		ex.KV("offset", opts.Offset)
		ex.KV("include_globs", opts.IncludeGlobs)
		ex.KV("exclude_globs", opts.ExcludeGlobs)
		ex.KV("unit", opts.Unit)
		if opts.Unit == "line" {
			ex.KV("context_lines", opts.ContextLines)
		}
	}

	if ex != nil {
		ex.KV("workspace_root", env.workspaceRoot)
		ex.KV("fts5", env.hasFTS)
		ex.KV("fts5_reason", env.ftsReason)
	}

	matchCaseInsensitive := opts.CaseInsensitive
	if env.hasFTS {
		matchCaseInsensitive = true
	}
	if ex != nil {
		ex.KV("match_case_insensitive", matchCaseInsensitive)
	}

	wantN := opts.Offset + opts.Limit
	if wantN < 0 {
		return nil, fmt.Errorf("limit+offset overflow")
	}

	pathTopN := 3
	if ex != nil {
		ex.KV("dedupe_topn", pathTopN)
		if env.fetchN > 0 {
			ex.KV("prefetch_n", env.fetchN)
		}
	}

	stopMatch := func() {}
	if ex != nil {
		stopMatch = ex.Timer("match")
	}
	items, err := buildItemsFromCandidates(env.candidates, q, opts, matchCaseInsensitive, pathTopN, ex)
	stopMatch()
	if err != nil {
		return nil, err
	}

	if ex != nil {
		ex.KV("items_after_dedupe", len(items))
	}

	items = sliceLimitOffset(items, opts.Offset, opts.Limit, ex)

	if opts.Unit == "symbol" {
		stopSymbol := func() {}
		if ex != nil {
			stopSymbol = ex.Timer("symbol")
		}
		if s, err := sqlite.Open(dbPath); err == nil {
			_ = s.EnsureWorkspace(workspaceID, "")
			fallback := refineSymbolRangesWithStore(s, workspaceID, items, ex)
			_ = s.Close()
			if ex != nil && fallback > 0 {
				ex.KV("unit_fallback", "symbol->block")
			}
		} else if ex != nil {
			ex.KV("unit_fallback", "symbol->block")
		}
		stopSymbol()
	}

	if env.workspaceRoot != "" {
		stopFile := func() {}
		if ex != nil {
			stopFile = ex.Timer("file_read")
		}
		refineRangesWithFiles(items, env.workspaceRoot, opts.Unit)
		stopFile()
	}

	if ex != nil {
		ex.KV("elapsed_ms_total", time.Since(startTotal).Milliseconds())
	}
	return items, nil
}

func makeSessionKey(dbPath string, workspaceID string, opts Options) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "ws=%s|db=%s", workspaceID, dbPath)
	_, _ = fmt.Fprintf(&b, "|unit=%s|i=%t", opts.Unit, opts.CaseInsensitive)
	_, _ = fmt.Fprintf(&b, "|limit=%d|offset=%d", opts.Limit, opts.Offset)
	_, _ = fmt.Fprintf(&b, "|ctx=%d", opts.ContextLines)

	if len(opts.IncludeGlobs) > 0 {
		inc := append([]string(nil), opts.IncludeGlobs...)
		sort.Strings(inc)
		_, _ = fmt.Fprintf(&b, "|inc=%s", strings.Join(inc, ","))
	}
	if len(opts.ExcludeGlobs) > 0 {
		exc := append([]string(nil), opts.ExcludeGlobs...)
		sort.Strings(exc)
		_, _ = fmt.Fprintf(&b, "|exc=%s", strings.Join(exc, ","))
	}
	return b.String()
}

func isPrefixExtension(oldQ string, newQ string, minLen int, caseInsensitive bool) bool {
	oldQ = strings.TrimSpace(oldQ)
	newQ = strings.TrimSpace(newQ)
	if len(oldQ) < minLen {
		return false
	}
	if len(newQ) <= len(oldQ) {
		return false
	}
	if !caseInsensitive {
		return strings.HasPrefix(newQ, oldQ)
	}
	return strings.HasPrefix(strings.ToLower(newQ), strings.ToLower(oldQ))
}

func narrowCandidates(in []candidateRow, tokens []string, caseInsensitive bool) []candidateRow {
	var cleaned []string
	for _, t := range tokens {
		trim := strings.TrimSpace(t)
		if trim == "" {
			continue
		}
		cleaned = append(cleaned, trim)
	}
	if len(cleaned) == 0 {
		return nil
	}

	out := make([]candidateRow, 0, len(in))
	if !caseInsensitive {
		for _, row := range in {
			text := row.Text
			ok := true
			for _, tok := range cleaned {
				if !strings.Contains(text, tok) {
					ok = false
					break
				}
			}
			if ok {
				out = append(out, row)
			}
		}
		return out
	}

	lower := make([]string, len(cleaned))
	for i := range cleaned {
		lower[i] = strings.ToLower(cleaned[i])
	}
	for _, row := range in {
		textLower := strings.ToLower(row.Text)
		ok := true
		for _, tok := range lower {
			if !strings.Contains(textLower, tok) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, row)
		}
	}
	return out
}

func queryWithSessionFetch(sess *SessionStore, version int64, dbPath string, workspaceID string, q string, opts Options, fetch func(q string, fetchN int) ([]candidateRow, error)) ([]model.ResultItem, error) {
	if fetch == nil {
		return nil, fmt.Errorf("fetch is nil")
	}
	if sess == nil {
		return nil, fmt.Errorf("session store is nil")
	}

	return queryWithSessionCommon(sess, version, dbPath, workspaceID, q, opts, func(dbPath string, workspaceID string, q string, opts Options) ([]model.ResultItem, queryInfo, error) {
		wantN := opts.Offset + opts.Limit
		if wantN < 0 {
			return nil, queryInfo{}, fmt.Errorf("limit+offset overflow")
		}
		fetchN := wantN * 5
		if fetchN < 100 {
			fetchN = 100
		}
		if len(opts.IncludeGlobs) > 0 || len(opts.ExcludeGlobs) > 0 {
			if fetchN < 500 {
				fetchN = 500
			}
		}

		rows, err := fetch(q, fetchN)
		if err != nil {
			return nil, queryInfo{}, err
		}
		info := queryInfo{
			workspaceRoot: "",
			hasFTS:        false,
			ftsReason:     "",
			candidates:    rows,
			fetchN:        fetchN,
			exhausted:     len(rows) < fetchN,
		}
		items, err := queryFromCandidates(dbPath, workspaceID, q, opts, info)
		return items, info, err
	})
}
