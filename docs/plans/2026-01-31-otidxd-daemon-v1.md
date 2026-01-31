# Otidxd Daemon V1 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 把 `otidxd` 扩展为可用的后台守护进程：支持 workspace 注册、索引构建、查询，以及文件监听增量更新；并提供最小 client 方便脚本/CLI 调用与回归测试。

**Architecture:** `otidxd` 继续使用 **TCP + JSON-RPC 2.0**（一请求一行 JSON、一响应一行 JSON）。核心逻辑集中在 `internal/otidxd/Handlers`。新增 watcher 管理（基于 `internal/core/watch` + `indexer.UpdateFile`）以在文件变化时增量更新 SQLite 索引。提供 `internal/otidxd/Client` 封装 RPC 调用，用于测试和未来 CLI 对接。

**Tech Stack:** Go、`net`/`bufio`/`encoding/json`、`github.com/fsnotify/fsnotify`、SQLite（`internal/index/sqlite`）、索引与查询（`internal/core/indexer` / `internal/core/query`）、（可选）tree-sitter（`-tags treesitter` + CGO）。

---

## 执行前说明（强烈推荐）

- 推荐在独立 `git worktree` / 分支中实现（避免污染主分支），并按任务频繁提交（每个 task 一个 commit）。
- 本计划默认用 PowerShell（pwsh）命令示例。

### Task 0: 准备 worktree（推荐）

**Files:** 无

**Step 1: 创建 worktree + 分支**

运行：

```powershell
git worktree add ..\worktrees\otidxd-daemon-v1 -b feat/otidxd-daemon-v1
Set-Location -LiteralPath ..\worktrees\otidxd-daemon-v1
```

期望：生成新目录并切到该目录，`git status` 显示在新分支。

---

## 现状速览（避免重复造轮子）

- server：`internal/otidxd/server.go` 已支持 `ping`、`version`、`workspace.add`、`index.build`、`query`（TCP JSON-RPC 一行一包）。
- handler：`internal/otidxd/handlers.go` 已接入 `indexer.Build` 和 `query.Query`，并内置 query cache + session store（对 daemon 模式更有意义）。
- watch：`internal/core/watch/watcher.go` 已实现 fsnotify + debounce + `indexer.UpdateFile` 的增量更新能力，但目前未接入 `otidxd` RPC。

---

### Task 1: 增加 `internal/otidxd` JSON-RPC Client（端到端可测）

**Files:**
- Create: `internal/otidxd/client.go`
- Test: `internal/otidxd/client_test.go`

**Step 1: 写一个端到端失败测试（先让它编译失败）**

创建 `internal/otidxd/client_test.go`：

```go
package otidxd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClient_MinLoop(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("hello\nworld\n"), 0o644)

	s := NewServer(Options{Listen: "127.0.0.1:0"})
	errCh := make(chan error, 1)
	go func() { errCh <- s.Run() }()
	addr := waitAddr(t, s, time.Second)
	t.Cleanup(func() { _ = s.Close() })

	c, err := Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if err := c.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if v, err := c.Version(); err != nil || v == "" {
		t.Fatalf("version=%q err=%v", v, err)
	}

	wsid, err := c.WorkspaceAdd(WorkspaceAddParams{Root: root})
	if err != nil || wsid == "" {
		t.Fatalf("workspace.add wsid=%q err=%v", wsid, err)
	}

	if _, err := c.IndexBuild(IndexBuildParams{WorkspaceID: wsid}); err != nil {
		t.Fatalf("index.build: %v", err)
	}

	items, err := c.Query(QueryParams{WorkspaceID: wsid, Q: "hello", Unit: "block", Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(items) == 0 || items[0].Path != "a.go" {
		t.Fatalf("bad items: %+v", items)
	}
}
```

**Step 2: 跑测试确认失败**

运行：

```powershell
go test ./internal/otidxd -run TestClient_MinLoop -v
```

期望：FAIL（提示 `Dial`/`Client` 未定义）。

**Step 3: 写最小实现（Client + Dial + 5 个方法）**

创建 `internal/otidxd/client.go`（最小可行版要点）：

