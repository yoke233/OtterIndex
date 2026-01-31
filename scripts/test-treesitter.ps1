[CmdletBinding()]
param(
  [switch]$NoTests,
  [switch]$Build,
  [switch]$Smoke,
  [switch]$Explain,
  [string]$Query = 'func',
  [int]$Limit = 5,
  [string]$DbPath = '.otidx/index.db',
  [string]$OutPath = '.otidx/bin/otidx-ts.exe'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Write-Step {
  param([Parameter(Mandatory)][string]$Title)
  Write-Host "`n== $Title ==" -ForegroundColor Cyan
}

function Invoke-External {
  param(
    [Parameter(Mandatory)][string]$FilePath,
    [Parameter()][string[]]$Args = @()
  )
  & $FilePath @Args
  if ($LASTEXITCODE -ne 0) {
    throw "$FilePath 失败（exit=$LASTEXITCODE）"
  }
}

function Ensure-Gcc {
  if (Get-Command gcc -ErrorAction SilentlyContinue) { return $true }

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

  return [bool](Get-Command gcc -ErrorAction SilentlyContinue)
}

function Build-OtidxTreesitter {
  param([Parameter(Mandatory)][string]$OutPath)

  $outDir = Split-Path -Parent $OutPath
  if ($outDir) {
    New-Item -ItemType Directory -Path $outDir -Force | Out-Null
  }

  Write-Host "build -> $OutPath"
  Invoke-External -FilePath 'go' -Args @('build', '-tags', 'treesitter', '-o', $OutPath, './cmd/otidx')

  return (Resolve-Path -LiteralPath $OutPath).Path
}

Set-Location -LiteralPath $PSScriptRoot

Write-Step 'Go 环境'
Invoke-External -FilePath 'go' -Args @('version')
Invoke-External -FilePath 'go' -Args @('env', 'GOOS', 'GOARCH', 'GOMOD', 'CGO_ENABLED')

if (-not $NoTests) {
  Write-Step '1) 纯 Go 测试（无 treesitter tag）'
  Invoke-External -FilePath 'go' -Args @('test', './...')
}

Write-Step '2) 准备 treesitter + cgo（需要 gcc）'
$env:CGO_ENABLED = '1'
if (-not (Ensure-Gcc)) {
  Write-Host '未找到 gcc（MinGW）。' -ForegroundColor Red
  Write-Host '建议安装：scoop install mingw' -ForegroundColor Yellow
  Write-Host '安装后重开终端或重新运行此脚本。' -ForegroundColor Yellow
  exit 1
}

$env:CC = (Get-Command gcc).Source
$env:CXX = (Get-Command g++).Source
Write-Host "CGO_ENABLED=$env:CGO_ENABLED"
Write-Host "CC=$env:CC"
Write-Host "CXX=$env:CXX"

if (-not $NoTests) {
  Write-Step '2.1) treesitter + cgo 测试（-tags treesitter）'
  Invoke-External -FilePath 'go' -Args @('test', '-tags', 'treesitter', './...')
}

if ($Build -or $Smoke) {
  Write-Step '2.2) 编译 otidx（treesitter）'
  $otidxExe = Build-OtidxTreesitter -OutPath $OutPath
  Write-Host "otidx: $otidxExe" -ForegroundColor Green
}

if ($Smoke) {
  Write-Step '3) smoke：索引 + --unit symbol 查询（使用已编译二进制）'

  $commonArgs = @('--database', $DbPath)
  if ($Explain) {
    $commonArgs += @('--explain')
  }

  $buildArgs = $commonArgs + @('index', 'build', $PSScriptRoot)
  Invoke-External -FilePath $otidxExe -Args $buildArgs

  $queryArgs = $commonArgs + @('q', $Query, '--unit', 'symbol', '--limit', "$Limit", '--compact')
  Invoke-External -FilePath $otidxExe -Args $queryArgs
}

Write-Host "`nOK" -ForegroundColor Green
