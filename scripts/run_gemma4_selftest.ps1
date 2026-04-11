param(
    [string]$BindHost = '127.0.0.1',
    [int]$Port = 18001,
    [int]$OllamaPort = 11435,
    [string]$Model = 'gemma4:e4b',
    [string]$OllamaExe = '',
    [string]$RunId = '',
    [string]$DatabasePath = '',
    [string[]]$Messages = @(),
    [switch]$ShowRaw,
    [switch]$SkipMetrics,
    [switch]$KeepRunning
)

$ErrorActionPreference = 'Stop'

$RepoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $RepoRoot

$PowerShellExe = (Get-Process -Id $PID).Path
$StartScript = Join-Path $RepoRoot 'start.ps1'
$TestScript = Join-Path $RepoRoot 'scripts\test_gemma4_planner.ps1'
$TmpDir = Join-Path $RepoRoot '.tmp'

New-Item -ItemType Directory -Force -Path $TmpDir | Out-Null

if ([string]::IsNullOrWhiteSpace($RunId)) {
    $RunId = 'gemma4_selftest_' + (Get-Date -Format 'yyyyMMdd_HHmmss') + '_' + ([guid]::NewGuid().ToString('N').Substring(0, 6))
}

$RunDir = Join-Path $TmpDir $RunId
New-Item -ItemType Directory -Force -Path $RunDir | Out-Null

if ([string]::IsNullOrWhiteSpace($DatabasePath)) {
    $DatabasePath = Join-Path $RunDir 'app.sqlite3'
}

$OllamaOutLog = Join-Path $RunDir 'ollama.out.log'
$OllamaErrLog = Join-Path $RunDir 'ollama.err.log'
$OllamaPidFile = Join-Path $RunDir 'ollama.pid'
$ServerOutLog = Join-Path $RunDir 'server.out.log'
$ServerErrLog = Join-Path $RunDir 'server.err.log'
$ServerPidFile = Join-Path $RunDir 'server.pid'

function Write-Section {
    param([string]$Title)
    Write-Host "==> $Title" -ForegroundColor Cyan
}

function Resolve-OllamaExePath {
    param([string]$PreferredPath)

    if (-not [string]::IsNullOrWhiteSpace($PreferredPath)) {
        if (-not (Test-Path -LiteralPath $PreferredPath)) {
            throw "Ollama executable not found: $PreferredPath"
        }
        return (Resolve-Path -LiteralPath $PreferredPath).Path
    }

    foreach ($candidate in @('ollama.exe', 'ollama')) {
        $command = Get-Command $candidate -ErrorAction SilentlyContinue
        if ($command -and $command.Source) {
            return $command.Source
        }
    }

    $defaultPath = Join-Path $env:LOCALAPPDATA 'Programs\Ollama\ollama.exe'
    if (Test-Path -LiteralPath $defaultPath) {
        return $defaultPath
    }

    throw 'Unable to locate ollama.exe. Pass -OllamaExe explicitly.'
}

function Remove-IfExists {
    param([string]$Path)
    if (Test-Path -LiteralPath $Path) {
        Remove-Item -LiteralPath $Path -Force
    }
}

function Try-UnloadModel {
    param(
        [string]$BaseUrl,
        [string]$TargetModel
    )

    try {
        Invoke-RestMethod -Method Post -Uri ($BaseUrl.TrimEnd('/') + '/api/generate') -ContentType 'application/json' -Body (@{
            model      = $TargetModel
            keep_alive = 0
        } | ConvertTo-Json -Depth 4) | Out-Null
    } catch {
    }
}

function Wait-ForHttp {
    param(
        [string]$Uri,
        [int]$TimeoutSeconds = 120,
        [int]$PollMilliseconds = 1000
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            Invoke-WebRequest -Method Get -Uri $Uri -UseBasicParsing | Out-Null
            return
        } catch {
            Start-Sleep -Milliseconds $PollMilliseconds
        }
    }

    throw "Timed out waiting for $Uri"
}

function Show-LogTail {
    param(
        [string]$Label,
        [string]$Path,
        [int]$Tail = 80
    )

    if (Test-Path -LiteralPath $Path) {
        Write-Host "--- $Label ---" -ForegroundColor DarkGray
        Get-Content -LiteralPath $Path -Tail $Tail
    }
}

function Stop-ManagedProcess {
    param($Process)

    if ($null -eq $Process) {
        return
    }

    try {
        if (-not $Process.HasExited) {
            Stop-Process -Id $Process.Id -Force -ErrorAction SilentlyContinue
        }
    } catch {
    }
}

