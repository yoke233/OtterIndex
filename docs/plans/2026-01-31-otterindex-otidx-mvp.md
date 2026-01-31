# OtterIndex (otidx/otidxd) Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 把 OtterIndex 落成一个可用的 CLI `otidx`（先支持关键词 `q` 一把梭）+ daemon `otidxd`（先骨架），并且能返回“最小代码单元块/可控粒度”的结果，附带相对路径与行号范围，支持 include/exclude、大小写、上下文行、颜色主题与调试可视化输出。

**Architecture:** Rust workspace：`otidx-core` 负责扫描/匹配/代码块抽取/结果结构；`otidx` 只做参数解析与输出渲染；`otidxd` 提供 JSON-RPC（后续可扩展索引与增量更新）。动态加载关闭：语言支持以内置“文件类型识别 + 通用块抽取(缩进/括号)”为 MVP，后续可加 tree-sitter 作为增强（仍为编译期内置）。

**Tech Stack:** Rust（`clap`, `ignore`, `regex`, `serde`, `serde_json`, `termcolor`/`owo-colors`, `anyhow`/`thiserror`）, （后续）SQLite（`rusqlite`）、（后续增强）Tree-sitter。

---

## CLI 目标参数（MVP）

- `otidx q <q>`：接受纯关键词；如果参数以 `$` 开头则按 DocQL 模式处理（后续），否则走关键词搜索。
- `-d <dbname|path>`：使用指定数据库/路径（MVP 可先解析并打印，DB 在后续任务落地）。
- `-A`：扫描 ALL（忽略 gitignore/隐藏/二进制/大文件等限制，具体策略后续细化）。
- `-x <globs>`：排除文件（逗号分隔，例如 `-x *.js,*.sql`）。
- `-g <glob>`：只包含这些文件（可重复 `-g` 多次）。
- `-i`：大小写不敏感。
- `-c <num>`：上下文行数，默认 1（仅对 `--unit line` 生效；其他 unit 作为 snippet 裁剪参考）。
- `--unit <line|block|symbol|file>`：返回粒度，默认 `block`（`symbol` 先占位，后续 tree-sitter 实现）。
- `-B`：不打印 banner。
- `-L`：vim 友好行格式（`path:startLine:startCol:`）。
- `-b`：色盲友好模板。
- `-z`：关闭颜色。
- `-Z`：高对比度颜色。
- `-l`：列出可用数据库（后续 DB 落地后实现；MVP 可先列出默认目录下的 `*.db`）。
- `-v`：版本。
- `-h`：帮助。
- `--jsonl`：每行一个 JSON 结果（给脚本/agent）。
- `--explain`：打印“查询计划/通道/过滤器/最终 unit 选择”的调试信息。
- `--viz <ascii>`：图像化打印（MVP 先 ascii）。

## 结果结构（MVP）

统一输出为结构化结果，至少包含：

```json
{
  "kind": "match|block|file",
  "path": "relative/path/to/file.rs",
  "range": { "sl": 120, "sc": 1, "el": 178, "ec": 1 },
  "title": "optional (symbol name / block hint)",
  "snippet": "optional short text",
  "matches": [{ "line": 133, "col": 9, "text": "..." }]
}
```

---

### Task 1: 初始化 Rust workspace 与二进制命名

**Files:**
- Create: `Cargo.toml`
- Create: `crates/otidx-core/Cargo.toml`
- Create: `crates/otidx-core/src/lib.rs`
- Create: `crates/otidx-cli/Cargo.toml`
- Create: `crates/otidx-cli/src/main.rs`
- Create: `crates/otidxd/Cargo.toml`
- Create: `crates/otidxd/src/main.rs`
- Create: `README.md`
- Test: `crates/otidx-cli/tests/help_smoke_test.rs`

**Step 1: 写一个会失败的 help 测试（确保二进制存在且 --help 可跑）**

```rust
// crates/otidx-cli/tests/help_smoke_test.rs
#[test]
fn help_runs() {
    let output = std::process::Command::new(env!("CARGO_BIN_EXE_otidx"))
        .arg("--help")
        .output()
        .expect("run otidx");
    assert!(output.status.success());
}
```

**Step 2: 运行测试，确认失败（因为还没有 otidx 二进制）**

