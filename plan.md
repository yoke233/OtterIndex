下面给你一份**可直接开工的开发计划 + 技术文档（Go 版本）**，目标是：

- **继承 DocQL 的语法与查询模型**（DocQL 风格结构查询）
- 做出 **rg++ 产品形态：CLI + daemon + 协议（IDE/Agent 可调用）**
- MVP 优先跑通：**语法解析 → 执行计划 → 索引/解析器 → daemon 服务化 → CLI thin client**

---

# 1. 项目总览

## 1.1 目标与非目标

### 目标

1. 实现 DocQL 风格的**结构化查询语言**（解析、AST、错误提示、函数调用、过滤器、链式访问）
2. 实现**执行器**：能对 workspace 内多文件执行 DocQL，并合并结果
3. 实现**索引/解析层**：提取结构（files/structure/toc/content/code/comments/symbols）
4. 实现 **daemon**：workspace 注册、初始化索引、文件监听增量更新、提供 RPC 查询接口
5. 实现 **CLI**：默认连接 daemon，连接失败可降级单次执行（可选）
6. 提供 **协议**：JSON-RPC（LSP 风格）或 gRPC（建议先 JSON-RPC）

### 非目标（阶段性不做）

- 完整 IDE 级语义绑定（LSP 精准 refs/rename）→ 先留接口
- 全量多格式（PDF/DOCX）深度支持 → 先把接口打好
- 向量检索/复杂 rerank → 先 BM25/FTS 或结构优先排序

---

# 2. 总体架构

```
+-----------------------+          +------------------------------+
|        CLI rgpp       | <------> |          daemon rgppd        |
|  - parse args         |  JSON-RPC| - workspace registry          |
|  - render output      |  /socket | - file watcher (incremental) |
|  - fallback (optional)|          | - index store (sqlite)        |
+-----------+-----------+          | - docql engine (parse/exec)   |
            |                      +------------------------------+
            |
            v
   (optional standalone)
   - scan/parse/index temp
```

核心分层（强约束）：

- **DocQL Engine**：Lexer/Parser → AST → Planner → Executor
- **Registry**：Workspace/DocumentRegistry（缓存、合并、多文件路由）
- **Parser Layer**：Markdown / Code（tree-sitter）/ PlainText /（未来 Tika）
- **Index Layer**：SQLite（files, symbols, comments, chunks, …） + FTS（可选）
- **Transport/Protocol**：JSON-RPC over unix socket / named pipe / tcp

---

# 3. Repo 结构（Go）

建议 mono-repo：

```
rgpp/
  cmd/
    rgpp/          # CLI
    rgppd/         # daemon
  internal/
    docql/
      lexer/
      parser/
      ast/
      planner/
      executor/
      errors/
    registry/
      workspace.go
      doc_registry.go
    parser/
      markdown/
      code/
      plaintext/
      iface.go
    index/
      sqlite/
      schema.sql
      fts.go
      iface.go
    daemon/
      server.go
      rpc_jsonrpc.go
      watcher.go
    cli/
      render.go
      connect.go
    common/
      types.go      # ResultItem / Position / DocChunk / Symbol...
      paths.go
      logging.go
  pkg/             # 可选：对外 SDK（给 IDE/Agent）
  docs/
    design.md
    protocol.md
    grammar.md
```

---

# 4. DocQL 语法：继承策略

你的策略是“继承他的语法”，我建议按下面方式落地：

## 4.1 语法范围（MVP → 完整）

### MVP（先实现最常用 70%）

- `query := '$' pathExpr`
- `pathExpr := ('.' IDENT | functionCall | arrayFilter)*`
- `functionCall := IDENT '(' args? ')'`
- `arrayFilter := '[' '?(' predicate ')' ']' | '[' '*' ']' | '[' NUMBER ']'`
- `predicate := value (==|!=|=~|!~|in) value | booleanCombos`
- `value := STRING | NUMBER | IDENT | pathRef`
- 支持 `$.files` / `$.structure` / `$.content.*` / `$.code.*` / `$.toc` / `$.entities` 的“路径空间”
- 支持链式调用：`$.content.heading("x").grep("y")`