```go
package otidxd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

type RPCError struct {
	Code    int
	Message string
}

func (e *RPCError) Error() string { return fmt.Sprintf("rpc error (%d): %s", e.Code, e.Message) }

type Client struct {
	conn   net.Conn
	r      *bufio.Reader
	w      *bufio.Writer
	nextID int64
}

func Dial(addr string) (*Client, error) {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn: conn,
		r:    bufio.NewReader(conn),
		w:    bufio.NewWriter(conn),
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

type rawResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ErrorObject    `json:"error,omitempty"`
}

func (c *Client) call(method string, params any, out any) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("client is nil")
	}
	id := atomic.AddInt64(&c.nextID, 1)
	req := Request{JSONRPC: "2.0", Method: method, ID: json.RawMessage(fmt.Sprintf("%d", id))}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		req.Params = b
	}

	if err := WriteOneLine(c.w, req); err != nil {
		return err
	}
	if err := c.w.Flush(); err != nil {
		return err
	}

	line, err := ReadOneLine(c.r)
	if err != nil {
		return err
	}
	var resp rawResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return err
	}
	if resp.Error != nil {
		return &RPCError{Code: resp.Error.Code, Message: resp.Error.Message}
	}
	if out == nil || len(resp.Result) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Result, out)
}

func (c *Client) Ping() error {
	var out string
	if err := c.call("ping", nil, &out); err != nil {
		return err
	}
	if out != "pong" {
		return fmt.Errorf("unexpected ping result: %q", out)
	}
	return nil
}

func (c *Client) Version() (string, error) {
	var out string
	if err := c.call("version", nil, &out); err != nil {
		return "", err
	}
	return out, nil
}

func (c *Client) WorkspaceAdd(p WorkspaceAddParams) (string, error) {
	var out string
	if err := c.call("workspace.add", p, &out); err != nil {
		return "", err
	}
	return out, nil
}

func (c *Client) IndexBuild(p IndexBuildParams) (any, error) {
	var out any
	if err := c.call("index.build", p, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Query(p QueryParams) ([]ResultItem, error) {
	var out []ResultItem
	if err := c.call("query", p, &out); err != nil {
		return nil, err
	}
	return out, nil
}
```

**Step 4: 跑测试确认通过**

运行：

```powershell
go test ./internal/otidxd -run TestClient_MinLoop -v
```

期望：PASS。

**Step 5: Commit**

```powershell
git add internal/otidxd/client.go internal/otidxd/client_test.go
git commit -m "feat(otidxd): add jsonrpc client"
```

---

### Task 2: 增加 watch RPC：`watch.start` / `watch.stop` / `watch.status`

**Files:**
- Modify: `internal/otidxd/protocol.go`
- Modify: `internal/otidxd/server.go`
- Modify: `internal/otidxd/handlers.go`
- Test: `internal/otidxd/server_test.go`（或新建 `internal/otidxd/watch_rpc_test.go`）

**Step 1: 写失败测试：调用 `watch.start` 应该成功（当前会 method not found）**

在 `internal/otidxd/server_test.go` 旁新增测试文件（例如 `internal/otidxd/watch_rpc_test.go`）：

```go
package otidxd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServer_WatchStartStopStatus(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("hello\n"), 0o644)

	s := NewServer(Options{Listen: "127.0.0.1:0"})
	go func() { _ = s.Run() }()
	addr := waitAddr(t, s, time.Second)
	t.Cleanup(func() { _ = s.Close() })

	c, err := Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	wsid, err := c.WorkspaceAdd(WorkspaceAddParams{Root: root})
	if err != nil {
		t.Fatalf("workspace.add: %v", err)
	}
	if _, err := c.IndexBuild(IndexBuildParams{WorkspaceID: wsid}); err != nil {
		t.Fatalf("index.build: %v", err)
	}

	var st WatchStatusResult
	if err := c.call("watch.status", WatchStatusParams{WorkspaceID: wsid}, &st); err != nil {
		t.Fatalf("watch.status: %v", err)
	}
	if st.Running {
		t.Fatalf("expected not running at start")
	}

	if err := c.call("watch.start", WatchStartParams{WorkspaceID: wsid}, &st); err != nil {
		t.Fatalf("watch.start: %v", err)
	}
	if !st.Running {
		t.Fatalf("expected running after start")
	}

	if err := c.call("watch.stop", WatchStopParams{WorkspaceID: wsid}, &st); err != nil {
		t.Fatalf("watch.stop: %v", err)
	}
	if st.Running {
		t.Fatalf("expected stopped")
	}
}
```