Run: `cargo test -p otidx-cli`
Expected: FAIL（找不到 `CARGO_BIN_EXE_otidx` 或二进制未生成）

**Step 3: 写最小实现：workspace + otidx/otidxd 二进制可编译**

- `otidx`/`otidxd` 先只打印 `--version/--help` 相关内容。

**Step 4: 再跑测试，确认通过**

Run: `cargo test -p otidx-cli`
Expected: PASS

**Step 5: Commit**

```bash
git add Cargo.toml crates README.md
git commit -m "chore(otidx): init OtterIndex workspace and binaries"
```

---

### Task 2: CLI 参数解析（对齐你给的参数表）

**Files:**
- Modify: `crates/otidx-cli/src/main.rs`
- Create: `crates/otidx-cli/src/cli.rs`
- Test: `crates/otidx-cli/tests/cli_parse_test.rs`

**Step 1: 写一个会失败的参数解析测试（-c 默认值、-x 逗号分隔）**

```rust
// crates/otidx-cli/tests/cli_parse_test.rs
use otidx_cli::cli::Cli;

#[test]
fn parse_defaults() {
    let cli = Cli::try_parse_from(["otidx", "q", "hello"]).unwrap();
    assert_eq!(cli.context_lines, 1);
}

#[test]
fn parse_excludes_csv() {
    let cli = Cli::try_parse_from(["otidx", "q", "k", "-x", "*.js,*.sql"]).unwrap();
    assert_eq!(cli.exclude_globs, vec!["*.js", "*.sql"]);
}
```

**Step 2: 运行测试，确认失败**

Run: `cargo test -p otidx-cli`
Expected: FAIL（`Cli` 未实现）

**Step 3: 最小实现 `Cli` 与 clap 定义**

- `q` 子命令：`otidx q <q>`
- 参数：`-d/-A/-x/-g/-i/-c/-B/-L/-b/-z/-Z/-l/-v/-h/--jsonl/--explain/--viz/--unit`

**Step 4: 再跑测试**

Run: `cargo test -p otidx-cli`
Expected: PASS

**Step 5: Commit**

```bash
git add crates/otidx-cli
git commit -m "feat(otidx): parse CLI args for q/include/exclude/context/theme"
```

---

### Task 3: 关键词搜索引擎（先不接 DB，直接扫目录）

**Files:**
- Modify: `crates/otidx-core/src/lib.rs`
- Create: `crates/otidx-core/src/search/mod.rs`
- Create: `crates/otidx-core/src/search/walk.rs`
- Create: `crates/otidx-core/src/search/matchers.rs`
- Test: `crates/otidx-core/tests/search_smoke_test.rs`

**Step 1: 写会失败的测试：给一个临时目录，能搜到匹配行号与内容**

```rust
// crates/otidx-core/tests/search_smoke_test.rs
use std::fs;
use tempfile::tempdir;

#[test]
fn finds_matches_with_line_numbers() {
    let dir = tempdir().unwrap();
    let p = dir.path().join("a.txt");
    fs::write(&p, "x\nhello\nz\n").unwrap();

    let results = otidx_core::search::search_keyword(dir.path(), "hello", false).unwrap();
    assert_eq!(results.len(), 1);
    assert_eq!(results[0].path, "a.txt");
    assert_eq!(results[0].matches[0].line, 2);
}
```

**Step 2: 运行测试，确认失败**

Run: `cargo test -p otidx-core`
Expected: FAIL（`search_keyword` 不存在）

**Step 3: 最小实现：目录遍历 + 行扫描**

- 先用 `ignore` crate 做 walker（支持后续 gitignore/hidden 控制）
- 逐文件读取为行（先从 UTF-8 文本开始；二进制直接跳过）
- 返回相对路径（相对 root）

**Step 4: 再跑测试**

Run: `cargo test -p otidx-core`
Expected: PASS

**Step 5: Commit**

```bash
git add crates/otidx-core
git commit -m "feat(otidx-core): keyword search with relative path + line numbers"
```

---

### Task 4: include/exclude/-A/-i 行为落地（文件过滤与大小写）

**Files:**
- Modify: `crates/otidx-core/src/search/walk.rs`
- Modify: `crates/otidx-core/src/search/matchers.rs`
- Test: `crates/otidx-core/tests/filtering_test.rs`