### 第二阶段（更贴近 DocQL 完整体验）

- 更多谓词运算符：`and/or/not`, `contains`, `startsWith`, `endsWith`
- `@` 当前对象引用（JSONPath 风格）
- 更强的函数 pipeline（map/sort/limit/groupBy）
- 错误恢复（能提示“你这里缺了 ] ”这种）

## 4.2 兼容性原则（非常重要）

1. **语法层面尽量兼容**：同样 DocQL 语句在你实现里不应报错（或给明确不支持提示）
2. **语义层面可渐进**：先给合理结果，后续增强精度（例如 code refs）
3. **扩展通过函数**：新能力一律用 `$.xxx.newFunc(...)` 形式加，避免破坏语法

---

# 5. 核心数据模型（结构化检索的根）

统一输出为“结构对象”，不要回到“行匹配”。

## 5.1 通用位置与命中项

```go
type Position struct {
  Path string
  StartLine, StartCol int
  EndLine, EndCol int
  // 扩展：ByteOffset/Page/ParagraphID
}

type ResultItem struct {
  Kind   string            // file, dir, chunk, symbol, comment, toc, entity, codeblock...
  Score  float64
  Title  string            // 展示标题（符号名/章节名）
  Snip   string            // 短摘要
  Pos    Position
  Extra  map[string]any    // signature, lang, tags, container...
}
```

## 5.2 文档结构对象（对齐 DocQL 的 toc/entities/chunks）

- `FileMeta`
- `DirNode`（structure tree）
- `TOCItem`（markdown headings）
- `DocumentChunk`（按 heading/窗口切片）
- `Symbol`（类/方法/变量）
- `Comment`（line/block/doc）

---

# 6. 执行引擎：Parser → Planner → Executor

## 6.1 为什么要 Planner（别直接“边走 AST 边查”）

DocQL 查询可能涉及：

- 多文件、跨索引的 join（例如 symbols + comments）
- smart search 多通道并行
- filter/sort/limit 组合

Planner 负责把 AST 编译成一个 `QueryPlan`，Executor 执行 plan，便于：

- explain
- 性能优化（走索引而不是扫文件）
- 缓存复用

## 6.2 QueryPlan（示意）

```go
type QueryPlan struct {
  Root string               // files/structure/content/code/toc/entities
  Steps []PlanStep          // path step / function / filter / projection...
  Limit int
  Sort  *SortSpec
}
```

PlanStep 可以有：

- `IndexLookupStep`（走 sqlite）
- `DocParseStep`（需要即时解析）
- `FilterStep` / `MapStep` / `LimitStep`

---

# 7. Parser/Index：MVP 如何选

你要做的是“平台”，所以建议**索引优先**，解析器可渐进。

## 7.1 Index 存储（建议 SQLite）

- 优点：跨平台、简单、可事务、可增量更新
- MVP 表（建议就这些先）：
  - `files(path, ext, lang, size, mtime, hash)`
  - `symbols(id, kind, name, name_norm, signature, lang, path, sl, sc, el, ec, container)`
  - `comments(id, kind, text, text_norm, tags, path, sl, sc, el, ec)`
  - `chunks(id, kind, title, text, path, sl, sc, el, ec)`
  - `toc(id, level, title, anchor, path, sl, sc, el, ec)`

- 可选：FTS5（comments/chunks）做 `grep` 加速

## 7.2 解析器（MVP）

- **Markdown**：heading、codeblock、chunk、toc
- **Code**：tree-sitter 抽 symbols/comments（refs 第二阶段）
- **PlainText**：分 chunk + 简单 grep

> 第二阶段再加：refs 表、AST query、PDF/DOCX

