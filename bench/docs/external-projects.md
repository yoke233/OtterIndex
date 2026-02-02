# 外部项目基准测试（otidx vs rg）

更新时间：2026-02-02 10:36:07

说明：
- otidx 走 SQLite/FTS 索引；rg 为直接扫描文件。
- 数值为脚本取多次运行中的最小 wall time（ms）。repeat=3，limit=20。
- 图表内 otidx 使用“查询耗时（wall - 加载）”；加载时间在图表顶部单独标注。
- 当前 tree-sitter 只接入 Go；非 Go 工程里 `--unit symbol` 会自动降级（fallback）。

## Java / Spring（JeecgBoot - jeecg-boot）

![Java / Spring（JeecgBoot - jeecg-boot） 速度对比](external-projects-java.svg)

- root: `D:\project\JeecgBoot\jeecg-boot`
- db: `.otidx/ext/java-jeecg-boot.db`
- include globs: `*.java, *.xml, *.yml, *.yaml, *.properties`
- index.build: `1674ms`（files_total=981 files_indexed=981 chunks_written=3381 symbols_written=5480 treesitter_unsupported=146 fts5=True）

| case | unit | globs | otidx(ms) | rg(ms) | 说明 |
|---|---|---|---:|---:|---|
| RestController (line) | line | *.java | 71 | 36 |  |
| RequestMapping (line) | line | *.java | 73 | 40 |  |
| public class (line) | line | *.java | 86 | 37 |  |
| throw new (line) | line | *.java | 78 | 37 |  |
| RestController (symbol, fallback) | symbol | *.java | 77 | 44 | symbol_fallback; unit_fallback=symbol->block |

- 加载时间（otidx wall - query）：中位 71ms（min 65 / max 74）

## Vue/TS（JeecgBoot - jeecgboot-vue3）

![Vue/TS（JeecgBoot - jeecgboot-vue3） 速度对比](external-projects-vue.svg)

- root: `D:\project\JeecgBoot\jeecgboot-vue3`
- db: `.otidx/ext/vue-jeecgboot-vue3.db`
- include globs: `*.vue, *.ts, *.tsx, *.js, *.jsx, *.css, *.scss, *.json`
- index.build: `1353ms`（files_total=1383 files_indexed=1383 chunks_written=5050 symbols_written=2874 treesitter_unsupported=744 fts5=True）

| case | unit | globs | otidx(ms) | rg(ms) | 说明 |
|---|---|---|---:|---:|---|
| defineComponent (line) | line | *.vue, *.ts, *.tsx | 83 | 38 |  |
| export default (line) | line | *.vue, *.ts, *.tsx, *.js, *.jsx | 85 | 38 |  |
| axios (line) | line | *.vue, *.ts, *.tsx, *.js, *.jsx | 72 | 39 |  |
| router (line) | line | *.vue, *.ts, *.tsx, *.js, *.jsx | 73 | 37 |  |
| defineComponent (symbol, fallback) | symbol | *.vue, *.ts, *.tsx | 86 | 38 | symbol_fallback; unit_fallback=symbol->block |

- 加载时间（otidx wall - query）：中位 72ms（min 66 / max 73）

## Python（crawl4ai）

![Python（crawl4ai） 速度对比](external-projects-python.svg)

- root: `D:\project\crawl4ai`
- db: `.otidx/ext/python-crawl4ai.db`
- include globs: `*.py, *.toml, *.yml, *.yaml, *.md`
- index.build: `2137ms`（files_total=482 files_indexed=482 chunks_written=4549 symbols_written=3229 treesitter_unsupported=134 fts5=True）

| case | unit | globs | otidx(ms) | rg(ms) | 说明 |
|---|---|---|---:|---:|---|
| async def (line) | line | *.py | 95 | 35 |  |
| def (line) | line | *.py | 104 | 35 |  |
| class (line) | line | *.py | 85 | 33 |  |
| import (line) | line | *.py | 89 | 33 |  |
| async def (symbol, fallback) | symbol | *.py | 93 | 35 | symbol_fallback; unit_fallback=symbol->block |

- 加载时间（otidx wall - query）：中位 72ms（min 66 / max 76）

## Python/ML（PaddleOCR）

![Python/ML（PaddleOCR） 速度对比](external-projects-python-paddleocr.svg)

- root: `D:\project\PaddleOCR`
- db: `.otidx/ext/python-paddleocr.db`
- include globs: `*.py, *.toml, *.yml, *.yaml, *.md`
- index.build: `2337ms`（files_total=1068 files_indexed=1068 chunks_written=6424 symbols_written=4777 treesitter_unsupported=581 fts5=True）

| case | unit | globs | otidx(ms) | rg(ms) | 说明 |
|---|---|---|---:|---:|---|
| import paddle (line) | line | *.py | 80 | 35 |  |
| paddle nn (line) | line | *.py | 84 | 50 |  |
| def (line) | line | *.py | 96 | 33 |  |
| class (line) | line | *.py | 87 | 33 |  |
| import paddle (symbol, fallback) | symbol | *.py | 79 | 38 | symbol_fallback; unit_fallback=symbol->block |

- 加载时间（otidx wall - query）：中位 73ms（min 69 / max 74）

## C/C++（gdmplab）

![C/C++（gdmplab） 速度对比](external-projects-cpp.svg)

- root: `D:\project\gdmplab`
- db: `.otidx/ext/cpp-gdmplab.db`
- include globs: `*.c, *.cc, *.cpp, *.cxx, *.h, *.hpp, CMakeLists.txt, *.cmake`
- index.build: `11086ms`（files_total=3569 files_indexed=3569 chunks_written=12690 symbols_written=26971 treesitter_unsupported=41 fts5=True）

| case | unit | globs | otidx(ms) | rg(ms) | 说明 |
|---|---|---|---:|---:|---|
| include (line) | line | *.c, *.cc, *.cpp, *.cxx, *.h, *.hpp | 96 | 34 |  |
| namespace (line) | line | *.c, *.cc, *.cpp, *.cxx, *.h, *.hpp | 98 | 32 |  |
| template (line) | line | *.c, *.cc, *.cpp, *.cxx, *.h, *.hpp | 90 | 35 |  |
| add_executable (line) | line | CMakeLists.txt, *.cmake | 69 | 52 |  |
| include (symbol, fallback) | symbol | *.c, *.cc, *.cpp, *.cxx, *.h, *.hpp | 91 | 33 | symbol_fallback; unit_fallback=symbol->block |

- 加载时间（otidx wall - query）：中位 73ms（min 67 / max 76）

---

原始数据：`result-2026-02-02.txt`

