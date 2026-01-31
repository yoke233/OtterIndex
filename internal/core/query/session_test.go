package query

import (
	"testing"
	"time"
)

func TestSessionPrefixReuse_NoSQL(t *testing.T) {
	sess := NewSessionStore(SessionOptions{TTL: 30 * time.Second})

	sqlCalls := 0
	sqlFn := func(q string, fetchN int) ([]candidateRow, error) {
		sqlCalls++
		return []candidateRow{
			{Path: "a.go", SL: 1, EL: 5, Text: "hello world"},
			{Path: "b.go", SL: 1, EL: 5, Text: "help me"},
			{Path: "c.go", SL: 1, EL: 5, Text: "say hello"},
		}, nil
	}

	base := Options{Unit: "block", Limit: 20, Offset: 0}
	ver := int64(10)

	_, err := queryWithSessionFetch(sess, ver, "index.db", "ws1", "hel", base, sqlFn)
	if err != nil {
		t.Fatalf("q1: %v", err)
	}
	if sqlCalls != 1 {
		t.Fatalf("sqlCalls=%d", sqlCalls)
	}

	_, err = queryWithSessionFetch(sess, ver, "index.db", "ws1", "hell", base, sqlFn)
	if err != nil {
		t.Fatalf("q2: %v", err)
	}
	if sqlCalls != 1 {
		t.Fatalf("expected sqlCalls still 1, got %d", sqlCalls)
	}
}

func TestNarrowCandidates_ANDTokens(t *testing.T) {
	rows := []candidateRow{
		{Text: "hello world"},
		{Text: "hello there"},
		{Text: "world there"},
	}
	out := narrowCandidates(rows, []string{"hello", "world"}, false)
	if len(out) != 1 {
		t.Fatalf("len=%d out=%v", len(out), out)
	}
}
