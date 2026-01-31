AutoDev DocQL | AutoDev - Tailor Your AI Coding Experience




[跳到主要内容](#__docusaurus_skipToContent_fallback)

[![AutoDev 标志](/img/logo.svg)![AutoDev 标志](/img/logo.svg)

**AutoDev**](/)[文档](/intro)[Customize](/customize)[Agent](/agent)[MultiPlatform](/mpp/autodev-multiplatform)[AI Friendly](/ai-friendly)[博客](/blog)[Development](/development)

[简体中文](#)

* [English](/en/mpp/autodev-mpp-docql)
* [简体中文](/mpp/autodev-mpp-docql)

[GitHub](https://github.com/unit-mesh/auto-dev)

* [AutoDev MPP](/mpp/autodev-multiplatform)
* [AutoDev MPP Quickstart](/mpp/autodev-mpp-quickstart)
* [AutoDev CLI/TUI](/mpp/autodev-cli-tui)
* [AutoDev Remote](/mpp/autodev-remote)
* [AutoDev Android](/mpp/autodev-mpp-android)
* [AutoDev Desktop](/mpp/autodev-mpp-desktop)
* [AutoDev DocQL](/mpp/autodev-mpp-docql)

* AutoDev DocQL

本页总览

AutoDev DocQL
=============

一、DocQL 概述[​](#一docql-概述 "一、DocQL 概述的直接链接")
-------------------------------------------

DocQL（Document Query Language）是一个**类 JSONPath 的文档查询语言**，专门设计用于在文档中查询目录（TOC）、实体（Entities）和内容（Content）。它既支持用户在
UI 中交互使用，也适合 AI Agent 程序化调用。

### 核心设计理念[​](#核心设计理念 "核心设计理念的直接链接")

```
$.toc[?(@.level==1)]          // 查询一级标题  
$.entities[?(@.type=="API")]  // 查询所有 API 实体  
$.content.heading("架构")      // 查询包含"架构"的章节内容  
$.code.class("DocQLLexer")    // 查询源代码中的类
```

二、系统架构[​](#二系统架构 "二、系统架构的直接链接")
-------------------------------

### 2.1 架构层次图[​](#21-架构层次图 "2.1 架构层次图的直接链接")

```
┌─────────────────────────────────────────────────────────────────────┐  
│                          应用层 (Application Layer)                   │  
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐  │  
│  │   DocumentAgent   │  │  CLI/TUI (TS)    │  │   Compose UI     │  │  
│  │  (AI 驱动查询)    │  │  (交互式查询)    │  │   (可视化查询)   │  │  
│  └────────┬─────────┘  └────────┬─────────┘  └────────┬─────────┘  │  
└───────────┼──────────────────────┼──────────────────────┼───────────┘  
            │                      │                      │  
            ▼                      ▼                      ▼  
┌─────────────────────────────────────────────────────────────────────┐  
│                          工具层 (Tool Layer)                         │  
│  ┌──────────────────────────────────────────────────────────────┐  │  
│  │                        DocQLTool                              │  │  
│  │  • Smart Search（关键词智能搜索）                              │  │  
│  │  • Direct Query（标准 DocQL 查询）                            │  │  
│  │  • Multi-Level Keyword Expansion（多级关键词扩展）            │  │  
│  └──────────────────────────────────────────────────────────────┘  │  
└─────────────────────────────────┬───────────────────────────────────┘  
                                  │  
                                  ▼  
┌─────────────────────────────────────────────────────────────────────┐  
│                          查询引擎 (Query Engine)                     │  
│  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────┐   │  
│  │ DocQLLexer │→ │DocQLParser │→ │DocQLQuery  │→ │DocQLExecutor│  │  
│  │  (词法分析) │  │ (语法分析) │  │   (AST)    │  │  (执行器)  │   │  
│  └────────────┘  └────────────┘  └────────────┘  └────────────┘   │  
└─────────────────────────────────┬───────────────────────────────────┘  
                                  │  
                                  ▼  
┌─────────────────────────────────────────────────────────────────────┐  
│                          注册表层 (Registry Layer)                   │  
│  ┌──────────────────────────────────────────────────────────────┐  │  
│  │                    DocumentRegistry                          │  │  
│  │  • 内存文档缓存    • 解析器管理    • 多文件查询融合          │  │  
│  │  • IndexProvider 接口（桥接持久化存储）                       │  │  
│  └──────────────────────────────────────────────────────────────┘  │  
└─────────────────────────────────┬───────────────────────────────────┘  
                                  │  
                                  ▼  
┌─────────────────────────────────────────────────────────────────────┐  
│                          解析层 (Parser Layer)                       │  
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │  
│  │ Markdown     │  │ Tika(PDF/    │  │ Code(Tree-   │              │  
│  │ Parser       │  │ DOCX/PPTX)   │  │ Sitter)      │              │  
│  └──────────────┘  └──────────────┘  └──────────────┘              │  
│                                                                      │  
│              DocumentParserService 接口统一抽象                      │  
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 核心组件[​](#22-核心组件 "2.2 核心组件的直接链接")

| 组件 | 位置 | 职责 |
| --- | --- | --- |
| `DocQLLexer` |  |  |
| 词法分析，将查询字符串转换为 Token 流 |  |  |
| `DocQLParser` |  |  |
| 语法分析，将 Token 解析为 AST |  |  |
| `DocQLSyntax` |  |  |
| 定义 AST 节点和过滤条件 |  |  |
| `DocQLExecutor` |  |  |
| 执行查询，返回结构化结果 |  |  |
| `DocumentRegistry` |  |  |
| 文档注册和缓存管理 |  |  |
| `DocQLTool` |  |  |
| Agent 工具封装，支持智能搜索 |  |  |

三、查询语法详解[​](#三查询语法详解 "三、查询语法详解的直接链接")
-------------------------------------

### 3.1 语法结构 (EBNF)[​](#31-语法结构-ebnf "3.1 语法结构 (EBNF)的直接链接")

```
query         := '$' path  
path          := (property | arrayAccess | functionCall)*  
property      := '.' IDENTIFIER  
arrayAccess   := '[' (STAR | NUMBER | filter) ']'  
filter        := '?' '(' condition ')'  
condition     := '@.' IDENTIFIER operator value  
operator      := '==' | '!=' | '~=' | '=~' | '>' | '>=' | '<' | '<='  
              | 'startsWith' | 'endsWith'  
value         := STRING | NUMBER | REGEX  
functionCall  := '.' IDENTIFIER '(' STRING? ')'
```

### 3.2 查询类型[​](#32-查询类型 "3.2 查询类型的直接链接")

#### 3.2.1 TOC（目录）查询[​](#321-toc目录查询 "3.2.1 TOC（目录）查询的直接链接")

```
$.toc[*]                        # 获取所有目录项  
$.toc[0]                        # 获取第一个目录项  
$.toc[?(@.level==1)]            # 获取一级标题  
$.toc[?(@.title~="架构")]       # 标题包含"架构"  
$.toc[?(@.level>1)]             # level > 1 的目录项  
$.toc[?(@.title =~ /MCP/i)]     # 正则匹配（忽略大小写）  
$.toc[?(@.title startsWith "Ch")]  # 标题以"Ch"开头
```

#### 3.2.2 Entities（实体）查询[​](#322-entities实体查询 "3.2.2 Entities（实体）查询的直接链接")

```
$.entities[*]                       # 获取所有实体  
$.entities[?(@.type=="API")]        # 获取 API 类型实体  
$.entities[?(@.type=="ClassEntity")]# 获取类实体  
$.entities[?(@.name~="User")]       # 名称包含"User"
```

#### 3.2.3 Content（内容）查询[​](#323-content内容查询 "3.2.3 Content（内容）查询的直接链接")

```
$.content.heading("架构")       # 查询标题匹配的章节内容  
$.content.chapter("1.2")        # 查询章节 1.2  
$.content.h1("Introduction")    # H1 标题  
$.content.h2("Design")          # H2 标题  
$.content.grep("关键词")         # 全文搜索  
$.content.chunks()              # 获取所有内容块
```

#### 3.2.4 Code（代码）查询[​](#324-code代码查询 "3.2.4 Code（代码）查询的直接链接")

```
$.code.class("DocQLLexer")      # 查询类的完整源码  
$.code.function("tokenize")     # 查询函数实现  
$.code.classes[*]               # 列出所有类  
$.code.functions[*]             # 列出所有函数  
$.code.classes[?(@.name~="Lexer")]  # 模糊匹配类名  
$.code.query("tokenize")        # 关键词搜索代码
```

#### 3.2.5 Files（文件）查询[​](#325-files文件查询 "3.2.5 Files（文件）查询的直接链接")

```
$.files[*]                          # 列出所有文件  
$.files[?(@.path contains "docs")]  # 按路径过滤  
$.files[?(@.extension == "md")]     # 按扩展名过滤
```

四、智能搜索机制（Smart Search）[​](#四智能搜索机制smart-search "四、智能搜索机制（Smart Search）的直接链接")
-----------------------------------------------------------------------------

### 4.1 多级关键词扩展策略[​](#41-多级关键词扩展策略 "4.1 多级关键词扩展策略的直接链接")

DocQL 的 Smart Search 采用**三级关键词扩展**策略：

```
原始查询: "base64 encoding"  
  
Level 1 (Primary) - 最精确  
├── "base64 encoding"  
├── "base64 encoder"  
└── "base64 encode"  
  
Level 2 (Secondary) - 组件词  
├── "base64"  
└── "encoding"  
  
Level 3 (Tertiary) - 词干变体  
├── "encode"  
├── "encoded"  
└── "encoder"
```

### 4.2 自适应搜索策略[​](#42-自适应搜索策略 "4.2 自适应搜索策略的直接链接")

```
enum class SearchStrategy {  
    KEEP,    // 结果数量理想，保持当前级别  
    EXPAND,  // 结果太少，扩展到下一级别  
    FILTER   // 结果太多，使用 secondaryKeyword 过滤  
}  
  
// 策略判断逻辑  
when {  
    resultCount < minThreshold && level < 3 -> EXPAND  
    resultCount > maxThreshold -> FILTER  
    resultCount in idealRange -> KEEP  
}
```

### 4.3 多通道融合搜索[​](#43-多通道融合搜索 "4.3 多通道融合搜索的直接链接")

```
┌─────────────────────────────────────────────────┐  
│              Smart Search 流程                   │  
├─────────────────────────────────────────────────┤  
│  1. 并行查询多个通道:                            │  
│     • $.code.classes[?(@.name ~= "keyword")]    │  
│     • $.code.functions[?(@.name ~= "keyword")]  │  
│     • $.content.heading("keyword")              │  
│     • $.entities[?(@.name ~= "keyword")]        │  
│                                                 │  
│  2. RRF (Reciprocal Rank Fusion) 融合排名        │  
│                                                 │  
│  3. Composite Scorer 评分:                       │  
│     • BM25: 词频相关性 (40%)                    │  
│     • Type: 代码实体优先 (30%)                  │  
│     • NameMatch: 名称匹配 (30%)                 │  
│                                                 │  
│  4. 返回 Top-K 结果                              │  
└─────────────────────────────────────────────────┘
```

五、文档模型[​](#五文档模型 "五、文档模型的直接链接")
-------------------------------

### 5.1 核心数据结构[​](#51-核心数据结构 "5.1 核心数据结构的直接链接")

```
// 文档文件  
data class DocumentFile(  
    val name: String,  
    val path: String,  
    val metadata: DocumentMetadata,  
    val toc: List<TOCItem>,         // 目录结构  
    val entities: List<Entity>       // 提取的实体  
)  
  
// 目录项  
data class TOCItem(  
    val level: Int,                  // 层级 (1=H1, 2=H2, ...)  
    val title: String,               // 章节标题  
    val anchor: String,              // 锚点 ID  
    val lineNumber: Int?,            // 行号  
    val children: List<TOCItem>      // 子章节  
)  
  
// 实体类型  
sealed class Entity {  
    data class Term(name, definition, location)     // 术语  
    data class API(name, signature, location)       // API  
    data class ClassEntity(name, packageName, location)  // 类  
    data class FunctionEntity(name, signature, location) // 函数  
}  
  
// 文档块（带位置信息）  
data class DocumentChunk(  
    val documentPath: String,  
    val chapterTitle: String?,  
    val content: String,  
    val position: PositionMetadata?  // 精确位置追踪  
)
```

### 5.2 位置追踪系统[​](#52-位置追踪系统 "5.2 位置追踪系统的直接链接")

```
sealed class DocumentPosition {  
    // 行范围（Markdown、源代码）  
    data class LineRange(startLine, endLine, startOffset?, endOffset?)  
  
    // 页范围（PDF）  
    data class PageRange(startPage, endPage)  
  
    // 段落位置（DOCX）  
    data class SectionRange(sectionId, paragraphIndex?)  
}  
  
// 位置元数据  
data class PositionMetadata(  
    val documentPath: String,  
    val formatType: DocumentFormatType,  
    val position: DocumentPosition  
) {  
    fun toLocationString(): String  
    // e.g., "/path/doc.md:10-15" 或 "/path/doc.pdf:page 5"  
}
```

六、多格式解析器[​](#六多格式解析器 "六、多格式解析器的直接链接")
-------------------------------------

### 6.1 解析器架构[​](#61-解析器架构 "6.1 解析器架构的直接链接")

```
interface DocumentParserService {  
    suspend fun parse(file: DocumentFile, content: String): DocumentTreeNode  
    suspend fun parseBytes(file: DocumentFile, bytes: ByteArray): DocumentTreeNode  
    suspend fun queryHeading(keyword: String): List<DocumentChunk>  
    suspend fun queryChapter(chapterId: String): DocumentChunk?  
}
```

### 6.2 支持的格式[​](#62-支持的格式 "6.2 支持的格式的直接链接")

| 格式 | 解析器 | 平台 | 特性 |
| --- | --- | --- | --- |
| Markdown (.md) | `MarkdownDocumentParser` | Common | JetBrains Markdown 库，层次化 TOC |
| PDF (.pdf) | `TikaDocumentParser` | JVM | Apache Tika，页码追踪 |
| Word (.docx/.doc) | `TikaDocumentParser` | JVM | Apache Tika，段落提取 |
| PowerPoint (.pptx) | `TikaDocumentParser` | JVM | Apache Tika |
| 源代码 (.kt/.java/...) | `CodeDocumentParser` | JVM | Tree-Sitter，类/函数提取 |

### 6.3 代码解析（Tree-Sitter）[​](#63-代码解析tree-sitter "6.3 代码解析（Tree-Sitter）的直接链接")

```
class CodeDocumentParser : DocumentParserService {  
    // 支持语言  
    val supportedLanguages = listOf(  
        Language.JAVA, Language.KOTLIN, Language.PYTHON,  
        Language.JAVASCRIPT, Language.TYPESCRIPT,  
        Language.GO, Language.RUST, Language.CSHARP  
    )  
  
    // 提取结构  
    // - 类定义 → Entity.ClassEntity + TOCItem  
    // - 函数/方法 → Entity.FunctionEntity + 嵌套 TOCItem  
    // - 完整源码保留用于 $.code.class("Name") 查询  
}
```

七、RAG 场景应用[​](#七rag-场景应用 "七、RAG 场景应用的直接链接")
-------------------------------------------

### 7.1 DocumentAgent 集成[​](#71-documentagent-集成 "7.1 DocumentAgent 集成的直接链接")

```
class DocumentAgent(  
    llmService: KoogLLMService,  
    parserService: DocumentParserService,  
    ...  
) : MainAgent<DocumentTask, ToolResult.AgentResult> {  
  
    // 注册 DocQLTool  
    init {  
        toolRegistry.registerTool(DocQLTool())  
    }  
  
    // 系统提示词包含 DocQL 使用指南  
    private suspend fun buildSystemPrompt(): String {  
        return """  
        ## Code Queries ($.code.*)  
        - $.code.class("ClassName") - Get full class source code  
        - $.code.function("funcName") - Get function implementation  
  
        ## Document Queries ($.content.*)  
        - $.content.heading("title") - Find section by heading  
        - $.toc[*] - Get table of contents  
  
        ## Smart Capabilities  
        - Automatic Summarization for large content  
        - SubAgent system for follow-up analysis  
        """.trimIndent()  
    }  
}
```

### 7.2 索引与缓存机制[​](#72-索引与缓存机制 "7.2 索引与缓存机制的直接链接")

```
┌─────────────────────────────────────────────────────────────┐  
│                    文档访问流程                              │  
├─────────────────────────────────────────────────────────────┤  
│                                                             │  
│   DocQLTool.execute()                                       │  
│         │                                                   │  
│         ▼                                                   │  
│   DocumentRegistry.queryDocument(path, query)               │  
│         │                                                   │  
│         ├── 1. 检查内存缓存                                 │  
│         │        │                                          │  
│         │        └── 命中 → 直接执行查询                    │  
│         │                                                   │  
│         ├── 2. 检查 IndexProvider (持久化存储)              │  
│         │        │                                          │  
│         │        └── 命中 → 加载到内存 → 执行查询           │  
│         │                                                   │  
│         └── 3. 未找到 → 返回错误                            │  
│                                                             │  
└─────────────────────────────────────────────────────────────┘
```

### 7.3 重排序评分模型[​](#73-重排序评分模型 "7.3 重排序评分模型的直接链接")

```
class CompositeScorer {  
    // 组合评分 = BM25 × 0.4 + TypeScore × 0.3 + NameMatch × 0.3  
  
    // BM25: 词频-逆文档频率  
    val bm25Scorer: BM25Scorer  
  
    // 类型优先级: Class > Function > Chunk > Other  
    val typeScorer: TypeScorer  
  
    // 名称匹配: 精确匹配 > 前缀匹配 > 包含匹配  
    val nameMatchScorer: NameMatchScorer  
}  
  
// RRF 融合多来源排名  
class RRFScorer<T> {  
    fun fuse(rankedLists: Map<String, List<T>>): List<ScoredItem<T>>  
    // RRF Score = Σ 1/(k + rank_i)  
}
```

八、使用场景[​](#八使用场景 "八、使用场景的直接链接")
-------------------------------

### 8.1 知识库问答[​](#81-知识库问答 "8.1 知识库问答的直接链接")

```
用户: "AuthService 的实现原理是什么？"  
  
Agent 执行流程:  
1. DocQLTool: {"query": "$.code.class(\"AuthService\")"}  
   → 返回完整的 AuthService.kt 源码  
  
2. AnalysisAgent: 自动摘要大型代码  
   → 返回结构化的类描述  
  
3. 合成回答: 结合代码和分析结果
```

### 8.2 文档导航[​](#82-文档导航 "8.2 文档导航的直接链接")

```
用户: "显示所有关于架构的章节"  
  
Agent 执行:  
1. DocQLTool: {"query": "$.content.heading(\"架构\")"}  
2. 返回匹配的章节列表和内容摘要
```

### 8.3 API 探索[​](#83-api-探索 "8.3 API 探索的直接链接")

```
用户: "有哪些跟用户相关的 API？"  
  
Agent 执行:  
1. Smart Search: {"query": "User", "maxResults": 20}  
2. 多通道并行:  
   - $.code.classes[?(@.name ~= "User")]  
   - $.code.functions[?(@.name ~= "User")]  
   - $.entities[?(@.name ~= "User")]  
3. RRF 融合 + 重排序  
4. 返回 Top-20 相关结果
```

九、设计亮点[​](#九设计亮点 "九、设计亮点的直接链接")
-------------------------------

### 9.1 轻量级查询语言[​](#91-轻量级查询语言 "9.1 轻量级查询语言的直接链接")

* **类 JSONPath 语法**：开发者熟悉，学习成本低
* **声明式查询**：表达意图而非过程
* **渐进式复杂度**：简单查询到高级过滤

### 9.2 Kotlin 多平台支持[​](#92-kotlin-多平台支持 "9.2 Kotlin 多平台支持的直接链接")

```
┌─────────────────────────────────────────┐  
│              mpp-core                    │  
│   (commonMain - 跨平台核心逻辑)          │  
│                                          │  
│   DocQL 解析器 / 执行器                  │  
│   DocumentRegistry                       │  
│   基础解析器 (Markdown)                  │  
├──────────────────┬──────────────────────┤  
│     jvmMain      │       jsMain         │  
│                  │                      │  
│ Tika (PDF/DOCX)  │   JS 导出层          │  
│ Tree-Sitter      │   JsDocumentAgent    │  
│ CodeParser       │                      │  
└──────────────────┴──────────────────────┘
```

### 9.3 智能降级策略[​](#93-智能降级策略 "9.3 智能降级策略的直接链接")

1. **精确查询** → **模糊匹配** → **全文搜索**
2. **代码结构** → **文档内容** → **全量扫描**
3. **内存缓存** → **索引加载** → **实时解析**

### 9.4 可扩展架构[​](#94-可扩展架构 "9.4 可扩展架构的直接链接")

* **解析器工厂**：`DocumentParserFactory.createParser(formatType)`
* **评分模型接口**：`ScoringModel`
* **索引提供者**：`DocumentIndexProvider`

十、总结[​](#十总结 "十、总结的直接链接")
-------------------------

DocQL 是一个为 RAG（检索增强生成）场景设计的轻量级文档查询语言，具有以下核心优势：

| 特性 | 说明 |
| --- | --- |
| **统一查询接口** | 一套语法查询文档、代码、API |
| **智能搜索** | 多级关键词扩展 + RRF 融合排序 |
| **精确定位** | 行号/页码/段落级别的位置追踪 |
| **多格式支持** | Markdown、PDF、Word、源代码 |
| **Agent 友好** | 结构化输出，易于 LLM 理解和使用 |
| **跨平台** | Kotlin Multiplatform，JVM/JS 通用 |

[编辑此页](https://github.com/unit-mesh/auto-dev/tree/master/docs/docs/mpp/autodev-mpp-docql.md)

[上一页

AutoDev Desktop](/mpp/autodev-mpp-desktop)

* [一、DocQL 概述](#一docql-概述)
  + [核心设计理念](#核心设计理念)
* [二、系统架构](#二系统架构)
  + [2.1 架构层次图](#21-架构层次图)
  + [2.2 核心组件](#22-核心组件)
* [三、查询语法详解](#三查询语法详解)
  + [3.1 语法结构 (EBNF)](#31-语法结构-ebnf)
  + [3.2 查询类型](#32-查询类型)
* [四、智能搜索机制（Smart Search）](#四智能搜索机制smart-search)
  + [4.1 多级关键词扩展策略](#41-多级关键词扩展策略)
  + [4.2 自适应搜索策略](#42-自适应搜索策略)
  + [4.3 多通道融合搜索](#43-多通道融合搜索)
* [五、文档模型](#五文档模型)
  + [5.1 核心数据结构](#51-核心数据结构)
  + [5.2 位置追踪系统](#52-位置追踪系统)
* [六、多格式解析器](#六多格式解析器)
  + [6.1 解析器架构](#61-解析器架构)
  + [6.2 支持的格式](#62-支持的格式)
  + [6.3 代码解析（Tree-Sitter）](#63-代码解析tree-sitter)
* [七、RAG 场景应用](#七rag-场景应用)
  + [7.1 DocumentAgent 集成](#71-documentagent-集成)
  + [7.2 索引与缓存机制](#72-索引与缓存机制)
  + [7.3 重排序评分模型](#73-重排序评分模型)
* [八、使用场景](#八使用场景)
  + [8.1 知识库问答](#81-知识库问答)
  + [8.2 文档导航](#82-文档导航)
  + [8.3 API 探索](#83-api-探索)
* [九、设计亮点](#九设计亮点)
  + [9.1 轻量级查询语言](#91-轻量级查询语言)
  + [9.2 Kotlin 多平台支持](#92-kotlin-多平台支持)
  + [9.3 智能降级策略](#93-智能降级策略)
  + [9.4 可扩展架构](#94-可扩展架构)
* [十、总结](#十总结)

文档

* [教程](/intro)

博客

* [博客](/blog)

社区

* [GitHub](https://github.com/unit-mesh/auto-dev)

Copyright © 2025 Unit Mesh. Built with Docusaurus.