**Step 1: 写失败测试：-g 只包含、-x 排除、-i 大小写**

```rust
// crates/otidx-core/tests/filtering_test.rs
use std::fs;
use tempfile::tempdir;

#[test]
fn include_and_exclude_globs_work() {
    let dir = tempdir().unwrap();
    fs::write(dir.path().join("a.rs"), "hello\n").unwrap();
    fs::write(dir.path().join("a.sql"), "hello\n").unwrap();

    let r = otidx_core::search::SearchOptions {
        include_globs: vec!["*.rs".into()],
        exclude_globs: vec!["*.sql".into()],
        case_insensitive: false,
        scan_all: false,
    };
    let results = otidx_core::search::search_keyword_with_opts(dir.path(), "hello", &r).unwrap();
    assert_eq!(results.len(), 1);
    assert_eq!(results[0].path, "a.rs");
}
```

**Step 2: 运行测试，确认失败**

Run: `cargo test -p otidx-core`
Expected: FAIL

**Step 3: 最小实现过滤与大小写**

- include/exclude：用 `globset`（或自己先实现非常简单的 `*` 匹配，后续替换）
- `-A`：walker 允许 hidden + 忽略规则（MVP：至少做到“不过滤隐藏文件”）
- `-i`：matchers 走不区分大小写（MVP：对 ASCII 先可用）

**Step 4: 再跑测试**

Run: `cargo test -p otidx-core`
Expected: PASS

**Step 5: Commit**

```bash
git add crates/otidx-core
git commit -m "feat(otidx-core): include/exclude globs, scan-all, case-insensitive"
```

---

### Task 5: “最小代码单元块”抽取（--unit line/block/file）

**Files:**
- Create: `crates/otidx-core/src/unit/mod.rs`
- Create: `crates/otidx-core/src/unit/line_unit.rs`
- Create: `crates/otidx-core/src/unit/block_unit.rs`
- Modify: `crates/otidx-core/src/lib.rs`
- Test: `crates/otidx-core/tests/unit_block_test.rs`

**Step 1: 写失败测试：block 单元会扩展到一个“合理块范围”并返回 range**

```rust
// crates/otidx-core/tests/unit_block_test.rs
use std::fs;
use tempfile::tempdir;

#[test]
fn block_unit_expands_around_match() {
    let dir = tempdir().unwrap();
    let p = dir.path().join("a.rs");
    fs::write(
        &p,
        "fn a() {\n  let x = 1;\n  // KEY\n  let y = 2;\n}\n",
    )
    .unwrap();

    let results = otidx_core::search_and_unitize(dir.path(), "KEY", "block").unwrap();
    assert_eq!(results[0].range.sl, 1);
    assert_eq!(results[0].range.el, 5);
}
```

**Step 2: 运行测试，确认失败**

Run: `cargo test -p otidx-core`
Expected: FAIL

**Step 3: 最小实现 unitize**

- `line`：使用 `-c` 输出上下文（range = 目标行上下 c 行）
- `file`：range = 全文件
- `block`（MVP 规则，跨语言通用，先够用）：
  - 对“括号类语言”：从命中行向上找最近的 `{`/`}` 平衡点，再向下找闭合（用简单栈）
  - 对“缩进类语言”：以命中行缩进为基准，向上找缩进更小且非空行作为块起点，向下直到缩进更小
  - 如果两种都失败：退回 `line` unit

**Step 4: 再跑测试**

Run: `cargo test -p otidx-core`
Expected: PASS

**Step 5: Commit**

```bash
git add crates/otidx-core
git commit -m "feat(otidx-core): unit extraction for line/block/file with ranges"
```

---

### Task 6: 输出渲染（-L/--jsonl/主题/无色/高对比）+ --explain/--viz ascii

**Files:**
- Modify: `crates/otidx-cli/src/main.rs`
- Create: `crates/otidx-cli/src/render.rs`
- Create: `crates/otidx-cli/src/viz.rs`
- Test: `crates/otidx-cli/tests/jsonl_output_test.rs`

**Step 1: 写失败测试：--jsonl 输出可被解析**

