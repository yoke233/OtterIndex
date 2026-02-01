package otidxd

import "encoding/json"

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *ErrorObject    `json:"error,omitempty"`
}

type ErrorObject struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type WorkspaceAddParams struct {
	Root   string `json:"root"`
	DBPath string `json:"db_path,omitempty"`
}

type IndexBuildParams struct {
	WorkspaceID  string   `json:"workspace_id"`
	ScanAll      bool     `json:"scan_all,omitempty"`
	IncludeGlobs []string `json:"include_globs,omitempty"`
	ExcludeGlobs []string `json:"exclude_globs,omitempty"`
}

type QueryParams struct {
	WorkspaceID      string   `json:"workspace_id"`
	Q               string   `json:"q"`
	Unit            string   `json:"unit,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	Offset          int      `json:"offset,omitempty"`
	ContextLines    int      `json:"context_lines,omitempty"`
	CaseInsensitive bool     `json:"case_insensitive,omitempty"`
	IncludeGlobs    []string `json:"include_globs,omitempty"`
	ExcludeGlobs    []string `json:"exclude_globs,omitempty"`
	Show            bool     `json:"show,omitempty"`
}

type WatchStartParams struct {
	WorkspaceID  string   `json:"workspace_id"`
	ScanAll      bool     `json:"scan_all,omitempty"`
	IncludeGlobs []string `json:"include_globs,omitempty"`
	ExcludeGlobs []string `json:"exclude_globs,omitempty"`
	SyncOnStart  bool     `json:"sync_on_start,omitempty"`
	DebounceMS   int      `json:"debounce_ms,omitempty"`
	SyncWorkers  int      `json:"sync_workers,omitempty"`
}

type WatchStopParams struct {
	WorkspaceID string `json:"workspace_id"`
}

type WatchStatusParams struct {
	WorkspaceID string `json:"workspace_id"`
}

type WatchStatusResult struct {
	Running bool `json:"running"`
}
