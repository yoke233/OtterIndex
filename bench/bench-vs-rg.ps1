[CmdletBinding()]
param(
  [string]$Root = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path,
  [string]$DbPath = '.otidx/index.db',
  [string]$OutDir = (Join-Path $PSScriptRoot 'out'),
  [switch]$Rebuild,
  [int]$Limit = 20,
  [int]$Repeat = 3
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

Set-Location -LiteralPath $RepoRoot

if ($OutDir) {
  New-Item -ItemType Directory -Path $OutDir -Force | Out-Null
}

$lines = @()
$resultPath = New-ResultPath -OutDir $OutDir

Add-Line ("time: {0}" -f (Get-Date).ToString('yyyy-MM-dd HH:mm:ss'))
Add-Line ("root: {0}" -f (Resolve-Path -LiteralPath $Root).Path)
Add-Line ("db: {0}" -f $DbPath)
Add-Line ("limit: {0}" -f $Limit)
Add-Line ("repeat: {0}" -f $Repeat)
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

function Run-Otidx {
  param(
    [Parameter(Mandatory)][string]$Name,
    [Parameter(Mandatory)][string[]]$CmdArgs
  )

  $stdout = New-TemporaryFile
  $stderr = New-TemporaryFile
  try {
    $elapsed = Measure-Command {
      & $build.ExePath @CmdArgs 1> $stdout.FullName 2> $stderr.FullName
    }
    $exit = $LASTEXITCODE
    if ($exit -ne 0) {
      $err = (Get-Content -LiteralPath $stderr.FullName -ErrorAction SilentlyContinue) -join "`n"
      throw "$Name 失败（exit=$exit）`n$err"
    }

    $ex = Read-ExplainJson -Path $stderr.FullName
    $first = (Get-Content -LiteralPath $stdout.FullName -ErrorAction SilentlyContinue | Select-Object -First 1)

    return [pscustomobject]@{
      WallMs = [int]$elapsed.TotalMilliseconds
      Explain = $ex
      First = $first
    }
  } finally {
    Remove-Item -LiteralPath $stdout.FullName -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $stderr.FullName -Force -ErrorAction SilentlyContinue
  }
}

function Run-RgFirstN {
  param(
    [Parameter(Mandatory)][string]$Pattern,
    [Parameter(Mandatory)][string[]]$Globs,
    [Parameter(Mandatory)][int]$Limit,
    [Parameter(Mandatory)][int]$Repeat
  )

  return (Measure-Min -Repeat $Repeat -Run {
    $elapsed = $null
    $count = 0
    $elapsed = Measure-Command {
      $args = @('--no-heading', '--color', 'never', '-n', '--fixed-strings')
      foreach ($g in $Globs) {
        $args += @('-g', $g)
      }
      $res = & rg @args $Pattern $Root | Select-Object -First $Limit
      $count = @($res).Count
    }
    $exit = $LASTEXITCODE
    if ($exit -gt 1) {
      throw "rg 失败（exit=$exit）"
    }

    [pscustomobject]@{
      WallMs = [int]$elapsed.TotalMilliseconds
      Count = $count
    }
  })
}

if ($Rebuild) {
  Write-Step 'Build 索引（index build）'
  $r = Measure-Min -Repeat $Repeat -Run {
    Run-Otidx -Name 'index.build' -CmdArgs @(
      '--no-banner', '--database', $DbPath, '--explain=json',
      '--exclude', '*.ps1,result-*.txt',
      'index', 'build', $Root
    ) | ForEach-Object {
      [pscustomobject]@{ WallMs = $_.WallMs; Explain = $_.Explain; First = $_.First }
    }
  }

  Add-Line ("--- index.build (otidx) ---")
  Add-Line ("wall_ms_min: {0}" -f $r.WallMs)
  $ex = $r.Explain
  if ($ex) {
    $t = Get-PropValue -Obj $ex -Name 'timings_ms'
    foreach ($k in @('walk', 'read_parse', 'write')) {
      $v = Get-PropValue -Obj $t -Name $k
      if ($v -ne $null) { Add-Line ("ex_elapsed_ms_{0}: {1}" -f $k, $v) }
    }
    foreach ($k in @('files_total', 'files_indexed', 'chunks_written', 'symbols_written', 'comments_written', 'treesitter_unsupported', 'treesitter_errors')) {
      $v = Get-PropValue -Obj $ex -Name $k
      if ($v -ne $null) { Add-Line ("{0}: {1}" -f $k, $v) }
    }
  }
  Add-Line ""
}

Write-Step 'Query 对比（otidx vs rg）'
$cases = @(
  [pscustomobject]@{ Name = 'ReplaceChunksBatch'; Query = 'ReplaceChunksBatch'; Unit = 'symbol'; Globs = @('*.go') },
  [pscustomobject]@{ Name = 'NewQueryCache'; Query = 'NewQueryCache'; Unit = 'symbol'; Globs = @('*.go') },
  [pscustomobject]@{ Name = 'if err != nil'; Query = 'if err != nil'; Unit = 'line'; Globs = @('*.go') },
  [pscustomobject]@{ Name = 'BEGIN IMMEDIATE'; Query = 'BEGIN IMMEDIATE'; Unit = 'block'; Globs = @('*.go') },
  [pscustomobject]@{ Name = 'CREATE TABLE'; Query = 'CREATE TABLE'; Unit = 'block'; Globs = @('*.sql') },
  [pscustomobject]@{ Name = 'SELECT'; Query = 'SELECT'; Unit = 'line'; Globs = @('*.go') }
)

foreach ($c in $cases) {
  $otidxArgs = @('--no-banner', '--database', $DbPath, '--explain=json', 'q', $c.Query, '--unit', $c.Unit, '--limit', "$Limit", '--compact')
  foreach ($g in $c.Globs) {
    $otidxArgs += @('--glob', $g)
  }

  $otBest = Measure-Min -Repeat $Repeat -Run { Run-Otidx -Name "q.$($c.Name)" -CmdArgs $otidxArgs }
  $rgBest = Run-RgFirstN -Pattern $c.Query -Globs $c.Globs -Limit $Limit -Repeat $Repeat

  Add-Line ("--- {0} ---" -f $c.Name)
  Add-Line ("otidx_wall_ms_min: {0}" -f $otBest.WallMs)
  $ex = $otBest.Explain
  if ($ex) {
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
}

Set-Content -LiteralPath $resultPath -Value $lines -Encoding UTF8
Write-Host "`n已输出：$resultPath" -ForegroundColor Green
