# OtterIndex (Go) Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 用 Go 实现本地 SQLite 的 OtterIndex：CLI `otidx`（关键词 `q` 一把梭 + 过滤/上下文/输出主题/调试可视化）+ daemon `otidxd`（先骨架），查询返回“最小代码/文本单元块”（可控力度 `--unit`），并返回相对路径与行号范围；后续保留 C 扩展（cgo SQLite/FTS5、自定义 tokenizer、tree-sitter symbol 单元）接口点。

**Architecture:** `otidx` 负责 CLI 参数/渲染；核心逻辑在 `internal/core`（文件枚举+ignore、分块、索引、查询、unitize）；索引落到本地 SQLite（默认 `.otidx/index.db`），优先用 FTS（失败则回退到 LIKE 扫描）；`otidxd` 提供 JSON-RPC（先 TCP 形态）供 IDE/Agent 调用。C 扩展后期通过 build tag 切换 sqlite 驱动与 tree-sitter。

**Tech Stack:** Go（`testing`），CLI：`github.com/spf13/cobra`，SQLite：`modernc.org/sqlite`（纯 Go，避免 cgo；后续可切 `github.com/mattn/go-sqlite3`），ignore：`github.com/sabhiram/go-gitignore`（或同类），glob：`github.com/bmatcuk/doublestar/v4`，JSON：`encoding/json`，可选颜色：`github.com/mattn/go-colorable` + 自定义 palette。

---

## CLI 约定（MVP 对齐你给的参数表 + 少量补充）

- `otidx q <q>`：关键词查询（默认命令也可等价于 `otidx "<q>"`，后续再做）
- `-d <dbname|path>`：指定 db（默认 `.otidx/index.db`）
- `-A`：扫描 ALL（关闭默认 ignore/hidden/二进制/大文件限制）
- `-x <globs>`：排除（逗号分隔，如 `-x *.js,*.sql`）
- `-g <glob>`：只包含这些文件（可重复）
- `-i`：大小写不敏感
- `-c <num>`：上下文行（默认 1，仅 `--unit line` 生效）
- `--unit <line|block|file>`：返回力度（默认 `block`）
- `-B`：不打印 banner
- `-L`：vim 友好行（`path:sl:sc:`）
- `-b`：色盲友好主题
- `-z`：关闭颜色
- `-Z`：高对比主题
- `-l`：列出可用数据库（默认搜索 `.otidx/*.db`）
- `-v`：打印版本并退出
- `-h`：帮助
- `--jsonl`：每行一个 JSON（给脚本/agent）
- `--explain`：打印执行计划/过滤条件/命中统计（stderr）
- `--viz ascii`：ASCII 管线图（调试）

## 输出结构（MVP）

统一结果结构（用于 `--jsonl` 和内部渲染）：

```json
{
  "kind": "unit",
  "path": "relative/path/to/file.go",
  "range": { "sl": 10, "sc": 1, "el": 42, "ec": 1 },
  "title": "optional",
  "snippet": "optional",
  "matches": [{ "line": 21, "col": 7, "text": "..." }]
}
```

---

### Task 1: 初始化 Go module + 版本信息（otidx/otidxd 可编译）

**Files:**
- Create: `go.mod`
- Create: `internal/version/version.go`
- Create: `internal/version/version_test.go`
- Create: `cmd/otidx/main.go`
- Create: `cmd/otidxd/main.go`

**Step 1: 写失败测试：版本字符串非空**

```go
// internal/version/version_test.go
package version

import "testing"

func TestString_NotEmpty(t *testing.T) {
	if String() == "" {
		t.Fatal("version.String() must not be empty")
	}
}
```

**Step 2: 运行测试，确认失败**

Run: `go test ./internal/version -v`
Expected: FAIL（`String` 未定义）

**Step 3: 写最小实现：version.String() 返回常量**

```go
// internal/version/version.go
package version

const Version = "0.0.0-dev"

func String() string { return Version }
```

**Step 4: 再跑测试，确认通过**

Run: `go test ./internal/version -v`
Expected: PASS

**Step 5: Commit**

```bash
git add go.mod internal/version cmd
git commit -m "chore(otidx): init Go module and version package"
```

---

### Task 2: CLI RootCommand（cobra）+ `-h/-v` 基础行为

**Files:**
- Create: `internal/otidxcli/root.go`
- Create: `internal/otidxcli/root_test.go`
- Modify: `cmd/otidx/main.go`

