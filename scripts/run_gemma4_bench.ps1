param(
    [string]$BindHost = '127.0.0.1',
    [int]$Port = 18001,
    [int]$OllamaPort = 11435,
    [string]$Model = 'gemma4:e4b',
    [string]$OllamaExe = '',
    [string]$RunId = '',
    [string]$DatabasePath = '',
    [string]$ReportPath = '',
    [int]$Rps = 2,
    [int]$Duration = 30,
    [int]$Concurrency = 1,
    [double]$TimeoutSeconds = 60,
    [string]$OllamaKeepAlive = '5m',
    [int]$WarmupPasses = 1,
    [string[]]$Messages = @(),
    [switch]$KeepRunning,
    [switch]$SkipMetrics,
    [switch]$SkipWarmup
)

$ErrorActionPreference = 'Stop'

$RepoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $RepoRoot

$PowerShellExe = (Get-Process -Id $PID).Path
$StartScript = Join-Path $RepoRoot 'start.ps1'
$LoadTestScript = Join-Path $RepoRoot 'scripts\run_load_test.py'
$TmpDir = Join-Path $RepoRoot '.tmp'

New-Item -ItemType Directory -Force -Path $TmpDir | Out-Null

if ([string]::IsNullOrWhiteSpace($RunId)) {
    $RunId = 'gemma4_bench_' + (Get-Date -Format 'yyyyMMdd_HHmmss') + '_' + ([guid]::NewGuid().ToString('N').Substring(0, 6))
}

$RunDir = Join-Path $TmpDir $RunId
New-Item -ItemType Directory -Force -Path $RunDir | Out-Null

if ([string]::IsNullOrWhiteSpace($DatabasePath)) {
    $DatabasePath = Join-Path $RunDir 'app.sqlite3'
}

if ([string]::IsNullOrWhiteSpace($ReportPath)) {
    $ReportPath = Join-Path $RunDir 'benchmark_report.json'
}

$MessageFile = Join-Path $RunDir 'messages.txt'
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

