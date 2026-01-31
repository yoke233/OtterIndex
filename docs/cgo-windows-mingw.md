# Windows 下 cgo（tree-sitter）编译失败的排查与解决（MinGW）

## 背景

本项目的 tree-sitter 解析器启用条件是：

- Go 构建 tag：`treesitter`
- 且启用 CGO：`CGO_ENABLED=1`

在 Windows 环境下，通常使用 MinGW（gcc/g++）作为 C 工具链。

## 典型报错现象

执行：

```powershell
go test -tags treesitter ./...
```

可能出现类似信息（没有更详细的 gcc 错误输出）：

```
github.com/tree-sitter/tree-sitter-xxx/bindings/go: ...\cgo.exe: exit status 2
FAIL  otterindex/internal/core/treesitter [build failed]
```

这类信息的特点是：**Go 侧只看到 cgo 退出码**，但看不到“真正是哪一步 gcc/ld/as 出错”。

## 当时的根因（本仓库遇到的情况）

当时已经显式设置了：

- `CGO_ENABLED=1`
- `CC=D:\dev\Scoop\apps\mingw\current\bin\gcc.exe`
- `CXX=D:\dev\Scoop\apps\mingw\current\bin\g++.exe`

但依旧失败。最终发现问题点在于：**`PATH` 里 MinGW 的 `bin` 目录没有置顶**（且环境中还存在其它可能干扰的工具链路径，比如 Conda 的 mingw-w64 / 其他 gcc 相关路径）。

在 Windows 上，哪怕 `CC/CXX` 指向了正确的 `gcc.exe`，gcc 在内部仍可能需要调用同目录或 `PATH` 上的其它工具/依赖（如 `as.exe`、`ld.exe`、运行时 DLL 等）。如果 `PATH` 优先级不对，就可能出现“返回非 0，但日志很少/没有”的情况，最终在 Go 侧只表现为 `cgo.exe: exit status 2`。

## 解决办法（对本仓库有效）

核心是两点：

1) **把 MinGW 的 `bin` 放到 `PATH` 最前面**

```powershell
$env:Path = 'D:\dev\Scoop\apps\mingw\current\bin;' + $env:Path
```

2) **明确开启 CGO 并指定 CC/CXX**

```powershell
$env:CGO_ENABLED = '1'
$env:CC  = 'D:\dev\Scoop\apps\mingw\current\bin\gcc.exe'
$env:CXX = 'D:\dev\Scoop\apps\mingw\current\bin\g++.exe'
```

然后重试：

```powershell
go test -tags treesitter ./... -v
```

## 推荐用法：直接使用脚本

仓库里已有脚本会自动做“找 gcc + 调整 PATH + 设置 CC/CXX”等工作：

- `scripts/test-treesitter.ps1`

示例：

```powershell
pwsh -NoProfile -File scripts/test-treesitter.ps1
```

（脚本内部的 `Ensure-Gcc` 会尝试把常见的 MinGW 路径加入 `PATH`）

## 排查手段（需要更详细日志时）

1) 看 go/cgo 是否真的启用：

```powershell
go env CGO_ENABLED
go env CC
go env CXX
```

2) 打印 Go 构建的底层执行细节（能看到 cgo 调用的命令）：

```powershell
go test -x -work -tags treesitter ./internal/core/treesitter
```

3) 快速确认当前实际使用的是哪个 gcc：

```powershell
Get-Command gcc
Get-Command g++
gcc --version
```

## 备注

- 如果你在 PowerShell 里“直接运行 `cc1.exe -v`”，可能会看到类似 `cannot open '<stdin>.s' for writing` 的信息，这是因为它默认会尝试生成 `<stdin>.s` 这种在 Windows 文件系统里不合法的文件名；这条**不等价于**项目编译失败的根因，排查时不要被它误导。