**Step 1: 写失败测试：--help 会输出 otidx 与 q 子命令**

```go
// internal/otidxcli/root_test.go
package otidxcli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpContainsSubcommand(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "otidx") || !strings.Contains(s, "q") {
		t.Fatalf("help missing expected text: %s", s)
	}
}
```

**Step 2: 运行测试，确认失败**

Run: `go test ./internal/otidxcli -v`
Expected: FAIL（`NewRootCommand` 未定义）

**Step 3: 写最小实现：root command + 预留全局 flags（先不实现 q）**

```go
// internal/otidxcli/root.go
package otidxcli

import (
	"github.com/spf13/cobra"
	"otterindex/internal/version"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "otidx",
		Short: "OtterIndex local index/search tool",
	}
	cmd.SetVersionTemplate("{{.Version}}\n")
	cmd.Version = version.String()
	return cmd
}
```

`cmd/otidx/main.go` 里只做 `os.Exit(NewRootCommand().Execute())`。

**Step 4: 再跑测试，确认通过**

Run: `go test ./internal/otidxcli -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/otidxcli cmd/otidx
git commit -m "feat(otidx): add cobra root command skeleton"
```

---

### Task 3: CLI 参数解析（对齐 -d/-A/-x/-g/-i/-c/-B/-L/-b/-z/-Z/-l/--jsonl/--explain/--viz/--unit）

**Files:**
- Create: `internal/otidxcli/options.go`
- Modify: `internal/otidxcli/root.go`
- Create: `internal/otidxcli/options_test.go`

**Step 1: 写失败测试：-c 默认 1，-x 支持逗号分隔，-g 可重复**

```go
// internal/otidxcli/options_test.go
package otidxcli

import "testing"

func TestParseDefaults(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"q", "hello"})
	_, opts, err := ExecuteForTest(cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if opts.ContextLines != 1 {
		t.Fatalf("ContextLines=%d", opts.ContextLines)
	}
}

func TestExcludeCSV(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"q", "k", "-x", "*.js,*.sql"})
	_, opts, err := ExecuteForTest(cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(opts.ExcludeGlobs) != 2 || opts.ExcludeGlobs[0] != "*.js" || opts.ExcludeGlobs[1] != "*.sql" {
		t.Fatalf("ExcludeGlobs=%v", opts.ExcludeGlobs)
	}
}
```

**Step 2: 运行测试，确认失败**

Run: `go test ./internal/otidxcli -v`
Expected: FAIL（`ExecuteForTest`/Options 未实现）

**Step 3: 最小实现 Options + q 子命令（先只解析，不执行搜索）**

- `Options` 包含：`DBPath, ScanAll, IncludeGlobs, ExcludeGlobs, CaseInsensitive, ContextLines, Unit, NoBanner, VimLines, Theme, Jsonl, Explain, Viz`
- `-b/-z/-Z` 映射到 `Theme`：`colorblind|none|high-contrast|default`
- `q` 子命令只把 Options 组装好并打印一行占位（测试里用 `ExecuteForTest` 拿到 opts）

**Step 4: 再跑测试**

Run: `go test ./internal/otidxcli -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/otidxcli
git commit -m "feat(otidx): parse flags for q/include/exclude/context/theme/unit"
```

---

### Task 4: 核心数据模型（Result/Position）+ 渲染（默认/-L/--jsonl）

**Files:**
- Create: `internal/model/types.go`
- Create: `internal/otidxcli/render.go`
- Create: `internal/otidxcli/render_test.go`

**Step 1: 写失败测试：--jsonl 每行是合法 JSON**

```go
// internal/otidxcli/render_test.go
package otidxcli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderJSONL(t *testing.T) {
	lines := RenderJSONL([]ResultItem{
		{Path: "a.go", Range: Range{SL: 1, SC: 1, EL: 2, EC: 1}},
	})
	for _, line := range strings.Split(strings.TrimSpace(lines), "\n") {
		var v any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("invalid json: %v (%s)", err, line)
		}
	}
}
```

**Step 2: 运行测试，确认失败**

Run: `go test ./internal/otidxcli -v`
Expected: FAIL（`ResultItem/RenderJSONL` 未定义）

**Step 3: 最小实现 model 与渲染**

- `ResultItem`：`Path + Range + Matches + Snippet + Title`
- `RenderDefault`：`path:line: snippet`
- `RenderVim`（`-L`）：`path:sl:sc: snippet`
- `RenderJSONL`：每行一个 JSON

**Step 4: 再跑测试**

