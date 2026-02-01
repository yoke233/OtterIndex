param(
    [string]$Listen = "127.0.0.1:7337",
    [string]$Root = ".",
    [string]$Query = "hello",
    [string]$Store = "sqlite",
    [switch]$Show
)

$ErrorActionPreference = "Stop"

function Parse-Listen {
    param([string]$Value)
    $parts = $Value.Split(":", 2)
    if ($parts.Count -ne 2) {
        throw "invalid -Listen value: $Value"
    }
    [pscustomobject]@{
        Host = $parts[0]
        Port = [int]$parts[1]
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

$listen = Parse-Listen -Value $Listen
$rootAbs = (Resolve-Path -LiteralPath $Root).Path

$client = [System.Net.Sockets.TcpClient]::new()
$client.Connect($listen.Host, $listen.Port)

try {
    $stream = $client.GetStream()
    $writer = [System.IO.StreamWriter]::new($stream)
    $writer.NewLine = "`n"
    $writer.AutoFlush = $true
    $reader = [System.IO.StreamReader]::new($stream)

    $id = 1

    Write-Host "ping ->"
    $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "ping" -Params $null
    Write-Host ($resp | ConvertTo-Json -Depth 6)
    $id++

    Write-Host "version ->"
    $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "version" -Params $null
    Write-Host ($resp | ConvertTo-Json -Depth 6)
    $id++

    Write-Host "workspace.add ->"
    $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "workspace.add" -Params @{ root = $rootAbs; store = $Store }
    Write-Host ($resp | ConvertTo-Json -Depth 6)
    $wsid = $resp.result
    $id++

    Write-Host "index.build ->"
    $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "index.build" -Params @{ workspace_id = $wsid }
    Write-Host ($resp | ConvertTo-Json -Depth 6)
    $id++

    Write-Host "query ->"
    $params = @{
        workspace_id = $wsid
        q            = $Query
        unit         = "block"
        limit        = 10
        offset       = 0
        show         = [bool]$Show
    }
    $resp = Send-Rpc -Writer $writer -Reader $reader -Id $id -Method "query" -Params $params
    Write-Host ($resp | ConvertTo-Json -Depth 6)
}
finally {
    $client.Close()
}