```rust
// crates/otidx-cli/tests/jsonl_output_test.rs
use serde_json::Value;

#[test]
fn jsonl_is_valid_json_per_line() {
    let output = std::process::Command::new(env!("CARGO_BIN_EXE_otidx"))
        .args(["q", "hello", "--jsonl", "-g", "*.md"])
        .output()
        .expect("run");
    // 允许没结果，但若有行必须是 JSON
    for line in String::from_utf8_lossy(&output.stdout).lines() {
        let _v: Value = serde_json::from_str(line).unwrap();
    }
}
```

**Step 2: 运行测试，确认失败**

Run: `cargo test -p otidx-cli`
Expected: FAIL

**Step 3: 最小实现渲染**

- 默认输出：类似 `path:line: snippet`
- `-L`：`path:sl:sc: snippet`
- `--jsonl`：每行一个 JSON
- `-z`：禁用颜色；`-b/-Z`：切换调色板
- `--explain`：stderr 打印选中的过滤器、unit 规则、命中计数
- `--viz ascii`：打印一个简单的“管线图”（输入→walk→match→unitize→render）

**Step 4: 再跑测试**

Run: `cargo test -p otidx-cli`
Expected: PASS

**Step 5: Commit**

```bash
git add crates/otidx-cli
git commit -m "feat(otidx): jsonl/vim output, themes, explain and ascii viz"
```

---

### Task 7: otidxd daemon 骨架（先能启动并响应 ping/version）

**Files:**
- Modify: `crates/otidxd/src/main.rs`
- Create: `crates/otidxd/src/rpc.rs`
- Test: `crates/otidxd/tests/ping_test.rs`

**Step 1: 写失败测试：起一个子进程，能响应 ping（MVP 用 tcp 127.0.0.1:0）**

```rust
// crates/otidxd/tests/ping_test.rs
#[test]
fn daemon_starts() {
    // MVP：只测试进程能启动并在短时间内存活
    let mut child = std::process::Command::new(env!("CARGO_BIN_EXE_otidxd"))
        .arg("--listen")
        .arg("127.0.0.1:0")
        .spawn()
        .expect("start");
    std::thread::sleep(std::time::Duration::from_millis(200));
    let _ = child.kill();
}
```

**Step 2: 运行测试，确认失败**

Run: `cargo test -p otidxd`
Expected: FAIL

**Step 3: 最小实现 daemon**

- 先支持 `--listen 127.0.0.1:PORT`
- 提供最小 JSON-RPC 方法：`ping`、`version`
- 先不接 workspace/index，先把通信打通

**Step 4: 再跑测试**

Run: `cargo test -p otidxd`
Expected: PASS

**Step 5: Commit**

```bash
git add crates/otidxd
git commit -m "feat(otidxd): minimal daemon skeleton with json-rpc ping/version"
```

---

### Task 8: （后续）DB/索引与 --unit symbol（tree-sitter 增强）

**Files:**
- Create: `crates/otidx-core/src/index/mod.rs`
- Create: `crates/otidx-core/src/index/sqlite.rs`
- Create: `crates/otidx-core/src/index/schema.sql`
- Create: `crates/otidx-core/src/unit/symbol_unit.rs`

**Step 1: 先落地 SQLite schema 与 `-d` 行为**

- `-d` 指向 `index.db`，默认 `.otidx/index.db`
- `-l` 列出默认目录下数据库

**Step 2: 实现索引构建与查询（files/chunks 的最小集合）**

**Step 3: tree-sitter 增强（仍为编译期内置，不做动态加载）**

- `--unit symbol`：返回最小包含命中点的函数/方法/类 range
- 依赖说明：需要 C 编译器（Windows 建议安装 VS Build Tools 或 LLVM/clang）

**Step 4: otidx ↔ otidxd 集成**

- `otidx` 默认连接 daemon，失败则降级本地扫描

**Step 5: Commit（按子任务拆分）**

---

## 验收清单（MVP）

- `otidx q <q>` 能在指定目录跑通，返回相对路径与行号范围（至少 `line/block/file`）
- `-g/-x/-i/-c` 影响结果符合预期
- `--jsonl` 输出可解析
- `--explain/--viz ascii` 能帮助调试
- `otidxd` 能启动并提供最小 RPC（为后续索引/增量打基础）