$ResolvedOllamaExe = Resolve-OllamaExePath -PreferredPath $OllamaExe
$BaseUrl = "http://$BindHost`:$Port"
$OllamaBaseUrl = "http://127.0.0.1:$OllamaPort"
$ServerProcess = $null
$OllamaProcess = $null

Remove-IfExists -Path $DatabasePath
foreach ($path in @($OllamaOutLog, $OllamaErrLog, $ServerOutLog, $ServerErrLog, $OllamaPidFile, $ServerPidFile)) {
    Remove-IfExists -Path $path
}

try {
    Write-Section "Run directory: $RunDir"
    Write-Section 'Unloading gemma4 from default Ollama if it is still resident'
    Try-UnloadModel -BaseUrl 'http://127.0.0.1:11434' -TargetModel $Model

    Write-Section "Starting dedicated Ollama on $OllamaBaseUrl"
    $ollamaCommand = @"
`$env:OLLAMA_HOST='127.0.0.1:$OllamaPort'
`$env:OLLAMA_CONTEXT_LENGTH='512'
`$env:OLLAMA_KEEP_ALIVE='0s'
`$env:OLLAMA_MAX_LOADED_MODELS='1'
`$env:OLLAMA_NUM_PARALLEL='1'
& '$ResolvedOllamaExe' serve
"@
    $OllamaProcess = Start-Process -FilePath $PowerShellExe -ArgumentList @('-NoProfile', '-Command', $ollamaCommand) -WorkingDirectory $RepoRoot -RedirectStandardOutput $OllamaOutLog -RedirectStandardError $OllamaErrLog -PassThru
    Set-Content -LiteralPath $OllamaPidFile -Value $OllamaProcess.Id
    Wait-ForHttp -Uri ($OllamaBaseUrl + '/api/tags') -TimeoutSeconds 30

    Write-Section "Starting project server on $BaseUrl"
    $serverCommand = @"
Set-Location '$RepoRoot'
`$env:LLM_PROVIDER='ollama'
`$env:LLM_BASE_URL='http://127.0.0.1:$OllamaPort/v1'
`$env:LLM_API_KEY='ollama-local'
`$env:LLM_MODEL='$Model'
`$env:LLM_AUTH_FILE=' '
`$env:AGENT_RUNTIME_MODE='llm'
`$env:DATABASE_URL='$DatabasePath'
`$env:REDIS_URL='memory'
`$env:OTEL_ENABLED='false'
& '$StartScript' -SkipDocker -SkipLLMCheck -Port $Port
"@
    $ServerProcess = Start-Process -FilePath $PowerShellExe -ArgumentList @('-NoProfile', '-Command', $serverCommand) -WorkingDirectory $RepoRoot -RedirectStandardOutput $ServerOutLog -RedirectStandardError $ServerErrLog -PassThru
    Set-Content -LiteralPath $ServerPidFile -Value $ServerProcess.Id
    Wait-ForHttp -Uri ($BaseUrl + '/healthz') -TimeoutSeconds 180

    Write-Section 'Running planner self-test'
    $testArgs = @{
        BaseUrl = $BaseUrl
    }
    if ($ShowRaw) {
        $testArgs.ShowRaw = $true
    }
    if ($SkipMetrics) {
        $testArgs.SkipMetrics = $true
    }
    if ($Messages.Count -gt 0) {
        $testArgs.Messages = $Messages
    }
    & $TestScript @testArgs

    if ($KeepRunning) {
        Write-Section 'KeepRunning enabled'
        Write-Host "Run dir    : $RunDir"
        Write-Host "Server PID : $($ServerProcess.Id)"
        Write-Host "Ollama PID : $($OllamaProcess.Id)"
        Write-Host "Server logs : $ServerOutLog / $ServerErrLog"
        Write-Host "Ollama logs : $OllamaOutLog / $OllamaErrLog"
        Write-Host "Base URL    : $BaseUrl"
    }
} catch {
    Write-Host $_.Exception.Message -ForegroundColor Red
    Show-LogTail -Label 'Ollama stderr' -Path $OllamaErrLog
    Show-LogTail -Label 'Server stderr' -Path $ServerErrLog
    throw
} finally {
    if (-not $KeepRunning) {
        Write-Section 'Stopping temporary processes'
        Stop-ManagedProcess -Process $ServerProcess
        Stop-ManagedProcess -Process $OllamaProcess
    }
}
