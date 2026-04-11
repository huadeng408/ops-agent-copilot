param(
    [string]$BindHost = '127.0.0.1',
    [int]$Port = 18001,
    [int]$OllamaPort = 11435,
    [string]$PrimaryModel = 'qwen3:4b',
    [string]$FallbackModel = 'gemma4:e4b',
    [string]$BaselineModel = 'gemma4:e4b',
    [string]$OllamaExe = '',
    [string]$RunId = '',
    [string]$ReportPath = '',
    [string]$Stages = '100,200',
    [int]$Duration = 10,
    [double]$TimeoutSeconds = 30,
    [string]$OllamaKeepAlive = '5m',
    [int]$WarmupPasses = 1,
    [switch]$UseDocker,
    [switch]$KeepRunning,
    [switch]$SkipMetrics
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
    $RunId = 'mixed_router_compare_' + (Get-Date -Format 'yyyyMMdd_HHmmss') + '_' + ([guid]::NewGuid().ToString('N').Substring(0, 6))
}

$RunDir = Join-Path $TmpDir $RunId
New-Item -ItemType Directory -Force -Path $RunDir | Out-Null

if ([string]::IsNullOrWhiteSpace($ReportPath)) {
    $ReportPath = Join-Path $RunDir 'mixed_router_compare_report.json'
}

$OllamaOutLog = Join-Path $RunDir 'ollama.out.log'
$OllamaErrLog = Join-Path $RunDir 'ollama.err.log'
$OllamaPidFile = Join-Path $RunDir 'ollama.pid'

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
        Remove-Item -LiteralPath $Path -Force -Recurse
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

