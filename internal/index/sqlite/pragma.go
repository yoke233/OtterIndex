package sqlite

import (
	"fmt"
	"strings"
)

func (s *Store) ApplyBuildPragmas() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not open")
	}

	stmts := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA temp_store=MEMORY;",
		"PRAGMA cache_size=-262144;",
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) QueryPragma(name string) (string, error) {
	if s == nil || s.db == nil {
		return "", fmt.Errorf("store is not open")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("pragma name is required")
	}
	for _, r := range name {
		if r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			continue
		}
		return "", fmt.Errorf("invalid pragma name: %q", name)
	}

	var v any
	if err := s.db.QueryRow("PRAGMA " + name + ";").Scan(&v); err != nil {
		return "", err
	}

	switch vv := v.(type) {
	case nil:
		return "", nil
	case string:
		return vv, nil
	case []byte:
		return string(vv), nil
	default:
		return fmt.Sprint(vv), nil
	}
}

