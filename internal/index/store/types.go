package store

import "otterindex/internal/model"

type File struct {
	WorkspaceID string
	Path        string
	Size        int64
	MTime       int64
	Hash        string
}

type Chunk struct {
	Path        string
	SL          int
	EL          int
	Kind        string
	Title       string
	Text        string
	Snippet     string
	WorkspaceID string
}

type ChunkInput struct {
	SL    int
	EL    int
	Kind  string
	Title string
	Text  string
}

type SymbolInput struct {
	Kind      string
	Name      string
	SL        int
	SC        int
	EL        int
	EC        int
	Container string
	Lang      string
	Signature string
}

type CommentInput struct {
	Kind string
	Text string
	SL   int
	SC   int
	EL   int
	EC   int
	Lang string
}

type Workspace struct {
	ID        string
	Root      string
	CreatedAt int64
}

type FilePlan struct {
	Path   string
	Size   int64
	MTime  int64
	Hash   string
	Chunks []ChunkInput
	Syms   []SymbolInput
	Comms  []CommentInput
	Delete bool
}

type SearchResult struct {
	Chunks               []Chunk
	MatchCaseInsensitive bool
	Backend              string
}

type Store interface {
	Close() error
	Backend() string

	HasFTS() bool
	FTSReason() string

	GetVersion(workspaceID string) (int64, error)
	BumpVersion(workspaceID string) error
	EnsureWorkspace(id string, root string) error

	UpsertFile(workspaceID string, path string, size int64, mtime int64, hash string) error
	GetFile(workspaceID string, path string) (File, error)
	GetFileMeta(workspaceID string, path string) (File, bool, error)
	GetFilesStats(workspaceID string) (int, int64, error)
	ListFilesMeta(workspaceID string) (map[string]File, error)
	DeleteFile(workspaceID string, path string) error

	GetWorkspace(workspaceID string) (Workspace, error)
	SearchChunks(workspaceID string, keyword string, limit int, caseInsensitive bool) (SearchResult, error)

	ReplaceChunksBatch(workspaceID string, path string, chunks []ChunkInput) error
	ReplaceSymbolsBatch(workspaceID string, path string, syms []SymbolInput) error
	ReplaceCommentsBatch(workspaceID string, path string, comms []CommentInput) error

	ReplaceFileAll(workspaceID string, path string, size int64, mtime int64, hash string, chunks []ChunkInput, syms []SymbolInput, comms []CommentInput) error
	DeleteFileAll(workspaceID string, path string) error
	ReplaceFilesBatch(workspaceID string, plans []FilePlan) error

	FindMinEnclosingSymbols(workspaceID string, path string, line int) ([]model.SymbolItem, error)

	CountChunks(workspaceID string) (int, error)
	CountFiles(workspaceID string) (int, error)
}

type BuildPragmaApplier interface {
	ApplyBuildPragmas() error
}

type PragmaReader interface {
	QueryPragma(name string) (string, error)
}