function Invoke-JsonPost {
    param(
        [string]$Uri,
        [hashtable]$Payload
    )

    return Invoke-RestMethod -Method Post -Uri $Uri -ContentType 'application/json; charset=utf-8' -Body ($Payload | ConvertTo-Json -Depth 8)
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

function Stop-ProcessesListeningOnPort {
    param([int]$LocalPort)

    $processIds = @()
    try {
        $processIds = Get-NetTCPConnection -LocalPort $LocalPort -State Listen -ErrorAction Stop |
            Select-Object -ExpandProperty OwningProcess -Unique
    } catch {
        return
    }

    foreach ($processId in $processIds) {
        try {
            Stop-Process -Id $processId -Force -ErrorAction SilentlyContinue
        } catch {
        }
    }
}

function Parse-StageValues {
    param([string]$StageText)

    $values = @()
    foreach ($part in ($StageText -split '[,\s]+' | Where-Object { $_ })) {
        $value = 0
        if (-not [int]::TryParse($part, [ref]$value)) {
            throw "Invalid stage RPS value: $part"
        }
        if ($value -le 0) {
            throw "Stage RPS must be > 0: $part"
        }
        $values += $value
    }
    if ($values.Count -eq 0) {
        throw 'At least one stage RPS is required.'
    }
    return $values
}

function New-MixedBenchmarkMessages {
    param([int]$Count)

    $messages = New-Object System.Collections.Generic.List[string]
    for ($index = 1; $index -le $Count; $index++) {
        $token = '{0:D5}' -f $index
        if (($index % 10) -eq 0) {
            $messages.Add("把北京昨天退款异常、超SLA工单和最近发布关联分析一下，请求编号 A$token")
            continue
        }

        switch (($index - 1) % 3) {
            0 { $messages.Add("最近7天退款率异常的类目有哪些？请求编号 R$token") }
            1 { $messages.Add("查一下 T202603280012 的详情，请求编号 T$token") }
            default { $messages.Add("生成一份今天的运营日报，请求编号 P$token") }
        }
    }
    return $messages
}

function Write-StageMessageFile {
    param(
        [string]$Path,
        [string[]]$Messages
    )

    Set-Content -LiteralPath $Path -Value $Messages -Encoding UTF8
}

function Get-MetricsSnapshot {
    param([string]$BaseUrl)

    $metricsContent = (Invoke-WebRequest -Uri ($BaseUrl + '/metrics') -UseBasicParsing).Content
    return ($metricsContent -split "`n") | Where-Object {
        $_ -match 'ops_agent_chat_requests_total' -or
        $_ -match 'ops_agent_planner_requests_total' -or
        $_ -match 'ops_agent_llm_requests_total' -or
        $_ -match 'ops_agent_llm_fallback_total' -or
        $_ -match 'ops_agent_planner_cache_total'
    }
}

function Start-BenchmarkServer {
    param(
        [pscustomobject]$Variant,
        [string]$DatabasePath,
        [string]$ServerOutLog,
        [string]$ServerErrLog,
        [string]$ServerPidFile,
        [string]$BaseUrl,
        [string]$OllamaBaseUrl
    )

    Stop-ProcessesListeningOnPort -LocalPort $Port
    Start-Sleep -Seconds 2

    if (Test-Path -LiteralPath $DatabasePath) {
        Remove-IfExists -Path $DatabasePath
    }

    $serverLines = @(
        "Set-Location '$RepoRoot'",
        "`$env:LLM_PROVIDER='ollama'",
        "`$env:LLM_BASE_URL='$OllamaBaseUrl/v1'",
        "`$env:LLM_API_KEY='ollama-local'",
        "`$env:LLM_MODEL='$($Variant.PrimaryModel)'",
        "`$env:LLM_AUTH_FILE=' '",
        "`$env:AGENT_RUNTIME_MODE='auto'",
        "`$env:ROUTER_PRIMARY_MODEL='$($Variant.PrimaryModel)'",
        "`$env:ROUTER_FALLBACK_MODEL='$($Variant.FallbackModel)'",
        "`$env:ROUTER_NO_THINK='true'",
        "`$env:ROUTER_RECENT_MESSAGE_COUNT='2'",
        "`$env:ROUTER_CONFIDENCE_CUTOFF='0.72'",
        "`$env:ROUTER_DISABLE_FAST_PATH='$($Variant.DisableFastPath.ToString().ToLower())'",
        "`$env:KEEP_RECENT_MESSAGE_COUNT='2'",
        "`$env:OTEL_ENABLED='false'"
    )

    if (-not $UseDocker) {
        $serverLines += "`$env:DATABASE_URL='$DatabasePath'"
        $serverLines += "`$env:REDIS_URL='memory'"
        $serverLines += "& '$StartScript' -SkipDocker -SkipLLMCheck -Port $Port"
    } else {
        $serverLines += "& '$StartScript' -SkipLLMCheck -Port $Port"
    }

    $serverCommand = ($serverLines -join "`n")
    $serverProcess = Start-Process -FilePath $PowerShellExe -ArgumentList @('-NoProfile', '-Command', $serverCommand) -WorkingDirectory $RepoRoot -RedirectStandardOutput $ServerOutLog -RedirectStandardError $ServerErrLog -PassThru
    Set-Content -LiteralPath $ServerPidFile -Value $serverProcess.Id
    Wait-ForHttp -Uri ($BaseUrl + '/healthz') -TimeoutSeconds 240
    return $serverProcess
}

function Invoke-WarmupPasses {
    param(
        [string]$BaseUrl,
        [string]$VariantName,
        [int]$PassCount
    )

    $results = @()
    if ($PassCount -le 0) {
        return $results
    }

    $warmupMessages = @(
        '最近7天退款率异常的类目有哪些？warmup_primary_refund',
        '查一下 T202603280012 的详情，warmup_primary_ticket',
        '生成一份今天的运营日报，warmup_primary_report',
        '把北京昨天退款异常、超SLA工单和最近发布关联分析一下，warmup_fallback_anomaly'
    )

    for ($pass = 1; $pass -le $PassCount; $pass++) {
        for ($index = 0; $index -lt $warmupMessages.Count; $index++) {
            $response = Invoke-JsonPost -Uri ($BaseUrl + '/api/v1/chat') -Payload @{
                session_id   = "warmup_${VariantName}_${pass}_$index"
                user_id      = 1
                message      = $warmupMessages[$index]
                runtime_mode = 'auto'
            }
            $results += [pscustomobject]@{
                pass               = $pass
                index              = $index + 1
                status             = $response.status
                planning_source    = $response.planning_source
                planner_latency_ms = $response.planner_latency_ms
            }
        }
    }

    return $results
}

function Percent-Change {
    param(
        [double]$Baseline,
        [double]$Current
    )

    if ($Baseline -eq 0) {
        return $null
    }
    return [math]::Round((($Baseline - $Current) / $Baseline) * 100, 2)
}

$ResolvedOllamaExe = Resolve-OllamaExePath -PreferredPath $OllamaExe
$ResolvedPythonExe = Resolve-PythonExePath
$BaseUrl = "http://$BindHost`:$Port"
$OllamaBaseUrl = "http://127.0.0.1:$OllamaPort"
$OllamaProcess = $null
$ServerProcess = $null
$StageValues = Parse-StageValues -StageText $Stages

$VariantSpecs = @(
    [pscustomobject]@{
        Name          = 'baseline_gemma_only'
        Label         = 'Baseline (Gemma4 only, no L0 fast path)'
        PrimaryModel  = $BaselineModel
        FallbackModel = $BaselineModel
        DisableFastPath = $true
    },
    [pscustomobject]@{
        Name          = 'optimized_qwen_plus_gemma'
        Label         = 'Optimized (L0 fast path + Qwen3 primary + Gemma4 fallback)'
        PrimaryModel  = $PrimaryModel
        FallbackModel = $FallbackModel
        DisableFastPath = $false
    }
)

foreach ($path in @($ReportPath, $OllamaOutLog, $OllamaErrLog, $OllamaPidFile)) {
    Remove-IfExists -Path $path
}

try {
    Write-Section "Run directory: $RunDir"
    Write-Section 'Cleaning dedicated benchmark ports'
    Stop-ProcessesListeningOnPort -LocalPort $Port
    Stop-ProcessesListeningOnPort -LocalPort $OllamaPort

    Write-Section 'Unloading benchmark models from default Ollama'
    foreach ($model in @($BaselineModel, $PrimaryModel, $FallbackModel) | Select-Object -Unique) {
        Try-UnloadModel -BaseUrl 'http://127.0.0.1:11434' -TargetModel $model
    }

    Write-Section "Starting dedicated Ollama on $OllamaBaseUrl"
    $ollamaCommand = @"
`$env:OLLAMA_HOST='127.0.0.1:$OllamaPort'
`$env:OLLAMA_CONTEXT_LENGTH='1024'
`$env:OLLAMA_KEEP_ALIVE='$OllamaKeepAlive'
`$env:OLLAMA_MAX_LOADED_MODELS='1'
`$env:OLLAMA_NUM_PARALLEL='1'
`$env:OLLAMA_FLASH_ATTENTION='1'
`$env:OLLAMA_KV_CACHE_TYPE='q8_0'
& '$ResolvedOllamaExe' serve
"@
    $OllamaProcess = Start-Process -FilePath $PowerShellExe -ArgumentList @('-NoProfile', '-Command', $ollamaCommand) -WorkingDirectory $RepoRoot -RedirectStandardOutput $OllamaOutLog -RedirectStandardError $OllamaErrLog -PassThru
    Set-Content -LiteralPath $OllamaPidFile -Value $OllamaProcess.Id
    Wait-ForHttp -Uri ($OllamaBaseUrl + '/api/tags') -TimeoutSeconds 30

    $variantReports = @()
    foreach ($variant in $VariantSpecs) {
        Write-Section "Benchmarking $($variant.Label)"

        $VariantDir = Join-Path $RunDir $variant.Name
        New-Item -ItemType Directory -Force -Path $VariantDir | Out-Null

        $DatabasePath = Join-Path $VariantDir 'app.sqlite3'
        $ServerOutLog = Join-Path $VariantDir 'server.out.log'
        $ServerErrLog = Join-Path $VariantDir 'server.err.log'
        $ServerPidFile = Join-Path $VariantDir 'server.pid'

        foreach ($path in @($DatabasePath, $ServerOutLog, $ServerErrLog, $ServerPidFile)) {
            Remove-IfExists -Path $path
        }

        $ServerProcess = Start-BenchmarkServer -Variant $variant -DatabasePath $DatabasePath -ServerOutLog $ServerOutLog -ServerErrLog $ServerErrLog -ServerPidFile $ServerPidFile -BaseUrl $BaseUrl -OllamaBaseUrl $OllamaBaseUrl

        try {
            Write-Section "Warmup $($variant.Name)"
            $warmupResults = Invoke-WarmupPasses -BaseUrl $BaseUrl -VariantName $variant.Name -PassCount $WarmupPasses

            $totalStageRequests = ($StageValues | Measure-Object -Sum).Sum * $Duration
            $allMessages = New-MixedBenchmarkMessages -Count $totalStageRequests
            $cursor = 0
            $stageReports = @()

            foreach ($stageRps in $StageValues) {
                $requestCount = $stageRps * $Duration
                $stageMessages = $allMessages[$cursor..($cursor + $requestCount - 1)]
                $cursor += $requestCount

                $messageFile = Join-Path $VariantDir ("messages_${stageRps}rps.txt")
                Write-StageMessageFile -Path $messageFile -Messages $stageMessages

                Write-Section "Running $($variant.Name) at ${stageRps} RPS for ${Duration}s"
                $benchJson = & $ResolvedPythonExe $LoadTestScript `
                    --base-url $BaseUrl `
                    --rps $stageRps `
                    --duration $Duration `
                    --concurrency $stageRps `
                    --timeout $TimeoutSeconds `
                    --runtime-mode auto `
                    --message-file $messageFile

                $benchReport = $benchJson | ConvertFrom-Json
                $stageMetrics = $null
                if (-not $SkipMetrics) {
                    $stageMetrics = Get-MetricsSnapshot -BaseUrl $BaseUrl
                }

                $stageReports += [pscustomobject]@{
                    stage_rps        = $stageRps
                    concurrency      = $stageRps
                    duration_seconds = $Duration
                    timeout_seconds  = $TimeoutSeconds
                    request_count    = $requestCount
                    benchmark        = $benchReport
                    metrics_snapshot = $stageMetrics
                }

                Write-Host ($benchReport | ConvertTo-Json -Depth 10)
            }

            $variantReports += [pscustomobject]@{
                name            = $variant.Name
                label           = $variant.Label
                primary_model   = $variant.PrimaryModel
                fallback_model  = $variant.FallbackModel
                disable_fast_path = $variant.DisableFastPath
                database_path   = $DatabasePath
                warmup_results  = $warmupResults
                stages          = $stageReports
                server_out_log  = $ServerOutLog
                server_err_log  = $ServerErrLog
            }
        } finally {
            if (-not $KeepRunning) {
                Stop-ManagedProcess -Process $ServerProcess
                $ServerProcess = $null
            }
        }
    }

    $baseline = $variantReports | Where-Object { $_.name -eq 'baseline_gemma_only' } | Select-Object -First 1
    $optimized = $variantReports | Where-Object { $_.name -eq 'optimized_qwen_plus_gemma' } | Select-Object -First 1

    $comparisons = @()
    foreach ($stageRps in $StageValues) {
        $baselineStage = $baseline.stages | Where-Object { $_.stage_rps -eq $stageRps } | Select-Object -First 1
        $optimizedStage = $optimized.stages | Where-Object { $_.stage_rps -eq $stageRps } | Select-Object -First 1
        if ($null -eq $baselineStage -or $null -eq $optimizedStage) {
            continue
        }

        $baselineBench = $baselineStage.benchmark
        $optimizedBench = $optimizedStage.benchmark

        $comparisons += [pscustomobject]@{
            stage_rps                             = $stageRps
            baseline_success_count                = $baselineBench.success_count
            optimized_success_count               = $optimizedBench.success_count
            baseline_error_rate                   = $baselineBench.error_rate
            optimized_error_rate                  = $optimizedBench.error_rate
            baseline_achieved_rps                 = $baselineBench.achieved_rps
            optimized_achieved_rps                = $optimizedBench.achieved_rps
            baseline_avg_latency_ms               = $baselineBench.avg_latency_ms
            optimized_avg_latency_ms              = $optimizedBench.avg_latency_ms
            avg_latency_improvement_pct           = Percent-Change -Baseline $baselineBench.avg_latency_ms -Current $optimizedBench.avg_latency_ms
            baseline_p95_latency_ms               = $baselineBench.p95_latency_ms
            optimized_p95_latency_ms              = $optimizedBench.p95_latency_ms
            p95_latency_improvement_pct           = Percent-Change -Baseline $baselineBench.p95_latency_ms -Current $optimizedBench.p95_latency_ms
            baseline_avg_planner_latency_ms       = $baselineBench.avg_planner_latency_ms
            optimized_avg_planner_latency_ms      = $optimizedBench.avg_planner_latency_ms
            avg_planner_latency_improvement_pct   = Percent-Change -Baseline $baselineBench.avg_planner_latency_ms -Current $optimizedBench.avg_planner_latency_ms
            baseline_planning_source_histogram    = $baselineBench.planning_source_histogram
            optimized_planning_source_histogram   = $optimizedBench.planning_source_histogram
        }
    }

    $report = [pscustomobject]@{
        generated_at       = (Get-Date).ToString('s')
        base_url           = $BaseUrl
        ollama_base_url    = $OllamaBaseUrl
        run_dir            = $RunDir
        stages             = $StageValues
        duration_seconds   = $Duration
        timeout_seconds    = $TimeoutSeconds
        use_docker         = [bool]$UseDocker
        ollama_keep_alive  = $OllamaKeepAlive
        warmup_passes      = $WarmupPasses
        variants           = $variantReports
        comparisons        = $comparisons
    }

    $report | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $ReportPath -Encoding UTF8

    Write-Section 'Comparison summary'
    $comparisons | ConvertTo-Json -Depth 10

    Write-Section 'Saved report'
    Write-Host $ReportPath

    if ($KeepRunning) {
        Write-Section 'KeepRunning enabled'
        Write-Host "Run dir    : $RunDir"
        Write-Host "Ollama PID : $($OllamaProcess.Id)"
        Write-Host "Ollama logs: $OllamaOutLog / $OllamaErrLog"
    }
} catch {
    Write-Host $_.Exception.Message -ForegroundColor Red
    Show-LogTail -Label 'Ollama stderr' -Path $OllamaErrLog
    if ($ServerProcess -ne $null) {
        Show-LogTail -Label 'Server stderr' -Path (Join-Path $RunDir 'baseline_gemma_only\server.err.log')
        Show-LogTail -Label 'Server stderr' -Path (Join-Path $RunDir 'optimized_qwen_plus_gemma\server.err.log')
    }
    throw
} finally {
    if (-not $KeepRunning) {
        Write-Section 'Stopping temporary processes'
        Stop-ManagedProcess -Process $ServerProcess
        Stop-ManagedProcess -Process $OllamaProcess
    }
}
