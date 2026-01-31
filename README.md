# OtterIndex（`otidx` / `otidxd`）

本项目提供一个 **本地** 的代码/文本索引与查询工具：

- `otidx`：命令行索引/查询（索引落到本地 SQLite）
- `otidxd`：daemon 骨架（TCP JSON-RPC：`ping`/`version`）

> 设计目标：根据关键词，返回“尽可能小的上下文单元块”，并带上文件相对路径 + 行号信息，方便携带上下文做进一步处理。

---

## 安装与构建

要求：已安装 Go（版本以 `go.mod` 为准）。

在仓库根目录：

```powershell
# 直接运行（不产物）
go run ./cmd/otidx --help

# 构建二进制（Windows 示例）
go build -o otidx.exe ./cmd/otidx
go build -o otidxd.exe ./cmd/otidxd
```

---

## 快速上手

### 1）在项目里建索引

```powershell
# 在当前目录（建议是你的 workspace 根）构建索引
go run ./cmd/otidx index build .
```

默认数据库路径为：`.otidx/index.db`（相对当前工作目录）。

### 2）关键词查询

```powershell
go run ./cmd/otidx q "keyword"
```

默认输出格式：`path:line: snippet`（路径为相对路径）。

---

## 为什么 OtterIndex（我们强在哪里）

- **更“贴近代码单元”**：不是只吐一行命中，而是给你一个可控的最小上下文单元块（`--unit line|block|file`），并带 `range {sl,el}` 方便继续取上下文。
- **定位信息完整**：默认输出 `path:line`，`-L` 输出 `path:line:col`，`--jsonl` 输出 `range`（行号范围）+ `matches`（命中位置）。
- **索引一次，多次查询**：`index build` 把内容落到本地 SQLite（可用则启用 FTS5），后续 `q` 不再全量遍历文件树，查询更快。
- **过滤与忽略更符合工程习惯**：支持 `-g/-x/-A`，默认按 `.gitignore` 语义过滤，并跳过 `.git/node_modules/dist/target` 与隐藏文件。
- **对脚本/Agent 友好**：`--jsonl` 适合直接喂给脚本；`--explain/--viz ascii` 方便调试与可解释输出；`otidxd` 预留给 IDE/Agent 的 RPC 接入。

---

## 真实输出示例（在本仓库跑出来的）

> 注：下面示例的 workspace 路径是 `D:\xyad\codegrep`，换机器会不同；但相对路径/行号/输出格式一致。

### 1）构建索引（带调试信息）

`--explain` 与 `--viz` 写入 stderr。

```powershell
> .\.otidx\bin\otidx.exe index build . --explain --viz ascii 2>&1
pipeline:

  walk   ->  index   ->  query   ->  unitize  ->  render
explain:
  action: index build
  root: D:\xyad\codegrep
  db: .otidx\index.db
  fts: true
  chunks: 273
```

### 2）默认查询输出（path:line: snippet）

```powershell
> .\.otidx\bin\otidx.exe q "maybePrintViz"
internal/otidxcli/explain.go:10: func maybePrintViz(cmd *cobra.Command) {
internal/otidxcli/index_cmd.go:30: maybePrintViz(cmd)
internal/otidxcli/q_cmd.go:24: maybePrintViz(cmd)
internal/otidxcli/root.go:33: maybePrintViz(cmd)
```

### 3）Vim 友好输出（path:line:col: snippet）

```powershell
> .\.otidx\bin\otidx.exe q "maybePrintViz" -L
internal/otidxcli/explain.go:10:6: func maybePrintViz(cmd *cobra.Command) {
internal/otidxcli/index_cmd.go:30:4: maybePrintViz(cmd)
internal/otidxcli/q_cmd.go:24:4: maybePrintViz(cmd)
internal/otidxcli/root.go:33:5: maybePrintViz(cmd)
```

### 4）JSONL 输出（带 range：可直接拿去取“最小上下文块”）

`--unit line -c 2`：返回命中行上下 2 行的范围（`sl..el`）。

```powershell
> .\.otidx\bin\otidx.exe q "maybePrintViz" --jsonl --unit line -c 2 | Select-Object -First 1
{"kind":"unit","path":"internal/otidxcli/explain.go","range":{"sl":8,"sc":1,"el":12,"ec":1},"snippet":"func maybePrintViz(cmd *cobra.Command) {","matches":[{"line":10,"col":6,"text":"func maybePrintViz(cmd *cobra.Command) {"}]}
```