Run: `go test ./internal/otidxcli -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/model internal/otidxcli
git commit -m "feat(otidx): result model + default/vim/jsonl rendering"
```

---

### Task 5: 文件枚举与过滤（不依赖 rg；支持 -g/-x/-A + .gitignore/.otidxignore）

**Files:**
- Create: `internal/core/walk/walk.go`
- Create: `internal/core/walk/walk_test.go`
- Create: `internal/core/walk/ignore.go`

**Step 1: 写失败测试：include/exclude 生效，返回相对路径**

```go
// internal/core/walk/walk_test.go
package walk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWalkIncludeExclude(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("x"), 0644)
	_ = os.WriteFile(filepath.Join(root, "a.sql"), []byte("x"), 0644)

	files, err := ListFiles(root, Options{
		IncludeGlobs: []string{"*.go"},
		ExcludeGlobs: []string{"*.sql"},
		ScanAll:      false,
	})
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "a.go" {
		t.Fatalf("files=%v", files)
	}
}
```

**Step 2: 运行测试，确认失败**

Run: `go test ./internal/core/walk -v`
Expected: FAIL

**Step 3: 最小实现 ListFiles**

- `filepath.WalkDir` 遍历
- 默认跳过：隐藏目录（`.` 开头）、常见大目录（`node_modules/.git/dist/target`）与二进制（按扩展/简单 sniff）
- `.gitignore` + `.otidxignore`：先只做“本目录文件 + 简单 pattern”（后续再完善层级合并）
- `-A`：跳过默认过滤与 ignore（ALL）
- 输出统一为相对 root 的 slash 风格（用于跨平台一致测试）

**Step 4: 再跑测试**

Run: `go test ./internal/core/walk -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/core/walk
git commit -m "feat(core): walk files with include/exclude and ignore support"
```

---

### Task 6: 纯扫描匹配（关键词→命中行号/列号；支持 -i）

**Files:**
- Create: `internal/core/search/search.go`
- Create: `internal/core/search/search_test.go`

**Step 1: 写失败测试：能返回 line/col**

```go
// internal/core/search/search_test.go
package search

import "testing"

func TestFindInText(t *testing.T) {
	ms := FindInText("x\nhello\nz\n", "hello", false)
	if len(ms) != 1 || ms[0].Line != 2 || ms[0].Col != 1 {
		t.Fatalf("matches=%v", ms)
	}
}
```

**Step 2: 运行测试，确认失败**

Run: `go test ./internal/core/search -v`
Expected: FAIL

**Step 3: 最小实现 FindInText**

- 按行扫描，`strings.Index` 找首次命中列（MVP 先单命中/行；后续可扩展多命中）
- `-i`：先用 `strings.ToLower` 的简单策略（ASCII 优先），并在注释里标出后续要换成更严格的 unicode case-fold

**Step 4: 再跑测试**

Run: `go test ./internal/core/search -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/core/search
git commit -m "feat(core): keyword scanning with line/col matches"
```

---

### Task 7: unitize（--unit line|block|file）返回最小单元块范围

**Files:**
- Create: `internal/core/unit/unit.go`
- Create: `internal/core/unit/unit_test.go`

**Step 1: 写失败测试：block 单元能扩展到括号块**

```go
// internal/core/unit/unit_test.go
package unit

import "testing"

func TestBlockUnit_Braces(t *testing.T) {
	text := "fn a() {\n  let x = 1;\n  // KEY\n  let y = 2;\n}\n"
	r := BlockRange(text, Match{Line: 3, Col: 3})
	if r.SL != 1 || r.EL != 5 {
		t.Fatalf("range=%+v", r)
	}
}
```

**Step 2: 运行测试，确认失败**

Run: `go test ./internal/core/unit -v`
Expected: FAIL

**Step 3: 最小实现 unitize**

- `line`：`Match.Line` 上下 `-c` 行（边界截断）
- `file`：`1..EOF`
- `block`（MVP 通用启发式）：
  - 先用 brace 平衡（向上找最近“可能的 `{` 开始”，向下找匹配 `}`）
  - 找不到则退回到“空行分隔块”（向上/向下直到空行）
  - 仍失败退回 `line`

**Step 4: 再跑测试**

Run: `go test ./internal/core/unit -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/core/unit
git commit -m "feat(core): unitize to line/block/file ranges"
```

---

### Task 8: SQLite schema + IndexStore（本地库；为 FTS 预留）