**Step 2: 跑测试确认失败**

运行：

```powershell
go test ./internal/otidxd -run TestServer_WatchStartStopStatus -v
```

期望：FAIL（`method not found`）。

**Step 3: 最小实现：补齐协议类型 + server dispatch + handler 入口（先只做状态机）**

1) 修改 `internal/otidxd/protocol.go` 增加参数/返回：

```go
type WatchStartParams struct {
	WorkspaceID  string   `json:"workspace_id"`
	ScanAll      bool     `json:"scan_all,omitempty"`
	IncludeGlobs []string `json:"include_globs,omitempty"`
	ExcludeGlobs []string `json:"exclude_globs,omitempty"`
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
```

2) 修改 `internal/otidxd/handlers.go`：在 `Handlers` 里加 watcher 状态（先用 map 记录 running）并实现：

```go
func (h *Handlers) WatchStart(p WatchStartParams) (WatchStatusResult, error)
func (h *Handlers) WatchStop(p WatchStopParams) (WatchStatusResult, error)
func (h *Handlers) WatchStatus(p WatchStatusParams) (WatchStatusResult, error)
```

最小行为：
- `WatchStatus`：workspace 不存在 => error；存在但未 start => `{Running:false}`
- `WatchStart`：若已 running => `{Running:true}`；否则设置 running=true
- `WatchStop`：若未 running => `{Running:false}`；否则设置 running=false

3) 修改 `internal/otidxd/server.go` 的 `dispatch` switch 增加 3 个 method，复用现有参数校验模式（空 `workspace_id` 返回 -32602）。

**Step 4: 跑测试确认通过**

```powershell
go test ./internal/otidxd -run TestServer_WatchStartStopStatus -v
```

期望：PASS。

**Step 5: Commit**

```powershell
git add internal/otidxd/protocol.go internal/otidxd/server.go internal/otidxd/handlers.go internal/otidxd/watch_rpc_test.go
git commit -m "feat(otidxd): add watch rpc (skeleton)"
```

---

### Task 3: 接入真实 fsnotify watcher：文件变化 → 增量更新索引

**Files:**
- Modify: `internal/otidxd/handlers.go`
- Modify: `internal/otidxd/server.go`（可选：关闭时清理 watcher）
- Test: `internal/otidxd/watch_integration_test.go`

**Step 1: 写失败测试：watch 启动后修改文件，新的关键词应可被 query 命中**

新增 `internal/otidxd/watch_integration_test.go`：

```go
package otidxd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWatch_UpdatesIndexOnFileChange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	_ = os.WriteFile(path, []byte("hello\n"), 0o644)

	s := NewServer(Options{Listen: "127.0.0.1:0"})
	go func() { _ = s.Run() }()
	addr := waitAddr(t, s, time.Second)
	t.Cleanup(func() { _ = s.Close() })

	c, err := Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	wsid, err := c.WorkspaceAdd(WorkspaceAddParams{Root: root})
	if err != nil {
		t.Fatalf("workspace.add: %v", err)
	}
	if _, err := c.IndexBuild(IndexBuildParams{WorkspaceID: wsid}); err != nil {
		t.Fatalf("index.build: %v", err)
	}
	var st WatchStatusResult
	if err := c.call("watch.start", WatchStartParams{WorkspaceID: wsid}, &st); err != nil {
		t.Fatalf("watch.start: %v", err)
	}
	if !st.Running {
		t.Fatalf("expected running")
	}

	needle := "NEW_TOKEN_123"
	_ = os.WriteFile(path, []byte("hello\n"+needle+"\n"), 0o644)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		items, err := c.Query(QueryParams{WorkspaceID: wsid, Q: needle, Unit: "block", Limit: 10, Offset: 0})
		if err == nil && len(items) > 0 {
			if items[0].Path == "a.go" && strings.Contains(items[0].Snippet, needle) || needle != "" {
				return
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout: did not observe updated index for %q", needle)
}
```

**Step 2: 跑测试确认失败**

```powershell
go test ./internal/otidxd -run TestWatch_UpdatesIndexOnFileChange -v
```

期望：FAIL（因为当前 watch 还是 skeleton，不会触发增量更新）。

