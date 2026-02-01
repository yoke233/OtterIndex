param(
    [string]$Root = ".",
    [string]$Listen = "",
    [string]$Query = "TODO",
    [switch]$Show,
    [int]$SampleMs = 200,
    [int]$DebounceMs = 0,
    [switch]$AdaptiveDebounce,
    [int]$DebounceMinMs = 0,
    [int]$DebounceMaxMs = 0,
    [int]$SyncWorkers = 0,
    [string]$QueueMode = "",
    [switch]$NoAutoTune
)

$ErrorActionPreference = "Stop"

function Get-Listen {
    param([string]$Value)
    if (-not [string]::IsNullOrWhiteSpace($Value)) {
        $parts = $Value.Split(":", 2)
        if ($parts.Count -ne 2) {
            throw "invalid -Listen value: $Value"
        }
        return [pscustomobject]@{
            Host = $parts[0]
            Port = [int]$parts[1]
        }
    }

    $port = Get-Random -Minimum 20000 -Maximum 40000
    return [pscustomobject]@{
        Host = "127.0.0.1"
        Port = $port
    }
}

function Send-Rpc {
    param(
        [System.IO.StreamWriter]$Writer,
        [System.IO.StreamReader]$Reader,
        [int]$Id,
        [string]$Method,
        [hashtable]$Params
    )
    $req = @{
        jsonrpc = "2.0"
        id      = $Id
        method  = $Method
    }
    if ($null -ne $Params) {
        $req.params = $Params
    }
    $line = $req | ConvertTo-Json -Depth 6 -Compress
    $Writer.WriteLine($line)
    $respLine = $Reader.ReadLine()
    if ([string]::IsNullOrWhiteSpace($respLine)) {
        throw "empty response for $Method"
    }
    return $respLine | ConvertFrom-Json -Depth 6
}

function Wait-Server {
    param([string]$Listen)
    $deadline = (Get-Date).AddSeconds(5)
    while ((Get-Date) -lt $deadline) {
        try {
            $client = [System.Net.Sockets.TcpClient]::new()
            $parts = $Listen.Split(":", 2)
            $client.Connect($parts[0], [int]$parts[1])
            $client.Close()
            return $true
        } catch {
            Start-Sleep -Milliseconds 100
        }
    }
    return $false
}

function Start-Sampler {
    param(
        [int]$TargetPid,
        [int]$SampleMs,
        [string]$OutPath,
        [string]$StopPath
    )
    return Start-Job -ArgumentList $TargetPid, $SampleMs, $OutPath, $StopPath -ScriptBlock {
        param($JobPid, $JobSampleMs, $JobOutPath, $JobStopPath)
        $maxWS = 0
        $maxPrivate = 0
        $maxCPU = 0.0
        while (-not (Test-Path -LiteralPath $JobStopPath)) {
            try {
                $p = Get-Process -Id $JobPid -ErrorAction Stop
                if ($p.WorkingSet64 -gt $maxWS) { $maxWS = $p.WorkingSet64 }
                if ($p.PrivateMemorySize64 -gt $maxPrivate) { $maxPrivate = $p.PrivateMemorySize64 }
                if ($p.CPU -gt $maxCPU) { $maxCPU = $p.CPU }
            } catch {
                break
            }
            Start-Sleep -Milliseconds $JobSampleMs
        }
        [pscustomobject]@{
            MaxWorkingSet64 = $maxWS
            MaxPrivateBytes = $maxPrivate
            MaxCPUSeconds   = $maxCPU
        } | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $JobOutPath
    }
}

function Measure-Phase {
    param(
        [string]$Name,
        [int]$TargetPid,
        [int]$SampleMs,
        [scriptblock]$Action
    )
    $stopFile = [System.IO.Path]::GetTempFileName()
    $outFile = [System.IO.Path]::GetTempFileName()
    Remove-Item -LiteralPath $stopFile -Force

    $job = Start-Sampler -TargetPid $TargetPid -SampleMs $SampleMs -OutPath $outFile -StopPath $stopFile
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    & $Action
    $sw.Stop()

    New-Item -ItemType File -Path $stopFile | Out-Null
    Wait-Job -Job $job | Out-Null
    Receive-Job -Job $job | Out-Null
    Remove-Job -Job $job | Out-Null

    $stats = Get-Content -LiteralPath $outFile | ConvertFrom-Json
    if ($stats.MaxWorkingSet64 -eq 0 -and $stats.MaxPrivateBytes -eq 0) {
        try {
            $p = Get-Process -Id $TargetPid -ErrorAction Stop
            $stats.MaxWorkingSet64 = $p.WorkingSet64
            $stats.MaxPrivateBytes = $p.PrivateMemorySize64
            if ($p.CPU -gt $stats.MaxCPUSeconds) {
                $stats.MaxCPUSeconds = $p.CPU
            }
        } catch {
        }
    }
    Remove-Item -LiteralPath $stopFile -Force
    Remove-Item -LiteralPath $outFile -Force

    [pscustomobject]@{
        Name            = $Name
        DurationMs      = [math]::Round($sw.Elapsed.TotalMilliseconds, 2)
        MaxWorkingSetMB = [math]::Round($stats.MaxWorkingSet64 / 1MB, 2)
        MaxPrivateMB    = [math]::Round($stats.MaxPrivateBytes / 1MB, 2)
        MaxCPUSeconds   = [math]::Round($stats.MaxCPUSeconds, 2)
    }
}