**Files:**
- Create: `internal/index/sqlite/schema.sql`
- Create: `internal/index/sqlite/store.go`
- Create: `internal/index/sqlite/store_test.go`

**Step 1: 写失败测试：能创建库并写入 files**

```go
// internal/index/sqlite/store_test.go
package sqlite

import "testing"

func TestCreateAndUpsertFile(t *testing.T) {
	dbPath := t.TempDir() + "/index.db"
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if err := s.UpsertFile("ws1", "a.go", 123, 1); err != nil {
		t.Fatalf("upsert: %v", err)
	}
}
```

**Step 2: 运行测试，确认失败**

Run: `go test ./internal/index/sqlite -v`
Expected: FAIL

**Step 3: 最小实现 schema + store**

`schema.sql`（MVP）建议：
- `workspaces(id TEXT PRIMARY KEY, root TEXT, created_at INTEGER)`
- `files(workspace_id TEXT, path TEXT, size INTEGER, mtime INTEGER, hash TEXT, PRIMARY KEY(workspace_id, path))`
- `chunks(id INTEGER PRIMARY KEY, workspace_id TEXT, path TEXT, sl INTEGER, el INTEGER, kind TEXT, title TEXT, text TEXT)`
- （可选）`chunks_fts`：尝试创建 `fts5(text, path UNINDEXED, content='chunks', content_rowid='id')`，失败则记录 capability 并回退到 LIKE

驱动选择（MVP）：`modernc.org/sqlite`（纯 Go）打开 db。

**Step 4: 再跑测试**

Run: `go test ./internal/index/sqlite -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/index/sqlite
git commit -m "feat(index): sqlite schema + store (pure Go) with optional FTS"
```

---

### Task 9: IndexBuilder（扫描→分块→写入 SQLite）与 `otidx index build`

**Files:**
- Create: `internal/core/indexer/indexer.go`
- Create: `internal/core/indexer/indexer_test.go`
- Modify: `internal/otidxcli/root.go`
- Create: `internal/otidxcli/index_cmd.go`

**Step 1: 写失败测试：对临时目录 build 后能查到 chunk**

```go
// internal/core/indexer/indexer_test.go
package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildWritesChunks(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("hello\nworld\n"), 0644)
	dbPath := filepath.Join(root, "index.db")

	if err := Build(root, dbPath, Options{}); err != nil {
		t.Fatalf("build: %v", err)
	}
	// 这里用 store 的 CountChunks()/SearchChunks() 断言（下一步实现）
}
```

**Step 2: 运行测试，确认失败**

Run: `go test ./internal/core/indexer -v`
Expected: FAIL

**Step 3: 最小实现 Build**

- walk 列出文件
- 对每个文件：读文本 → 用“固定窗口 chunk”（例如 40 行，overlap 10）生成 `chunks(sl,el,text)`
- 写入 sqlite：UpsertFile + ReplaceChunks(path)

**Step 4: 再跑测试**

Run: `go test ./internal/core/indexer -v`
Expected: PASS（或至少能检查 chunks 数量 > 0）

**Step 5: Commit**

```bash
git add internal/core/indexer internal/otidxcli
git commit -m "feat(otidx): index build command writes chunks into sqlite"
```

---

### Task 10: `otidx q` 走 SQLite 查询 + unitize 输出（不再全量扫描）

**Files:**
- Create: `internal/core/query/query.go`
- Create: `internal/core/query/query_test.go`
- Modify: `internal/otidxcli/root.go`
- Create: `internal/otidxcli/q_cmd.go`

**Step 1: 写失败测试：build → q 能返回 path+range**

```go
// internal/core/query/query_test.go
package query

import "testing"

func TestQueryReturnsRanges(t *testing.T) {
	// 预置：用 indexer.Build 生成 db，再 Query(db,q,unit)
	// 断言返回 ResultItem.Path 非空，Range.SL/EL > 0
	_ = testing.T{}
}
```

**Step 2: 运行测试，确认失败**

Run: `go test ./internal/core/query -v`
Expected: FAIL

**Step 3: 最小实现 Query**

- `chunks_fts` 可用：`SELECT id,path,sl,el,highlight(text,0,'<<','>>') ... WHERE chunks_fts MATCH ?`
- 不可用：`WHERE text LIKE '%'||?||'%'`（慢但可用）
- 命中后：根据 `--unit` 决定输出：
  - `block`：直接返回 chunk 的 `sl..el`
  - `line`：在 chunk text 内定位命中行（FindInText），返回 `line±c`
  - `file`：返回 `1..EOF`（读取文件行数；或用 chunk 退化）