**Step 3: 最小实现：在 Handler 里真正启动 `internal/core/watch.Watcher`**

实现要点（在 `internal/otidxd/handlers.go`）：

- 在 `Handlers` 里增加：
  - `watchers map[string]*watcherEntry`
  - `type watcherEntry struct { w *watch.Watcher; cancel context.CancelFunc; done chan struct{} }`
- `WatchStart`：
  - 校验 workspace 存在
  - 若已存在 entry 且未退出，直接返回 Running=true
  - 创建 `watch.NewWatcher(ws.root, ws.dbPath, indexer.Options{ WorkspaceID: p.WorkspaceID, ScanAll: p.ScanAll, IncludeGlobs: p.IncludeGlobs, ExcludeGlobs: p.ExcludeGlobs })`
  - `ctx, cancel := context.WithCancel(context.Background())`
  - `done := make(chan struct{})`；`go func(){ _ = w.Run(ctx); close(done) }()`
  - 保存到 map
- `WatchStop`：
  - cancel + w.Close()
  - 从 map 删除（或标记 stopped）
- `WatchStatus`：map 里存在且未退出 => Running=true

建议同时加一个 `func (h *Handlers) Close() error`，关闭所有 watcher；并在 `Server.Close()` 中调用它，避免资源泄露（对长期运行 daemon 很关键）。

**Step 4: 跑测试确认通过**

```powershell
go test ./internal/otidxd -run TestWatch_UpdatesIndexOnFileChange -v
```

期望：PASS（如果偶尔抖动，先把 deadline 拉到 5s，再排查 fsnotify 事件是否未触发）。

**Step 5: Commit**

```powershell
git add internal/otidxd/handlers.go internal/otidxd/server.go internal/otidxd/watch_integration_test.go
git commit -m "feat(otidxd): watch files and update index"
```

---

### Task 4: `query` RPC 增加 `show` 参数（返回 `ResultItem.text`）

**Files:**
- Modify: `internal/otidxd/protocol.go`
- Modify: `internal/otidxd/handlers.go`
- Test: `internal/otidxd/query_show_test.go`

**Step 1: 写失败测试：show=true 时 result item 里应包含 text**

新增 `internal/otidxd/query_show_test.go`：

```go
package otidxd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuery_ShowAttachesText(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("hello\nworld\n"), 0o644)

	h := NewHandlers()
	wsid, err := h.WorkspaceAdd(WorkspaceAddParams{Root: root})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := h.IndexBuild(IndexBuildParams{WorkspaceID: wsid}); err != nil {
		t.Fatalf("build: %v", err)
	}

	items, err := h.Query(QueryParams{WorkspaceID: wsid, Q: "hello", Unit: "block", Limit: 10, Offset: 0, Show: true})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(items) == 0 || !strings.Contains(items[0].Text, "hello") {
		t.Fatalf("expected text attached: %+v", items)
	}
}
```

**Step 2: 跑测试确认失败**

```powershell
go test ./internal/otidxd -run TestQuery_ShowAttachesText -v
```

期望：FAIL（`Show` 字段不存在/或者 text 为空）。

**Step 3: 最小实现**

1) 修改 `internal/otidxd/protocol.go`：

```go
type QueryParams struct {
	// ... existing fields ...
	Show bool `json:"show,omitempty"`
}
```

2) 修改 `internal/otidxd/handlers.go`：在 `Query(p QueryParams)` 里，如果 `p.Show` 为 true：
- 用 `ws.root` + `items[i].Path` 读文件
- 按 `items[i].Range.SL..EL` 切片出对应行
- 写入 `items[i].Text`

推荐复用 `internal/otidxcli/show.go` 的逻辑，但不要 import CLI 包；最小实现可在 `internal/otidxd` 内新增一个私有 helper（例如 `attachText(workspaceRoot string, items []model.ResultItem)`）。

**Step 4: 跑测试确认通过**

```powershell
go test ./internal/otidxd -run TestQuery_ShowAttachesText -v
```

期望：PASS。

**Step 5: Commit**

```powershell
git add internal/otidxd/protocol.go internal/otidxd/handlers.go internal/otidxd/query_show_test.go
git commit -m "feat(otidxd): query show attaches text"
```

---

### Task 5: `index.build` 返回版本号（便于 client/IDE 判断缓存失效）

