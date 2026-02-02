[CmdletBinding()]
param(
  [int]$Limit = 20,
  [int]$Repeat = 3,
  [int]$BuildRepeat = 1,
  [string]$OutDir = (Join-Path $PSScriptRoot 'out'),
  [string]$MdPath = (Join-Path $PSScriptRoot 'docs\external-projects.md'),
  [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$RepoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path

function Write-Step {
  param([Parameter(Mandatory)][string]$Title)
  Write-Host "`n== $Title ==" -ForegroundColor Cyan
}

function Add-Line {
  param([Parameter(Mandatory)][AllowEmptyString()][string]$Text)
  $script:lines += $Text
}

function Ensure-GccInPath {
  if (Get-Command gcc -ErrorAction SilentlyContinue) { return }

  $scoop = Get-Command scoop -ErrorAction SilentlyContinue
  if ($scoop) {
    try {
      $prefix = & scoop prefix mingw 2>$null
      if ($prefix) {
        $bin = Join-Path $prefix 'bin'
        if (Test-Path -LiteralPath $bin) {
          $env:Path = "$bin;$env:Path"
        }
      }
    } catch {
      # ignore
    }
  }

  foreach ($bin in @(
    'D:\dev\Scoop\apps\mingw\current\bin',
    (Join-Path $env:USERPROFILE 'scoop\apps\mingw\current\bin')
  )) {
    if (Test-Path -LiteralPath $bin) {
      $env:Path = "$bin;$env:Path"
    }
  }
}

function New-ResultPath {
  param([Parameter(Mandatory)][string]$OutDir)
  $date = Get-Date -Format 'yyyy-MM-dd'
  $p = Join-Path $OutDir "result-$date.txt"
  if (-not (Test-Path -LiteralPath $p)) {
    return $p
  }
  $ts = Get-Date -Format 'HHmmss'
  return (Join-Path $OutDir "result-$date-$ts.txt")
}

function Read-ExplainJson {
  param([Parameter(Mandatory)][string]$Path)
  if (-not (Test-Path -LiteralPath $Path)) {
    return $null
  }
  $raw = Get-Content -LiteralPath $Path -ErrorAction SilentlyContinue
  if (-not $raw) {
    return $null
  }
  $line = $raw | Where-Object { $_.TrimStart().StartsWith('{') } | Select-Object -Last 1
  if (-not $line) {
    return $null
  }
  try {
    return ($line | ConvertFrom-Json)
  } catch {
    return $null
  }
}

function Get-PropValue {
  param(
    [Parameter(Mandatory)][AllowNull()][object]$Obj,
    [Parameter(Mandatory)][string]$Name
  )
  if ($null -eq $Obj) {
    return $null
  }
  if ($Obj -is [System.Collections.IDictionary]) {
    if ($Obj.Contains($Name)) {
      return $Obj[$Name]
    }
    return $null
  }
  $p = $Obj.PSObject.Properties[$Name]
  if ($p) {
    return $p.Value
  }
  return $null
}

function Measure-Min {
  param(
    [Parameter(Mandatory)][int]$Repeat,
    [Parameter(Mandatory)][scriptblock]$Run
  )
  $best = $null
  for ($i = 0; $i -lt $Repeat; $i++) {
    $r = & $Run
    if ($null -eq $best -or $r.WallMs -lt $best.WallMs) {
      $best = $r
    }
  }
  return $best
}

function Get-LoadStats {
  param([Parameter(Mandatory)][int[]]$Values)
  if (-not $Values -or $Values.Count -eq 0) {
    return $null
  }
  $sorted = $Values | Sort-Object
  $min = $sorted[0]
  $max = $sorted[$sorted.Count - 1]
  $mid = [int]([math]::Floor($sorted.Count / 2))
  if ($sorted.Count % 2 -eq 1) {
    $median = $sorted[$mid]
  } else {
    $median = [int][math]::Round(($sorted[$mid - 1] + $sorted[$mid]) / 2.0)
  }
  return [pscustomobject]@{
    Min    = $min
    Max    = $max
    Median = $median
  }
}

function Build-OtidxTreesitter {
  param([Parameter(Mandatory)][string]$OutPath)

  $outDir = Split-Path -Parent $OutPath
  if ($outDir) {
    New-Item -ItemType Directory -Path $outDir -Force | Out-Null
  }

  $elapsed = Measure-Command {
    go build -tags treesitter -o $OutPath ./cmd/otidx | Out-Null
  }
  if ($LASTEXITCODE -ne 0) {
    throw "go build 失败（exit=$LASTEXITCODE）"
  }

  return [pscustomobject]@{
    ExePath = (Resolve-Path -LiteralPath $OutPath).Path
    WallMs  = [int]$elapsed.TotalMilliseconds
  }
}

function Run-Otidx {
  param(
    [Parameter(Mandatory)][string]$ExePath,
    [Parameter(Mandatory)][string]$WorkingDir,
    [Parameter(Mandatory)][string[]]$CmdArgs
  )

  $stdout = New-TemporaryFile
  $stderr = New-TemporaryFile
  try {
    $elapsed = Measure-Command {
      Push-Location -LiteralPath $WorkingDir
      try {
        & $ExePath @CmdArgs 1> $stdout.FullName 2> $stderr.FullName
      } finally {
        Pop-Location
      }
    }
    $exit = $LASTEXITCODE
    if ($exit -ne 0) {
      $err = (Get-Content -LiteralPath $stderr.FullName -ErrorAction SilentlyContinue) -join "`n"
      throw "otidx 失败（exit=$exit）`n$err"
    }

    $ex = Read-ExplainJson -Path $stderr.FullName
    $first = (Get-Content -LiteralPath $stdout.FullName -ErrorAction SilentlyContinue | Select-Object -First 1)

    return [pscustomobject]@{
      WallMs  = [int]$elapsed.TotalMilliseconds
      Explain = $ex
      First   = $first
    }
  } finally {
    Remove-Item -LiteralPath $stdout.FullName -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $stderr.FullName -Force -ErrorAction SilentlyContinue
  }
}

function Run-RgFirstN {
  param(
    [Parameter(Mandatory)][string]$Root,
    [Parameter(Mandatory)][string]$Pattern,
    [Parameter(Mandatory)][string[]]$Globs,
    [Parameter(Mandatory)][int]$Limit,
    [Parameter(Mandatory)][int]$Repeat
  )

  return (Measure-Min -Repeat $Repeat -Run {
    $elapsed = $null
    $count = 0
    $elapsed = Measure-Command {
      Push-Location -LiteralPath $Root
      try {
        $args = @('--no-heading', '--color', 'never', '-n', '--fixed-strings')
        foreach ($g in $Globs) {
          $args += @('-g', $g)
        }
        $res = & rg @args $Pattern . | Select-Object -First $Limit
        $count = @($res).Count
      } finally {
        Pop-Location
      }
    }
    $exit = $LASTEXITCODE
    if ($exit -gt 1) {
      throw "rg 失败（exit=$exit）"
    }

    [pscustomobject]@{
      WallMs = [int]$elapsed.TotalMilliseconds
      Count  = $count
    }
  })
}

Set-Location -LiteralPath $RepoRoot

if ($OutDir) {
  New-Item -ItemType Directory -Path $OutDir -Force | Out-Null
}

$lines = @()
$resultPath = New-ResultPath -OutDir $OutDir

Add-Line ("time: {0}" -f (Get-Date).ToString('yyyy-MM-dd HH:mm:ss'))
Add-Line ("kind: external-projects")
Add-Line ("repeat: {0}" -f $Repeat)
Add-Line ("build_repeat: {0}" -f $BuildRepeat)
Add-Line ("limit: {0}" -f $Limit)
Add-Line ""

Write-Step '准备 cgo + treesitter（gcc）'
$env:CGO_ENABLED = '1'
Ensure-GccInPath
if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
  throw "未找到 gcc（MinGW）。建议：scoop install mingw"
}
$env:CC = (Get-Command gcc).Source
$env:CXX = (Get-Command g++).Source
Add-Line ("CGO_ENABLED={0}" -f $env:CGO_ENABLED)
Add-Line ("CC={0}" -f $env:CC)
Add-Line ("CXX={0}" -f $env:CXX)
Add-Line ""

Write-Step '准备 rg'
$rg = Get-Command rg -ErrorAction SilentlyContinue
if (-not $rg) {
  throw "未找到 rg（ripgrep）。"
}
$rgVersion = (& rg --version | Select-Object -First 1)
Add-Line ("rg_version: {0}" -f $rgVersion)
Add-Line ""

Write-Step '编译 otidx（treesitter）'
$build = Build-OtidxTreesitter -OutPath '.otidx/bin/otidx-ts.exe'
Add-Line ("build_otidx_treesitter_ms: {0}" -f $build.WallMs)
Add-Line ("otidx_exe: {0}" -f $build.ExePath)
Add-Line ""

$projects = @(
  [pscustomobject]@{
    Key         = 'java-jeecg-boot'
    ChartName   = 'java'
    Title       = 'Java / Spring（JeecgBoot - jeecg-boot）'
    Root        = 'D:\project\JeecgBoot\jeecg-boot'
    DbPath      = '.otidx/ext/java-jeecg-boot.db'
    IncludeGlobs = @('*.java', '*.xml', '*.yml', '*.yaml', '*.properties')
    Cases       = @(
      [pscustomobject]@{ Name = 'RestController (line)'; Query = 'RestController'; Unit = 'line'; Globs = @('*.java') },
      [pscustomobject]@{ Name = 'RequestMapping (line)'; Query = 'RequestMapping'; Unit = 'line'; Globs = @('*.java') },
      [pscustomobject]@{ Name = 'public class (line)'; Query = 'public class'; Unit = 'line'; Globs = @('*.java') },
      [pscustomobject]@{ Name = 'throw new (line)'; Query = 'throw new'; Unit = 'line'; Globs = @('*.java') },
      [pscustomobject]@{ Name = 'RestController (symbol, fallback)'; Query = 'RestController'; Unit = 'symbol'; Globs = @('*.java') }
    )
  },
  [pscustomobject]@{
    Key         = 'vue-jeecgboot-vue3'
    ChartName   = 'vue'
    Title       = 'Vue/TS（JeecgBoot - jeecgboot-vue3）'
    Root        = 'D:\project\JeecgBoot\jeecgboot-vue3'
    DbPath      = '.otidx/ext/vue-jeecgboot-vue3.db'
    IncludeGlobs = @('*.vue', '*.ts', '*.tsx', '*.js', '*.jsx', '*.css', '*.scss', '*.json')
    Cases       = @(
      [pscustomobject]@{ Name = 'defineComponent (line)'; Query = 'defineComponent'; Unit = 'line'; Globs = @('*.vue', '*.ts', '*.tsx') },
      [pscustomobject]@{ Name = 'export default (line)'; Query = 'export default'; Unit = 'line'; Globs = @('*.vue', '*.ts', '*.tsx', '*.js', '*.jsx') },
      [pscustomobject]@{ Name = 'axios (line)'; Query = 'axios'; Unit = 'line'; Globs = @('*.vue', '*.ts', '*.tsx', '*.js', '*.jsx') },
      [pscustomobject]@{ Name = 'router (line)'; Query = 'router'; Unit = 'line'; Globs = @('*.vue', '*.ts', '*.tsx', '*.js', '*.jsx') },
      [pscustomobject]@{ Name = 'defineComponent (symbol, fallback)'; Query = 'defineComponent'; Unit = 'symbol'; Globs = @('*.vue', '*.ts', '*.tsx') }
    )
  },
  [pscustomobject]@{
    Key         = 'python-crawl4ai'
    ChartName   = 'python'
    Title       = 'Python（crawl4ai）'
    Root        = 'D:\project\crawl4ai'
    DbPath      = '.otidx/ext/python-crawl4ai.db'
    IncludeGlobs = @('*.py', '*.toml', '*.yml', '*.yaml', '*.md')
    Cases       = @(
      [pscustomobject]@{ Name = 'async def (line)'; Query = 'async def'; Unit = 'line'; Globs = @('*.py') },
      [pscustomobject]@{ Name = 'def (line)'; Query = 'def'; Unit = 'line'; Globs = @('*.py') },
      [pscustomobject]@{ Name = 'class (line)'; Query = 'class'; Unit = 'line'; Globs = @('*.py') },
      [pscustomobject]@{ Name = 'import (line)'; Query = 'import'; Unit = 'line'; Globs = @('*.py') },
      [pscustomobject]@{ Name = 'async def (symbol, fallback)'; Query = 'async def'; Unit = 'symbol'; Globs = @('*.py') }
    )
  },
  [pscustomobject]@{
    Key         = 'python-paddleocr'
    ChartName   = 'python-paddleocr'
    Title       = 'Python/ML（PaddleOCR）'
    Root        = 'D:\project\PaddleOCR'
    DbPath      = '.otidx/ext/python-paddleocr.db'
    IncludeGlobs = @('*.py', '*.toml', '*.yml', '*.yaml', '*.md')
    Cases       = @(
      [pscustomobject]@{ Name = 'import paddle (line)'; Query = 'import paddle'; Unit = 'line'; Globs = @('*.py') },
      [pscustomobject]@{ Name = 'paddle nn (line)'; Query = 'paddle nn'; Unit = 'line'; Globs = @('*.py') },
      [pscustomobject]@{ Name = 'def (line)'; Query = 'def'; Unit = 'line'; Globs = @('*.py') },
      [pscustomobject]@{ Name = 'class (line)'; Query = 'class'; Unit = 'line'; Globs = @('*.py') },
      [pscustomobject]@{ Name = 'import paddle (symbol, fallback)'; Query = 'import paddle'; Unit = 'symbol'; Globs = @('*.py') }
    )
  },
  [pscustomobject]@{
    Key         = 'cpp-gdmplab'
    ChartName   = 'cpp'
    Title       = 'C/C++（gdmplab）'
    Root        = 'D:\project\gdmplab'
    DbPath      = '.otidx/ext/cpp-gdmplab.db'
    IncludeGlobs = @('*.c', '*.cc', '*.cpp', '*.cxx', '*.h', '*.hpp', 'CMakeLists.txt', '*.cmake')
    Cases       = @(
      [pscustomobject]@{ Name = 'include (line)'; Query = 'include'; Unit = 'line'; Globs = @('*.c', '*.cc', '*.cpp', '*.cxx', '*.h', '*.hpp') },
      [pscustomobject]@{ Name = 'namespace (line)'; Query = 'namespace'; Unit = 'line'; Globs = @('*.c', '*.cc', '*.cpp', '*.cxx', '*.h', '*.hpp') },
      [pscustomobject]@{ Name = 'template (line)'; Query = 'template'; Unit = 'line'; Globs = @('*.c', '*.cc', '*.cpp', '*.cxx', '*.h', '*.hpp') },
      [pscustomobject]@{ Name = 'add_executable (line)'; Query = 'add_executable'; Unit = 'line'; Globs = @('CMakeLists.txt', '*.cmake') },
      [pscustomobject]@{ Name = 'include (symbol, fallback)'; Query = 'include'; Unit = 'symbol'; Globs = @('*.c', '*.cc', '*.cpp', '*.cxx', '*.h', '*.hpp') }
    )
  }
)

$md = @()
$md += '# 外部项目基准测试（otidx vs rg）'
$md += ''
$md += ('更新时间：{0}' -f (Get-Date).ToString('yyyy-MM-dd HH:mm:ss'))
$md += ''
$md += '说明：'
$md += '- otidx 走 SQLite/FTS 索引；rg 为直接扫描文件。'
$md += ('- 数值为脚本取多次运行中的最小 wall time（ms）。repeat={0}，limit={1}。' -f $Repeat, $Limit)
$md += '- 图表内 otidx 使用“查询耗时（wall - 加载）”；加载时间在图表顶部单独标注。'
$md += '- 当前 tree-sitter 只接入 Go；非 Go 工程里 `--unit symbol` 会自动降级（fallback）。'
$md += ''

foreach ($p in $projects) {
  $root = $p.Root
  if (-not (Test-Path -LiteralPath $root)) {
    Write-Host "跳过（路径不存在）：$root" -ForegroundColor Yellow
    continue
  }

  Write-Step ("项目：{0}" -f $p.Title)
  $rootAbs = (Resolve-Path -LiteralPath $root).Path
  $dbAbs = $p.DbPath
  if (-not [System.IO.Path]::IsPathRooted($dbAbs)) {
    $dbAbs = Join-Path $RepoRoot $dbAbs
  }
  $dbAbs = [System.IO.Path]::GetFullPath($dbAbs)
  $dbDir = Split-Path -Parent $dbAbs
  if ($dbDir) {
    New-Item -ItemType Directory -Path $dbDir -Force | Out-Null
  }

  $buildExplain = $null
  $buildWall = $null
  if (-not $SkipBuild) {
    Write-Step ("index build：{0}" -f $p.Key)
    $buildArgs = @('--no-banner', '--database', $dbAbs, '--explain=json')
    foreach ($g in $p.IncludeGlobs) {
      $buildArgs += @('--glob', $g)
    }
    $buildArgs += @('index', 'build', $rootAbs)

    $bestBuild = Measure-Min -Repeat $BuildRepeat -Run { Run-Otidx -ExePath $build.ExePath -WorkingDir $RepoRoot -CmdArgs $buildArgs }
    $buildExplain = $bestBuild.Explain
    $buildWall = $bestBuild.WallMs

    Add-Line ("--- {0} / index.build ---" -f $p.Key)
    Add-Line ("project: {0}" -f $p.Key)
    Add-Line ("project_title: {0}" -f $p.Title)
    Add-Line ("root: {0}" -f $rootAbs)
    Add-Line ("db: {0}" -f $p.DbPath)
    Add-Line ("db_abs: {0}" -f $dbAbs)
    Add-Line ("wall_ms_min: {0}" -f $bestBuild.WallMs)
    if ($buildExplain) {
      foreach ($k in @('files_total', 'files_indexed', 'chunks_written', 'symbols_written', 'comments_written', 'treesitter_unsupported', 'treesitter_errors', 'fts5', 'fts5_reason')) {
        $v = Get-PropValue -Obj $buildExplain -Name $k
        if ($v -ne $null) { Add-Line ("{0}: {1}" -f $k, $v) }
      }
      $t = Get-PropValue -Obj $buildExplain -Name 'timings_ms'
      if ($t) {
        foreach ($k in @('walk', 'read_parse', 'write')) {
          $v = Get-PropValue -Obj $t -Name $k
          if ($v -ne $null) { Add-Line ("ex_elapsed_ms_{0}: {1}" -f $k, $v) }
        }
      }
    }
    Add-Line ""
  }

  $md += ('## {0}' -f $p.Title)
  $md += ''
  $md += ('![{0} 速度对比](external-projects-{1}.svg)' -f $p.Title, $p.ChartName)
  $md += ''
  $md += ('- root: `{0}`' -f $rootAbs)
  $md += ('- db: `{0}`' -f $p.DbPath)
  $md += ('- include globs: `{0}`' -f (($p.IncludeGlobs) -join ', '))
  if ($buildWall -ne $null) {
    $ft = Get-PropValue -Obj $buildExplain -Name 'files_total'
    $fi = Get-PropValue -Obj $buildExplain -Name 'files_indexed'
    $cw = Get-PropValue -Obj $buildExplain -Name 'chunks_written'
    $su = Get-PropValue -Obj $buildExplain -Name 'symbols_written'
    $tu = Get-PropValue -Obj $buildExplain -Name 'treesitter_unsupported'
    $fts = Get-PropValue -Obj $buildExplain -Name 'fts5'
    $md += ('- index.build: `{0}ms`（files_total={1} files_indexed={2} chunks_written={3} symbols_written={4} treesitter_unsupported={5} fts5={6}）' -f $buildWall, $ft, $fi, $cw, $su, $tu, $fts)
  }
  $md += ''
  $md += '| case | unit | globs | otidx(ms) | rg(ms) | 说明 |'
  $md += '|---|---|---|---:|---:|---|'

  $loadSamples = @()
  foreach ($c in $p.Cases) {
    $otArgs = @('--no-banner', '--database', $dbAbs, '--explain=json', 'q', $c.Query, '--unit', $c.Unit, '--limit', "$Limit", '--compact')
    foreach ($g in $c.Globs) {
      $otArgs += @('--glob', $g)
    }

    $otBest = Measure-Min -Repeat $Repeat -Run { Run-Otidx -ExePath $build.ExePath -WorkingDir $rootAbs -CmdArgs $otArgs }
    $rgBest = Run-RgFirstN -Root $rootAbs -Pattern $c.Query -Globs $c.Globs -Limit $Limit -Repeat $Repeat

    $noteParts = @()
    $ex = $otBest.Explain
    if ($ex) {
      $sf = Get-PropValue -Obj $ex -Name 'symbol_fallback'
      $uf = Get-PropValue -Obj $ex -Name 'unit_fallback'
      if ($sf -ne $null -and [int]$sf -ne 0) { $noteParts += 'symbol_fallback' }
      if ($uf) { $noteParts += ("unit_fallback={0}" -f $uf) }
      $hit = Get-PropValue -Obj $ex -Name 'cache_hit'
      if ($hit -ne $null -and [int]$hit -ne 0) { $noteParts += 'cache_hit' }
    }
    $note = ($noteParts -join '; ')

    Add-Line ("--- {0} / {1} ---" -f $p.Key, $c.Name)
    Add-Line ("project: {0}" -f $p.Key)
    Add-Line ("project_title: {0}" -f $p.Title)
    Add-Line ("root: {0}" -f $rootAbs)
    Add-Line ("db: {0}" -f $p.DbPath)
    Add-Line ("db_abs: {0}" -f $dbAbs)
    Add-Line ("unit: {0}" -f $c.Unit)
    Add-Line ("query: {0}" -f $c.Query)
    Add-Line ("otidx_wall_ms_min: {0}" -f $otBest.WallMs)
    if ($ex) {
      $totalMs = Get-PropValue -Obj $ex -Name 'elapsed_ms_total'
      if ($totalMs -ne $null) { Add-Line ("otidx_ex_elapsed_ms_total: {0}" -f $totalMs) }
      $t = Get-PropValue -Obj $ex -Name 'timings_ms'
      foreach ($k in @('sql', 'match', 'unitize', 'symbol', 'file_read')) {
        $v = Get-PropValue -Obj $t -Name $k
        if ($v -ne $null) { Add-Line ("otidx_ex_elapsed_ms_{0}: {1}" -f $k, $v) }
      }
      foreach ($k in @('rows_returned', 'items_returned', 'symbol_fallback', 'unit_fallback', 'cache_hit')) {
        $v = Get-PropValue -Obj $ex -Name $k
        if ($v -ne $null) { Add-Line ("otidx_{0}: {1}" -f $k, $v) }
      }
    }
    Add-Line ("rg_wall_ms_min: {0}" -f $rgBest.WallMs)
    Add-Line ("rg_lines_returned: {0}" -f $rgBest.Count)
    Add-Line ""

    if ($ex) {
      $totalMs = Get-PropValue -Obj $ex -Name 'elapsed_ms_total'
      if ($totalMs -ne $null) {
        $load = [int]$otBest.WallMs - [int]$totalMs
        if ($load -lt 0) { $load = 0 }
        $loadSamples += $load
      }
    }

    $md += ('| {0} | {1} | {2} | {3} | {4} | {5} |' -f $c.Name, $c.Unit, (($c.Globs) -join ', '), $otBest.WallMs, $rgBest.WallMs, $note)
  }

  if ($loadSamples.Count -gt 0) {
    $stats = Get-LoadStats -Values $loadSamples
    if ($stats) {
      $md += ''
      $md += ('- 加载时间（otidx wall - query）：中位 {0}ms（min {1} / max {2}）' -f $stats.Median, $stats.Min, $stats.Max)
    }
  }

  $md += ''
}

Set-Content -LiteralPath $resultPath -Value $lines -Encoding UTF8

$svgDir = Split-Path -Parent $MdPath
$svgPath = if ($svgDir) { Join-Path $svgDir 'external-projects-vs-rg.svg' } else { 'external-projects-vs-rg.svg' }
try {
  if (Get-Command python -ErrorAction SilentlyContinue) {
    if ($svgDir) { New-Item -ItemType Directory -Path $svgDir -Force | Out-Null }
    foreach ($p in $projects) {
      $outName = 'external-projects-{0}.svg' -f $p.ChartName
      $outPath = if ($svgDir) { Join-Path $svgDir $outName } else { $outName }
      Write-Step ("生成 SVG 图表：{0}" -f $p.Key)
      python (Join-Path $PSScriptRoot 'plot_bench_svg.py') --in $resultPath --out $outPath --project $p.Key | Out-Null
    }
  } else {
    Write-Host '未找到 python，跳过生成 SVG。' -ForegroundColor Yellow
  }
} catch {
  Write-Host "生成 SVG 失败：$($_.Exception.Message)" -ForegroundColor Yellow
}

$md += '---'
$md += ''
$md += ('原始数据：`{0}`' -f (Split-Path -Leaf $resultPath))
$md += ''

$mdDir = Split-Path -Parent $MdPath
if ($mdDir) {
  New-Item -ItemType Directory -Path $mdDir -Force | Out-Null
}
Set-Content -LiteralPath $MdPath -Value $md -Encoding UTF8

Write-Host "`n已输出：" -ForegroundColor Green
Write-Host "  $resultPath" -ForegroundColor Green
Write-Host "  $MdPath" -ForegroundColor Green
if (Test-Path -LiteralPath $svgPath) {
  Write-Host "  $svgPath" -ForegroundColor Green
}
