[CmdletBinding()]
param(
  [string]$Root = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path,
  [string]$DbPath = '.otidx/index.db',
  [string]$OutDir = (Join-Path $PSScriptRoot 'out'),
  [switch]$Rebuild,
  [int]$Limit = 20
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

function Invoke-External {
  param(
    [Parameter(Mandatory)][string]$FilePath,
    [Parameter()][string[]]$CmdArgs = @(),
    [Parameter(Mandatory)][string]$StdoutPath,
    [Parameter(Mandatory)][string]$StderrPath
  )

  $elapsed = Measure-Command {
    & $FilePath @CmdArgs 1> $StdoutPath 2> $StderrPath
  }
  $exit = $LASTEXITCODE
  return [pscustomobject]@{
    ExitCode = $exit
    WallMs   = [int]$elapsed.TotalMilliseconds
  }
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

function Build-OtidxTreesitter {
  param([Parameter(Mandatory)][string]$OutPath)
  $outDir = Split-Path -Parent $OutPath
  if ($outDir) {
    New-Item -ItemType Directory -Path $outDir -Force | Out-Null
  }

  $stdout = New-TemporaryFile
  $stderr = New-TemporaryFile
  try {
    $r = Invoke-External -FilePath 'go' -CmdArgs @('build', '-tags', 'treesitter', '-o', $OutPath, './cmd/otidx') -StdoutPath $stdout.FullName -StderrPath $stderr.FullName
    if ($r.ExitCode -ne 0) {
      $err = (Get-Content -LiteralPath $stderr.FullName -ErrorAction SilentlyContinue) -join "`n"
      throw "go build 失败（exit=$($r.ExitCode)）`n$err"
    }
    return [pscustomobject]@{
      ExePath = (Resolve-Path -LiteralPath $OutPath).Path
      WallMs  = $r.WallMs
    }
  } finally {
    Remove-Item -LiteralPath $stdout.FullName -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $stderr.FullName -Force -ErrorAction SilentlyContinue
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

Write-Step '编译 otidx（treesitter）'
$build = Build-OtidxTreesitter -OutPath '.otidx/bin/otidx-ts.exe'
Add-Line ("build_otidx_treesitter_ms: {0}" -f $build.WallMs)
Add-Line ("otidx_exe: {0}" -f $build.ExePath)
Add-Line ""

function Run-Case {
  param(
    [Parameter(Mandatory)][string]$Name,
    [Parameter(Mandatory)][string[]]$CmdArgs
  )

  $stdout = New-TemporaryFile
  $stderr = New-TemporaryFile
  try {
    $r = Invoke-External -FilePath $build.ExePath -CmdArgs $CmdArgs -StdoutPath $stdout.FullName -StderrPath $stderr.FullName
    if ($r.ExitCode -ne 0) {
      $errText = (Get-Content -LiteralPath $stderr.FullName -ErrorAction SilentlyContinue) -join "`n"
      throw "$Name 失败（exit=$($r.ExitCode)）`n$errText"
    }

    $ex = Read-ExplainJson -Path $stderr.FullName
    $outLines = Get-Content -LiteralPath $stdout.FullName -ErrorAction SilentlyContinue
    $first = ($outLines | Select-Object -First 1)

    Add-Line ("--- {0} ---" -f $Name)
    Add-Line ("wall_ms: {0}" -f $r.WallMs)
    if ($ex) {
      $elapsedTotal = Get-PropValue -Obj $ex -Name 'elapsed_ms_total'
      if ($elapsedTotal -ne $null) { Add-Line ("ex_elapsed_ms_total: {0}" -f $elapsedTotal) }

      $t = Get-PropValue -Obj $ex -Name 'timings_ms'
      if ($t -ne $null) {
        foreach ($k in @('sql', 'match', 'unitize', 'symbol', 'file_read', 'walk', 'read_parse', 'write', 'write_one')) {
          $tv = Get-PropValue -Obj $t -Name $k
          if ($tv -ne $null) {
            Add-Line ("ex_elapsed_ms_{0}: {1}" -f $k, $tv)
          }
        }
      }
      foreach ($k in @('phase', 'q', 'unit', 'cache_hit', 'items_returned', 'rows_returned', 'symbol_fallback', 'unit_fallback', 'treesitter_disabled', 'treesitter_unsupported', 'treesitter_errors', 'files_total', 'files_indexed', 'chunks_written', 'symbols_written', 'comments_written')) {
        $v = Get-PropValue -Obj $ex -Name $k
        if ($v -ne $null) {
          Add-Line ("{0}: {1}" -f $k, $v)
        }
      }
    }
    if ($first) {
      Add-Line ("first: {0}" -f $first)
    }
    Add-Line ""
  } finally {
    Remove-Item -LiteralPath $stdout.FullName -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $stderr.FullName -Force -ErrorAction SilentlyContinue
  }
}

if ($Rebuild) {
  Write-Step 'Build 索引（index build）'
  Run-Case -Name 'index.build' -CmdArgs @(
    '--no-banner', '--database', $DbPath, '--explain=json',
    '--exclude', '*.ps1,result-*.txt',
    'index', 'build', $Root
  )
}

Write-Step 'Query 用例（方法 / 语句）'
$cases = @(
  [pscustomobject]@{ Name = 'q.method.symbol.ReplaceChunksBatch'; Query = 'ReplaceChunksBatch'; Unit = 'symbol'; Show = $false; Globs = @('*.go') },
  [pscustomobject]@{ Name = 'q.func.symbol.NewQueryCache'; Query = 'NewQueryCache'; Unit = 'symbol'; Show = $false; Globs = @('*.go') },
  [pscustomobject]@{ Name = 'q.statement.line.if-err'; Query = 'if err != nil'; Unit = 'line'; Show = $false; Globs = @('*.go') },
  [pscustomobject]@{ Name = 'q.statement.block.BEGIN-IMMEDIATE'; Query = 'BEGIN IMMEDIATE'; Unit = 'block'; Show = $false; Globs = @('*.go') },
  [pscustomobject]@{ Name = 'q.statement.block.CREATE-TABLE'; Query = 'CREATE TABLE'; Unit = 'block'; Show = $false; Globs = @('*.sql') },
  [pscustomobject]@{ Name = 'q.statement.line.SELECT'; Query = 'SELECT'; Unit = 'line'; Show = $false; Globs = @('*.go') },
  [pscustomobject]@{ Name = 'q.method.symbol.ReplaceChunksBatch.show'; Query = 'ReplaceChunksBatch'; Unit = 'symbol'; Show = $true; Globs = @('*.go') }
)

foreach ($c in $cases) {
  $cmdArgs = @('--no-banner', '--database', $DbPath, '--explain=json', 'q', $c.Query, '--unit', $c.Unit, '--limit', "$Limit", '--compact')
  if ($c.Globs -and $c.Globs.Count -gt 0) {
    foreach ($g in $c.Globs) {
      $cmdArgs += @('--glob', $g)
    }
  }
  if ($c.Show) {
    $cmdArgs += @('--show')
  }
  Run-Case -Name $c.Name -CmdArgs $cmdArgs
}

Set-Content -LiteralPath $resultPath -Value $lines -Encoding UTF8
Write-Host "`n已输出：$resultPath" -ForegroundColor Green