---

# 8. daemon：Workspace 注册/初始化/监听/查询

## 8.1 Workspace Registry

维护：

- root path
- ignore rules（gitignore + 用户配置）
- index path（`.rgpp/index.db`）
- parser capabilities（有哪些语言/解析器可用）
- 状态（building/ready/error）

## 8.2 文件监听（增量索引）

- fsnotify（Go 标准生态成熟）
- debouncing（避免保存时触发多次）
- 变更队列 → worker pool → 更新索引（单文件事务）

增量策略：

1. changed → hash 变了才处理
2. 删除：清理该 path 所有记录
3. 重命名：按 delete + add 处理即可

---

# 9. 协议：JSON-RPC（LSP 风格）文档

## 9.1 传输

优先顺序：

1. unix socket（Linux/macOS）/ named pipe（Windows）
2. localhost tcp（可选）
3. stdio（可选，给 IDE 拉起子进程）

## 9.2 方法定义（MVP）

- `initialize(params)` → capabilities、server version
- `workspace.add({root, ignores?, globs?})` → workspaceId
- `workspace.status({workspaceId})` → {state, progress, stats}
- `index.build({workspaceId})` → jobId（异步，但返回可查询进度）
- `query({workspaceId, q, limit?, offset?, format?})` → ResultItem[]
- `explain({workspaceId, q})` → {ast, plan, indexHits}
- `events.subscribe({workspaceId, topics})` → stream id（或 server push）

> 说明：即使 build 是“长任务”，也不要让 CLI 等死。用 jobId + events 最顺滑。

---

# 10. CLI：用户体验与输出规范

## 10.1 命令（建议）

- `rgpp daemon start|stop|status`
- `rgpp ws add <path>`
- `rgpp ws list`
- `rgpp q '<docql>' [--jsonl] [--limit N] [--explain]`
- `rgpp explain '<docql>'`
- `rgpp events`（可选）

## 10.2 输出格式

- 默认：类 rg 输出（path:line: snippet）
- `--jsonl`：每行一个 ResultItem（给 agent/脚本）
- `--compact`：只打印位置 + title
- `--explain`：同时打印 query plan（调试必备）

---

# 11. 开发计划（里程碑）

下面是一个“从零到可用”的顺序，按依赖关系排好了。

## Milestone 0：项目脚手架（1–2 天）

- 仓库结构 + logging + config
- cmd/rgpp, cmd/rgppd 可编译运行
- 基础 types（Position/ResultItem）

交付物：

- `rgpp --help` / `rgppd --help`
- docs/ 目录骨架

---

## Milestone 1：DocQL Lexer/Parser/AST（4–7 天）

- 实现 Lexer（token：`$ . ident string number ( ) [ ] ? @ operators`）
- 实现 Parser（递归下降）
- AST 结构（PathStep / FunctionCall / FilterPredicate）
- 错误提示（至少能定位 offset、给出“期望 token”）

交付物：

- `rgpp parse '<docql>'` 输出 AST（调试命令）
- docs/grammar.md（你语法的“官方规范”）

---

## Milestone 2：Planner + Explain（3–5 天）

- AST → QueryPlan
- 支持 explain 输出（AST + plan）
- 先不执行，只验证 plan 生成

交付物：

- `rgpp explain '<docql>'` 有稳定输出
- docs/design.md 增加 planner 章节

---

## Milestone 3：SQLite Index + Schema（3–6 天）

- schema.sql（files/toc/chunks/comments/symbols）
- IndexStore 接口（UpsertFile/DeleteFile/QuerySymbols/QueryComments/QueryChunks…）
- FTS5（可选，建议 comments/chunks 先开）

交付物：

- `rgppd` 能创建 index.db
- 最小查询接口跑通（直接 SQL）

---

## Milestone 4：Parser Layer（Markdown/PlainText）（5–10 天）