$rootAbs = (Resolve-Path -LiteralPath $Root).Path
$listenInfo = Get-Listen -Value $Listen
$listenHost = $listenInfo.Host
$listenPort = $listenInfo.Port
$listenAddr = "$listenHost`:$listenPort"

Write-Host "Root: $rootAbs"
Write-Host "Listen: $listenAddr"

$fileStats = Get-ChildItem -LiteralPath $rootAbs -Recurse -File -Force |
    Measure-Object -Property Length -Sum

Write-Host ("Files: {0}, Size: {1:n0} bytes" -f $fileStats.Count, $fileStats.Sum)

$exeRoot = Join-Path $rootAbs ".otidx\perf"
New-Item -ItemType Directory -Path $exeRoot -Force | Out-Null
$exePath = Join-Path $exeRoot "otidxd-perf.exe"

Write-Host "Building otidxd..."
& go build -o $exePath ./cmd/otidxd

$serverProc = Start-Process -FilePath $exePath -ArgumentList @("-listen", $listenAddr) -PassThru -WindowStyle Hidden
if (-not (Wait-Server -Listen $listenAddr)) {
    try { $serverProc.Kill() } catch {}
    throw "failed to start otidxd"
}

try {
    $client = [System.Net.Sockets.TcpClient]::new()
    $client.Connect($listenHost, $listenPort)
    $stream = $client.GetStream()
    $writer = [System.IO.StreamWriter]::new($stream)
    $writer.NewLine = "`n"
    $writer.AutoFlush = $true
    $reader = [System.IO.StreamReader]::new($stream)

    $id = 1

    $ping = Measure-Phase -Name "ping" -TargetPid $serverProc.Id -SampleMs $SampleMs -Action {
        $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "ping" -Params $null
    }
    $id++

    $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "workspace.add" -Params @{ root = $rootAbs }
    $wsid = $resp.result
    $id++

    $build = Measure-Phase -Name "index.build" -TargetPid $serverProc.Id -SampleMs $SampleMs -Action {
        $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "index.build" -Params @{ workspace_id = $wsid }
    }
    $id++

    $queryPhase = Measure-Phase -Name "query" -TargetPid $serverProc.Id -SampleMs $SampleMs -Action {
        $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "query" -Params @{
            workspace_id = $wsid
            q            = $Query
            unit         = "block"
            limit        = 20
            offset       = 0
            show         = [bool]$Show
        }
    }
    $id++

    $sync = Measure-Phase -Name "watch.start(sync_on_start)" -TargetPid $serverProc.Id -SampleMs $SampleMs -Action {
        $params = @{
            workspace_id  = $wsid
            sync_on_start = $true
        }
        if ($DebounceMs -gt 0) { $params.debounce_ms = $DebounceMs }
        if ($AdaptiveDebounce) { $params.adaptive_debounce = $true }
        if ($DebounceMinMs -gt 0) { $params.debounce_min_ms = $DebounceMinMs }
        if ($DebounceMaxMs -gt 0) { $params.debounce_max_ms = $DebounceMaxMs }
        if ($SyncWorkers -gt 0) { $params.sync_workers = $SyncWorkers }
        if (-not [string]::IsNullOrWhiteSpace($QueueMode)) { $params.queue_mode = $QueueMode }
        if ($NoAutoTune) { $params.auto_tune = $false }
        $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "watch.start" -Params $params
    }
    $id++

    $tmpFile = Join-Path $rootAbs "otidxd-perf-temp.txt"
    $needle = "PERF_TOKEN_$([Guid]::NewGuid().ToString('N'))"
    Set-Content -LiteralPath $tmpFile -Value ("hello`n" + $needle + "`n")

    $update = Measure-Phase -Name "watch.update+query" -TargetPid $serverProc.Id -SampleMs $SampleMs -Action {
        $deadline = (Get-Date).AddSeconds(3)
        while ((Get-Date) -lt $deadline) {
            $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "query" -Params @{
                workspace_id = $wsid
                q            = $needle
                unit         = "block"
                limit        = 10
                offset       = 0
            }
            if ($resp.result.Count -gt 0) {
                break
            }
            Start-Sleep -Milliseconds 50
        }
    }
    $id++

    Remove-Item -LiteralPath $tmpFile -Force

    $results = @($ping, $build, $queryPhase, $sync, $update)
    Write-Host ""
    Write-Host "Timings + Peak Usage:"
    foreach ($r in $results) {
        Write-Host ("  {0,-26} {1,8:n2} ms | WS {2,8:n2} MB | Private {3,8:n2} MB | CPU {4,6:n2}s" -f $r.Name, $r.DurationMs, $r.MaxWorkingSetMB, $r.MaxPrivateMB, $r.MaxCPUSeconds)
    }
}
finally {
    try { $client.Close() } catch {}
    try { $serverProc.Kill() } catch {}
}
