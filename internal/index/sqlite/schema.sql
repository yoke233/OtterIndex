-- Base schema (FTS created separately; optional).

CREATE TABLE IF NOT EXISTS workspaces (
  id TEXT PRIMARY KEY,
  root TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS meta (
  workspace_id TEXT PRIMARY KEY,
  version INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS files (
  workspace_id TEXT NOT NULL,
  path TEXT NOT NULL,
  size INTEGER NOT NULL,
  mtime INTEGER NOT NULL,
  hash TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (workspace_id, path),
  FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS chunks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id TEXT NOT NULL,
  path TEXT NOT NULL,
  sl INTEGER NOT NULL,
  el INTEGER NOT NULL,
  kind TEXT NOT NULL,
  title TEXT NOT NULL,
  text TEXT NOT NULL,
  FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_chunks_workspace_path ON chunks(workspace_id, path);

CREATE TABLE IF NOT EXISTS symbols (
  id INTEGER PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  path TEXT NOT NULL,
  kind TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  sl INTEGER NOT NULL,
  sc INTEGER NOT NULL DEFAULT 1,
  el INTEGER NOT NULL,
  ec INTEGER NOT NULL DEFAULT 1,
  container TEXT NOT NULL DEFAULT '',
  lang TEXT NOT NULL DEFAULT '',
  signature TEXT NOT NULL DEFAULT '',
  FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_symbols_path_range ON symbols(workspace_id, path, sl, el);

CREATE TABLE IF NOT EXISTS comments (
  id INTEGER PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  path TEXT NOT NULL,
  kind TEXT NOT NULL,
  sl INTEGER NOT NULL,
  sc INTEGER NOT NULL DEFAULT 1,
  el INTEGER NOT NULL,
  ec INTEGER NOT NULL DEFAULT 1,
  text TEXT NOT NULL DEFAULT '',
  lang TEXT NOT NULL DEFAULT '',
  FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_comments_path_range ON comments(workspace_id, path, sl, el);