function Resolve-PythonExePath {
    $venvPython = Join-Path $RepoRoot '.venv\Scripts\python.exe'
    if (Test-Path -LiteralPath $venvPython) {
        return $venvPython
    }

    foreach ($candidate in @('python.exe', 'python')) {
        $command = Get-Command $candidate -ErrorAction SilentlyContinue
        if ($command -and $command.Source) {
            return $command.Source
        }
    }

    throw 'Unable to locate python executable.'
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

function Invoke-JsonPost {
    param(
        [string]$Uri,
        [hashtable]$Payload
    )

    return Invoke-RestMethod -Method Post -Uri $Uri -ContentType 'application/json; charset=utf-8' -Body ($Payload | ConvertTo-Json -Depth 8)
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
        [int]$Tail = 120
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

if ($Messages.Count -eq 0) {
    $Messages = @(
        [regex]::Unescape('\u5317\u4eac\u533a\u6628\u5929\u8d85SLA\u7684\u5de5\u5355\u6309\u539f\u56e0\u5206\u7c7b'),
        [regex]::Unescape('\u6700\u8fd17\u5929\u5317\u4eac\u533a\u9000\u6b3e\u7387\u6700\u9ad8\u7684\u7c7b\u76ee\u662f\u4ec0\u4e48\uff1f'),
        [regex]::Unescape('\u5317\u4eac\u533a\u6628\u5929\u9000\u6b3e\u7387\u5f02\u5e38\u548c\u8d85SLA\u5de5\u5355\u505a\u4e00\u4e0b\u5f52\u56e0\u5206\u6790')
    )
}

$ResolvedOllamaExe = Resolve-OllamaExePath -PreferredPath $OllamaExe
$ResolvedPythonExe = Resolve-PythonExePath
$BaseUrl = "http://$BindHost`:$Port"
$OllamaBaseUrl = "http://127.0.0.1:$OllamaPort"
$OllamaProcess = $null
$ServerProcess = $null

foreach ($path in @($DatabasePath, $ReportPath, $MessageFile, $OllamaOutLog, $OllamaErrLog, $ServerOutLog, $ServerErrLog, $OllamaPidFile, $ServerPidFile)) {
    Remove-IfExists -Path $path
}

Set-Content -LiteralPath $MessageFile -Value $Messages -Encoding UTF8

try {
    Write-Section "Run directory: $RunDir"
    Write-Section 'Unloading gemma4 from default Ollama if it is still resident'
    Try-UnloadModel -BaseUrl 'http://127.0.0.1:11434' -TargetModel $Model

    Write-Section "Starting dedicated Ollama on $OllamaBaseUrl"
    $ollamaCommand = @"
`$env:OLLAMA_HOST='127.0.0.1:$OllamaPort'
`$env:OLLAMA_CONTEXT_LENGTH='512'
`$env:OLLAMA_KEEP_ALIVE='$OllamaKeepAlive'
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

    $warmupResults = @()
    if (-not $SkipWarmup -and $WarmupPasses -gt 0) {
        Write-Section 'Running warmup requests'
        for ($pass = 1; $pass -le $WarmupPasses; $pass++) {
            for ($index = 0; $index -lt $Messages.Count; $index++) {
                $message = $Messages[$index]
                $response = Invoke-JsonPost -Uri ($BaseUrl + '/api/v1/chat') -Payload @{
                    session_id = "warmup_${pass}_$index"
                    user_id    = 1
                    message    = $message
                }
                $warmupResults += [pscustomobject]@{
                    pass               = $pass
                    index              = $index + 1
                    status             = $response.status
                    planning_source    = $response.planning_source
                    planner_latency_ms = $response.planner_latency_ms
                }
            }
        }
    }

    Write-Section 'Running load test'
    $benchJson = & $ResolvedPythonExe $LoadTestScript `
        --base-url $BaseUrl `
        --rps $Rps `
        --duration $Duration `
        --concurrency $Concurrency `
        --timeout $TimeoutSeconds `
        --message-file $MessageFile

    $benchReport = $benchJson | ConvertFrom-Json
    $report = [pscustomobject]@{
        generated_at        = (Get-Date).ToString('s')
        base_url            = $BaseUrl
        ollama_base_url     = $OllamaBaseUrl
        model               = $Model
        database_path       = $DatabasePath
        rps                 = $Rps
        duration_seconds    = $Duration
        concurrency         = $Concurrency
        timeout_seconds     = $TimeoutSeconds
        ollama_keep_alive   = $OllamaKeepAlive
        warmup_passes       = $WarmupPasses
        benchmark_mode      = if ($SkipWarmup) { 'cold_or_mixed' } else { 'steady_state_after_warmup' }
        messages            = $Messages
        warmup_results      = $warmupResults
        benchmark           = $benchReport
        metrics_snapshot    = $null
    }

    if (-not $SkipMetrics) {
        Write-Section 'Collecting metrics snapshot'
        $metricsContent = (Invoke-WebRequest -Uri ($BaseUrl + '/metrics') -UseBasicParsing).Content
        $metricLines = ($metricsContent -split "`n") | Where-Object {
            $_ -match 'ops_agent_chat_requests_total' -or
            $_ -match 'ops_agent_planner_requests_total' -or
            $_ -match 'ops_agent_llm_requests_total' -or
            $_ -match 'ops_agent_llm_fallback_total'
        }
        $report.metrics_snapshot = $metricLines
    }

    $report | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $ReportPath -Encoding UTF8

    Write-Section 'Benchmark result'
    $benchReport | ConvertTo-Json -Depth 10

    if (-not $SkipMetrics -and $report.metrics_snapshot) {
        Write-Section 'Metrics snapshot'
        $report.metrics_snapshot
    }

    Write-Section 'Saved report'
    Write-Host $ReportPath

    if ($KeepRunning) {
        Write-Section 'KeepRunning enabled'
        Write-Host "Run dir    : $RunDir"
        Write-Host "Server PID : $($ServerProcess.Id)"
        Write-Host "Ollama PID : $($OllamaProcess.Id)"
        Write-Host "Server logs : $ServerOutLog / $ServerErrLog"
        Write-Host "Ollama logs : $OllamaOutLog / $OllamaErrLog"
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