拿到 `path + range.sl/range.el` 后，你可以在本地直接取出对应代码块：

```powershell
# 示例：取出 internal/otidxcli/explain.go 的 8..12 行
Get-Content -LiteralPath .\internal\otidxcli\explain.go |
  Select-Object -Skip 7 -First 5
```

---

## 性能（实测）

> 实测环境：在本仓库（`chunks: 273`、`fts: true`）用已构建的 `otidx.exe` 运行（不是 `go run`）。不同机器/仓库规模会有差异。

- 构建索引：约 `227ms`（`otidx index build .`）
- 执行一次查询：约 `84ms`（`otidx q "maybePrintViz"`）

## 常用参数（对齐 mgrep 风格）

### 数据库

- `-d <dbname|path>`：指定数据库。
  - 如果传的是 **dbname**（不含 `/\:`），会自动落到 `.otidx/<dbname>.db`  
    例：`-d demo` → `.otidx/demo.db`
  - 如果传的是路径（如 `D:\x\y.db` 或 `./x/y.db`），则直接使用该路径
- `-l`：列出当前目录下 `.otidx/*.db`

### 扫描/过滤（用于 `index build`；也会用于 `q` 的结果二次过滤）

- `-A`：扫描 ALL（不跳过隐藏文件/默认目录；不使用 `.gitignore` 过滤）
- `-g <glob>`：只包含这些文件（可重复）
  - 例：`-g "*.go" -g "docs/*.md"`
- `-x <glob>`：排除这些文件（支持逗号分隔或重复）
  - 例：`-x "*.js,*.sql"` 或 `-x "*.js" -x "*.sql"`

忽略规则（默认）：

- 使用 `.gitignore`（语义由 go-git 实现，含嵌套 `.gitignore`）
- 默认跳过目录：`.git` / `node_modules` / `dist` / `target`
- 默认跳过隐藏文件（以 `.` 开头）

### 查询

- `-i`：大小写不敏感（用于文本定位/LIKE 回退等）
- `--unit <line|block|file>`：返回力度（默认 `block`）
  - `block`：返回索引 chunk 的行号范围（目前 chunk 默认按 40 行切分）
  - `line`：返回命中行上下文（受 `-c` 影响）
  - `file`：返回整文件范围（如果能拿到 workspace root 则计算到 EOF）
- `-c <num>`：上下文行数（默认 1；仅 `--unit line` 生效）

### 输出

- `-L`：vim 友好行：`path:line:col: snippet`
- `--jsonl`：每行一个 JSON（包含 `range {sl,sc,el,ec}`，适合脚本/agent）
- `--explain`：在 stderr 输出执行信息（db、过滤条件、命中数、unit 决策等）
- `--viz ascii`：在 stderr 打印固定的 ASCII 管线图（调试）

> `-B/-b/-z/-Z` 目前作为预留参数：用于将来 banner/主题/颜色相关输出（当前版本暂未做彩色渲染）。

---

## 示例

### 只查 Go 文件，返回命中行上下文

```powershell
go run ./cmd/otidx q "NewRootCommand" --unit line -c 2 -g "*.go"
```

### 输出 JSONL（带 range）

```powershell
go run ./cmd/otidx q "sqlite" --jsonl
```

### 调试：查看 explain + 管线图

```powershell
go run ./cmd/otidx q "index" --explain --viz ascii
```

---

## `otidxd`（daemon）最小用法

启动：

```powershell
go run ./cmd/otidxd -listen 127.0.0.1:7337
```

协议：TCP JSON-RPC 2.0，一条请求一行 JSON（服务端按 JSON 解码）。

示例请求：

```json
{"jsonrpc":"2.0","method":"ping","id":1}
{"jsonrpc":"2.0","method":"version","id":2}
```

当前仅实现 `ping` / `version`，后续可扩展为：workspace 注册、索引构建、查询接口等。

---

## 说明与限制（MVP）

- 需要先 `otidx index build` 生成 SQLite 索引；目前不做增量更新/监听，文件变更后建议重建索引。
- SQLite FTS5 **可用则用**，不可用会自动回退到 `LIKE`（速度较慢但可用）。
- “最小代码单元块”当前以 `--unit` 控制（`line/block/file`）；`symbol`（tree-sitter）预留在后续阶段实现。