- Markdown 解析：headings/toc/chunks/codeblocks/comments（注释可先不做）
- PlainText：chunking + grep
- 抽象接口 `DocumentParser`：Parse(file)->Extracted

交付物：

- 能对 workspace 扫描并入库 files/toc/chunks
- `$.files` / `$.structure` / `$.content.heading()` 这类能有结果

---

## Milestone 5：Executor（DocQL → Index 查询）（5–10 天）

- 实现 Executor：根据 QueryPlan 走 IndexStore
- 支持：
  - `$.files[...]`
  - `$.structure`
  - `$.toc[...]`
  - `$.content.grep()/heading()/chapter()`（chapter 可后置）

- 合并多文件结果、排序、limit

交付物：

- `rgpp q '$.structure'`
- `rgpp q '$.content.grep("xxx")'`

---

## Milestone 6：daemon RPC + Workspace Registry（5–10 天）

- JSON-RPC server over unix socket/named pipe
- workspace.add/status/query/explain
- job + progress events（简版也行：先 status 轮询）

交付物：

- CLI 能连接 daemon 查询
- daemon 维护 workspace 状态

---

## Milestone 7：文件监听 + 增量更新（3–7 天）

- fsnotify + debounce
- 单文件事务更新索引
- events 推送（indexProgress/fileChanged）

交付物：

- 改文件 → 结果秒级更新

---

## Milestone 8：Code Parser（tree-sitter）+ symbols/comments（7–14 天）

- 集成 tree-sitter（建议先 TS/Go 任意一种）
- 提取 symbols（function/class/var）
- 提取 comments（line/block/doc）
- DocQL 增加：
  - `$.code.symbol(...)`
  - `$.code.comment.grep(...)`

交付物：

- “搜方法名/搜变量/搜注释”成立（哪怕 refs 暂时不准）

---

## Milestone 9：refs + AST query（可选，后续）

- refs：先名字级，再做绑定增强
- `$.code.ast.query(...)`：直接支持 tree-sitter query

---

# 12. 质量与工程保障（别省，后面会救你命）

## 测试

- Lexer/Parser：golden tests（输入→AST）
- Planner：AST→plan（golden）
- Executor：plan→results（用固定小 repo）
- daemon：协议集成测试（起服务→调用→断言）

## 性能

- 索引构建：并发 parse（但 DB 写入要批处理/事务）
- 查询：优先走索引；避免全文件扫描
- 结果：默认只回传 snippet（长度限制）

## 安全

- daemon 只允许访问注册 workspace 内路径（路径规范化 + 前缀校验）
- 限制一次 query 的最大返回条数/最大 snippet 长度
- explain 仅本地可用（可配置）

---

# 13. 文档交付清单（你要的“开发计划和文档”）

放在 `docs/`：

1. `docs/design.md`：总体架构、模块职责、数据模型、执行流程
2. `docs/grammar.md`：DocQL 子集 EBNF + 示例（这就是你的“兼容承诺”）
3. `docs/protocol.md`：JSON-RPC 方法、参数、返回、错误码、事件流
4. `docs/index.md`：SQLite schema、表字段、索引策略、增量更新规则
5. `docs/ops.md`：daemon 启动方式、socket 路径、日志、配置项、故障排查

---

## 最后一句：你现在就能开工的“第一步”

如果你同意这套路线，我建议**先把 Milestone 1–3（DocQL 解析+Planner+SQLite schema）**一口气做完——这是整个系统的地基，做完后你再接解析器/daemon 会非常顺。

如果你希望我更“落地到可复制粘贴开库”的程度，我也可以继续把：

- `schema.sql`（完整字段与索引）
- `protocol.md`（JSON-RPC 详细定义）
- `grammar.md`（EBNF + token 定义）
- `Planner/Executor` 的接口定义（Go 代码骨架）

一次性给你写出来，直接按这个骨架开仓库就能跑。