**Step 4: 再跑测试**

Run: `go test ./internal/core/query -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/core/query internal/otidxcli
git commit -m "feat(otidx): query sqlite index and unitize results"
```

---

### Task 11: `--explain/--viz ascii`（可解释调试输出）

**Files:**
- Create: `internal/otidxcli/explain.go`
- Create: `internal/otidxcli/viz.go`
- Create: `internal/otidxcli/viz_test.go`

**Step 1: 写失败测试：viz 输出包含管线节点**

```go
// internal/otidxcli/viz_test.go
package otidxcli

import (
	"strings"
	"testing"
)

func TestVizASCII(t *testing.T) {
	s := VizASCII()
	for _, want := range []string{"walk", "index", "query", "unitize", "render"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in %s", want, s)
		}
	}
}
```

**Step 2: 运行测试，确认失败**

Run: `go test ./internal/otidxcli -v`
Expected: FAIL

**Step 3: 最小实现**

- `--explain`：stderr 打印：dbPath、FTS 是否可用、过滤器、命中数、unit 决策
- `--viz ascii`：输出固定 ASCII 图

**Step 4: 再跑测试**

Run: `go test ./internal/otidxcli -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/otidxcli
git commit -m "feat(otidx): explain and ascii viz for debugging"
```

---

### Task 12: `otidxd` daemon 骨架（JSON-RPC over TCP）+ ping/version

**Files:**
- Create: `internal/otidxd/server.go`
- Create: `internal/otidxd/protocol.go`
- Create: `internal/otidxd/server_test.go`
- Modify: `cmd/otidxd/main.go`

**Step 1: 写失败测试：daemon 能启动并响应 ping**

```go
// internal/otidxd/server_test.go
package otidxd

import (
	"testing"
	"time"
)

func TestServerStarts(t *testing.T) {
	s := NewServer(Options{Listen: "127.0.0.1:0"})
	go func() { _ = s.Run() }()
	time.Sleep(200 * time.Millisecond)
	_ = s.Close()
}
```

**Step 2: 运行测试，确认失败**

Run: `go test ./internal/otidxd -v`
Expected: FAIL

**Step 3: 最小实现 server**

- `Run()` 监听 TCP
- 协议先用最小 JSON-RPC 2.0（`{"jsonrpc":"2.0","method":"ping","id":1}`）
- 支持 `ping`、`version`（返回 `version.String()`）

**Step 4: 再跑测试**

Run: `go test ./internal/otidxd -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/otidxd cmd/otidxd
git commit -m "feat(otidxd): minimal json-rpc server with ping/version"
```

---

## 后续（C 扩展预留点，不在 MVP 里做）

### Task X1: SQLite 驱动切换（cgo 提升能力）

- 增加 `internal/index/sqlite/driver_purego.go`（默认 `modernc.org/sqlite`）
- 增加 `internal/index/sqlite/driver_cgo.go`（`//go:build cgo`，使用 `github.com/mattn/go-sqlite3`）
- 在 cgo 版中启用/验证：FTS5 compile options、自定义 tokenizer（需要 C 工具链）

### Task X2: `--unit symbol`（tree-sitter）

- 推荐使用 `github.com/tree-sitter/go-tree-sitter` 作为 Go bindings（项目本身涉及 CGO/C 内存）
- Grammar 载入策略二选一：
  - **编译期内置（默认）**：直接依赖 `github.com/tree-sitter/tree-sitter-<lang>/bindings/go`（更符合“关闭动态加载”，但会增加体积/编译成本）
  - **运行时加载（可选）**：按 `go-tree-sitter` README 的思路，用 `purego` 从共享库加载 grammar（Linux/macOS 先行；Windows 需 DLL 分发与加载策略）
- Build tags 建议：
  - `//go:build treesitter && cgo`：启用 symbol unit（需要 C 工具链）
  - 未启用时：`--unit symbol` 自动降级为 `block` 并在 `--explain` 标明原因
- 资源管理注意：凡是从 C 分配内存的对象必须显式 `Close()`（例如 Parser/Tree/Query/QueryCursor 等；避免依赖 finalizer）
- `symbol` unit 行为：对命中点（path+line+col）定位到最小包含的函数/方法/类节点，输出其 `Range(SL,SC,EL,EC)` 并携带适当 snippet
- 兼容策略：解析失败/语言不支持时退回 `block`（始终保证可用）