**Files:**
- Modify: `internal/otidxd/handlers.go`
- Test: `internal/otidxd/index_build_version_test.go`

**Step 1: 写失败测试：index.build 返回 int64 version**

新增 `internal/otidxd/index_build_version_test.go`：

```go
package otidxd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIndexBuild_ReturnsVersion(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("hello\n"), 0o644)

	h := NewHandlers()
	wsid, err := h.WorkspaceAdd(WorkspaceAddParams{Root: root})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	v, err := h.IndexBuild(IndexBuildParams{WorkspaceID: wsid})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if vv, ok := v.(int64); !ok || vv <= 0 {
		t.Fatalf("expected int64 version > 0, got=%T %#v", v, v)
	}
}
```

**Step 2: 跑测试确认失败**

```powershell
go test ./internal/otidxd -run TestIndexBuild_ReturnsVersion -v
```

期望：FAIL（当前返回 true）。

**Step 3: 最小实现：build 后读取 sqlite version**

修改 `internal/otidxd/handlers.go` 的 `IndexBuild`：
- `indexer.Build(...)` 成功后
- `sqlite.Open(ws.dbPath)`，`GetVersion(p.WorkspaceID)`，返回该 version（int64）

**Step 4: 跑测试确认通过**

```powershell
go test ./internal/otidxd -run TestIndexBuild_ReturnsVersion -v
```

期望：PASS。

**Step 5: Commit**

```powershell
git add internal/otidxd/handlers.go internal/otidxd/index_build_version_test.go
git commit -m "feat(otidxd): index.build returns workspace version"
```

---

### Task 6: 更新 README + 增加 PowerShell smoke 脚本（手动调试友好）

**Files:**
- Modify: `README.md`
- Create: `scripts/otidxd-smoke.ps1`

**Step 1: README 对齐当前/新增 RPC**

更新 `README.md` 的 `otidxd` 章节：
- 列出方法：`ping`、`version`、`workspace.add`、`index.build`、`query`、`watch.start`、`watch.stop`、`watch.status`
- 说明 `query` 的参数（特别是 `show`）与默认值策略（`limit/offset/unit/context_lines`）
- 给出一段最小交互示例（含 request/response）

**Step 2: 新增 `scripts/otidxd-smoke.ps1`**

目标：不用额外工具，在 PowerShell 里用 `System.Net.Sockets.TcpClient` 发送 JSON 行并读取响应，便于快速复现问题。

脚本建议参数：
- `-Listen`（默认 `127.0.0.1:7337`）
- `-Root`（workspace root）
- `-Query`（默认 `hello`）

**Step 3: 手动验证脚本**

运行（示例）：

```powershell
go run ./cmd/otidxd -listen 127.0.0.1:7337
pwsh -NoProfile -File scripts/otidxd-smoke.ps1 -Root . -Query maybePrintViz
```

期望：脚本输出包含 `workspace_id`、`index.build` 返回 version、`query` 返回 items（可选 show）。

**Step 4: 运行全量测试**

```powershell
go test ./...
```

期望：PASS。

**Step 5: Commit**

```powershell
git add README.md scripts/otidxd-smoke.ps1
git commit -m "docs: document otidxd rpc and add smoke script"
```

---

## 可选任务（先别做，除非你明确需要）

### Optional A: `otidx` 增加 `daemon` 子命令（用 Client 访问 otidxd）

动机：让用户无需手写 JSON-RPC，就能 `otidx daemon query ...`。

建议命令：
- `otidx daemon ping --listen 127.0.0.1:7337`
- `otidx daemon workspace add --root .`
- `otidx daemon index build --workspace-id ...`
- `otidx daemon q --workspace-id ... <query...>`

如果要做：
- Create: `internal/otidxcli/daemon_cmd.go`
- Modify: `internal/otidxcli/root.go`（`cmd.AddCommand(newDaemonCommand())`）
- Test: `internal/otidxcli/daemon_cmd_test.go`

---

## 收尾验收清单（Definition of Done）

- `go test ./...` 全绿（至少在 Windows 上）。
- `otidxd` 支持 watch 增量更新，并能在 3s 内反映到 `query`。
- `query(show=true)` 能返回 `ResultItem.text`。
- README 的 `otidxd` 章节与实现一致，且提供可复制的调试示例。

