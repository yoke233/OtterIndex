package bleve

import (
	"encoding/json"
	"time"
)

const (
	bucketWorkspaces = "workspaces"
	bucketFiles      = "files"
)

type workspaceMeta struct {
	ID        string `json:"id"`
	Root      string `json:"root"`
	CreatedAt int64  `json:"created_at"`
	Version   int64  `json:"version"`
}

type fileMeta struct {
	Size         int64  `json:"size"`
	MTime        int64  `json:"mtime"`
	Hash         string `json:"hash"`
	ChunkCount   int    `json:"chunk_count"`
	SymbolCount  int    `json:"symbol_count"`
	CommentCount int    `json:"comment_count"`
}

func encodeJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

func decodeJSON(data []byte, target any) error {
	return json.Unmarshal(data, target)
}

func nowUnix() int64 {
	return time.Now().Unix()
}
