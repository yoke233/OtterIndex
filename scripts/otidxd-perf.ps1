param(
    [string]$Root = ".",
    [string]$Listen = "",
    [string]$Query = "TODO",
    [switch]$Show
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

$rootAbs = (Resolve-Path -LiteralPath $Root).Path
$listenInfo = Get-Listen -Value $Listen
$listenHost = "127.0.0.1"
$listenPort = Get-Random -Minimum 20000 -Maximum 40000

if ($listenInfo -is [hashtable]) {
    if (-not [string]::IsNullOrWhiteSpace($listenInfo.Host)) {
        $listenHost = $listenInfo.Host
    }
    if ($listenInfo.Port) {
        $listenPort = [int]$listenInfo.Port
    }
} elseif ($listenInfo -is [pscustomobject]) {
    if (-not [string]::IsNullOrWhiteSpace($listenInfo.Host)) {
        $listenHost = $listenInfo.Host
    }
    if ($listenInfo.Port) {
        $listenPort = [int]$listenInfo.Port
    }
}

$listenAddr = "$listenHost`:$listenPort"

Write-Host "Root: $rootAbs"
Write-Host "Listen: $listenAddr"

$fileStats = Get-ChildItem -LiteralPath $rootAbs -Recurse -File -Force |
    Measure-Object -Property Length -Sum

Write-Host ("Files: {0}, Size: {1:n0} bytes" -f $fileStats.Count, $fileStats.Sum)

$serverProc = Start-Process -FilePath "go" -ArgumentList @("run", "./cmd/otidxd", "-listen", $listenAddr) -PassThru -WindowStyle Hidden
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

    $pingMs = (Measure-Command {
        $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "ping" -Params $null
    }).TotalMilliseconds
    $id++

    $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "workspace.add" -Params @{ root = $rootAbs }
    $wsid = $resp.result
    $id++

    $buildMs = (Measure-Command {
        $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "index.build" -Params @{ workspace_id = $wsid }
    }).TotalMilliseconds
    $id++

    $queryMs = (Measure-Command {
        $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "query" -Params @{
            workspace_id = $wsid
            q            = $Query
            unit         = "block"
            limit        = 20
            offset       = 0
            show         = [bool]$Show
        }
    }).TotalMilliseconds
    $id++

    $syncMs = (Measure-Command {
        $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "watch.start" -Params @{
            workspace_id  = $wsid
            sync_on_start = $true
        }
    }).TotalMilliseconds
    $id++

    $tmpFile = Join-Path $rootAbs "otidxd-perf-temp.txt"
    $needle = "PERF_TOKEN_$([Guid]::NewGuid().ToString('N'))"
    Set-Content -LiteralPath $tmpFile -Value ("hello`n" + $needle + "`n")

    $updateMs = (Measure-Command {
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
    }).TotalMilliseconds
    $id++

    Remove-Item -LiteralPath $tmpFile -Force

    Write-Host ""
    Write-Host "Timings (ms):"
    Write-Host ("  ping:        {0,8:n2}" -f $pingMs)
    Write-Host ("  index.build: {0,8:n2}" -f $buildMs)
    Write-Host ("  query:       {0,8:n2}" -f $queryMs)
    Write-Host ("  watch.start(sync_on_start): {0,8:n2}" -f $syncMs)
    Write-Host ("  watch.update+query:         {0,8:n2}" -f $updateMs)
}
finally {
    try { $client.Close() } catch {}
    try { $serverProc.Kill() } catch {}
}
