param(
    [string]$BindHost = '127.0.0.1',
    [int]$Port = 18000,
    [switch]$NoReload,
    [switch]$SkipSeed,
    [switch]$SkipMigrate,
    [switch]$SkipDocker,
    [switch]$SkipInstall,
    [switch]$SkipLLMCheck,
    [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

$GoExe = (Get-Command go -ErrorAction Stop).Source
$BuildDir = Join-Path $PSScriptRoot '.tmp'
$ServerExe = Join-Path $BuildDir 'ops-agent-go.exe'
$EnvFile = Join-Path $PSScriptRoot '.env'

function Invoke-Step {
    param(
        [string]$Name,
        [scriptblock]$Action
    )

    Write-Host "==> $Name" -ForegroundColor Cyan
    & $Action
}

function Get-DotEnvValue {
    param([string]$Key)

    if (-not (Test-Path $EnvFile)) {
        return $null
    }

    foreach ($line in Get-Content $EnvFile) {
        $trimmed = $line.Trim()
        if (-not $trimmed -or $trimmed.StartsWith('#')) {
            continue
        }
        $parts = $trimmed -split '=', 2
        if ($parts.Count -ne 2) {
            continue
        }
        if ($parts[0].Trim() -ieq $Key) {
            return $parts[1].Trim()
        }
    }

    return $null
}

function Get-ResolvedSetting {
    param(
        [string]$Key,
        [string]$Default = ''
    )

    $fromEnv = [Environment]::GetEnvironmentVariable($Key)
    if ($fromEnv) {
        return $fromEnv
    }

    $fromDotEnv = Get-DotEnvValue -Key $Key
    if ($fromDotEnv) {
        return $fromDotEnv
    }

    return $Default
}

function Get-ResolvedSettingAny {
    param(
        [string[]]$Keys,
        [string]$Default = ''
    )

    foreach ($key in $Keys) {
        $value = Get-ResolvedSetting -Key $key
        if ($value) {
            return $value
        }
    }

    return $Default
}

function Get-ResolvedLLMProvider {
    $provider = (Get-ResolvedSettingAny -Keys @('LLM_PROVIDER') -Default '').Trim().ToLower()
    if ($provider) {
        if ($provider -eq 'openai') {
            return 'kimi'
        }
        if (@('kimi', 'ollama') -notcontains $provider) {
            throw "Unsupported LLM_PROVIDER='$provider'. Only 'kimi' and 'ollama' are allowed."
        }
        return $provider
    }

    $model = (Get-ResolvedSettingAny -Keys @('LLM_MODEL', 'OPENAI_MODEL') -Default '').Trim().ToLower()
    if ($model.StartsWith('gemma4')) {
        return 'ollama'
    }
    $baseUrl = (Get-ResolvedSettingAny -Keys @('LLM_BASE_URL', 'OPENAI_BASE_URL') -Default '').Trim().ToLower()
    if ($baseUrl.Contains(':11434')) {
        return 'ollama'
    }
    return 'kimi'
}

function Get-ResolvedLLMAuthFile {
    param([string]$Provider)

    if ($Provider -eq 'ollama') {
        return Get-ResolvedSettingAny -Keys @('LLM_AUTH_FILE') -Default ''
    }

    return Get-ResolvedSettingAny -Keys @('LLM_AUTH_FILE', 'OPENAI_AUTH_FILE') -Default 'auth.json'
}

function Get-ResolvedLLMKey {
    param([string]$Provider)

    $key = Get-ResolvedSettingAny -Keys @('LLM_API_KEY', 'OPENAI_API_KEY')
    if ($key) {
        return $key
    }

    $authFile = Get-ResolvedLLMAuthFile -Provider $Provider
    if ($authFile -and (Test-Path $authFile)) {
        try {
            $payload = Get-Content $authFile -Raw | ConvertFrom-Json
            if ($payload.LLM_API_KEY) {
                return [string]$payload.LLM_API_KEY
            }
            if ($payload.OPENAI_API_KEY) {
                return [string]$payload.OPENAI_API_KEY
            }
        } catch {
        }
    }

    if ($Provider -eq 'ollama') {
        return 'ollama-local'
    }

    return ''
}

function Get-ResolvedLLMBaseUrl {
    param([string]$Provider)

    $default = if ($Provider -eq 'ollama') { 'http://127.0.0.1:11434/v1' } else { 'https://api.moonshot.cn/v1' }
    return Get-ResolvedSettingAny -Keys @('LLM_BASE_URL', 'OPENAI_BASE_URL') -Default $default
}

function Get-ResolvedLLMModel {
    param([string]$Provider)

    $default = if ($Provider -eq 'ollama') { 'gemma4:e4b' } else { 'kimi-k2-0905-preview' }
    return Get-ResolvedSettingAny -Keys @('LLM_MODEL', 'OPENAI_MODEL') -Default $default
}

function Normalize-RuntimeMode {
    param([string]$Mode)

    switch ($Mode.Trim().ToLower()) {
        'langgraph' { return 'langgraph' }
        'openai' { return 'llm' }
        'llm' { return 'llm' }
        'heuristic' { return 'heuristic' }
        Default { return 'auto' }
    }
}

function Test-RealLLMKey {
    param(
        [string]$Provider,
        [string]$ApiKey
    )

    if ($Provider -eq 'ollama') {
        return $true
    }

    $placeholders = @('', 'sk-test', 'sk-xxx', 'your_openai_api_key_here', 'your-openai-api-key', 'your_llm_api_key_here', 'your-llm-api-key')
    return -not ($placeholders -contains $ApiKey.Trim())
}

function Test-LLMConnectivity {
    $mode = Normalize-RuntimeMode (Get-ResolvedSetting -Key 'AGENT_RUNTIME_MODE' -Default 'auto')
    if ($mode -eq 'heuristic') {
        Write-Host 'Skipping LLM preflight because AGENT_RUNTIME_MODE=heuristic.' -ForegroundColor Yellow
        return
    }

    $provider = Get-ResolvedLLMProvider
    $baseUrl = Get-ResolvedLLMBaseUrl -Provider $provider
    $model = Get-ResolvedLLMModel -Provider $provider
    $apiKey = Get-ResolvedLLMKey -Provider $provider

    if (-not (Test-RealLLMKey -Provider $provider -ApiKey $apiKey)) {
        throw 'LLM preflight failed: LLM_API_KEY is still a placeholder. Fix .env or auth.json, or rerun with -SkipLLMCheck.'
    }

    $uri = ($baseUrl.TrimEnd('/')) + '/chat/completions'
    $body = @{
        model = $model
        messages = @(
            @{
                role = 'user'
                content = 'ping'
            }
        )
    } | ConvertTo-Json -Depth 5

    $headers = @{
        Authorization = "Bearer $apiKey"
        'Content-Type' = 'application/json'
    }

    Invoke-RestMethod -Method Post -Uri $uri -Headers $headers -Body $body | Out-Null
}

if ($NoReload) {
    Write-Host '-NoReload is now a no-op because the Go main service runs as a compiled binary.' -ForegroundColor Yellow
}

if ($SkipMigrate) {
    Write-Host '-SkipMigrate is now a no-op because schema bootstrap is handled by Go startup / seed.' -ForegroundColor Yellow
}

$RuntimeMode = Normalize-RuntimeMode (Get-ResolvedSetting -Key 'AGENT_RUNTIME_MODE' -Default 'auto')
$UseLangGraph = $RuntimeMode -eq 'langgraph'
$PythonExe = $null
$LangGraphProcess = $null

if ($SkipInstall -and -not $UseLangGraph) {
    Write-Host '-SkipInstall is now a no-op for the Go main service. Python is only needed for LangGraph runtime or offline scripts.' -ForegroundColor Yellow
}

if (-not $SkipDocker) {
    Invoke-Step 'Checking Docker Desktop' {
        docker version | Out-Null
    }

    Invoke-Step 'Starting mysql / redis / prometheus / grafana / jaeger' {
        docker compose up -d
    }
}

if (-not $SkipLLMCheck) {
    Invoke-Step 'Checking LLM connectivity' {
        Test-LLMConnectivity
    }
}

if ($UseLangGraph) {
    $PythonExe = (Get-Command python -ErrorAction Stop).Source

    if (-not $SkipInstall) {
        Invoke-Step 'Installing Python runtime dependencies for LangGraph' {
            & $PythonExe -m pip install -r requirements.txt
        }
    }
}

if (-not (Test-Path $BuildDir)) {
    New-Item -ItemType Directory -Path $BuildDir | Out-Null
}

if (-not $SkipBuild -or -not (Test-Path $ServerExe)) {
    Invoke-Step 'Building Go server binary' {
        & $GoExe build -o $ServerExe .\cmd\server
    }
}

if (-not $SkipSeed) {
    Invoke-Step 'Seeding demo data with Go bootstrap' {
        & $GoExe run .\cmd\seed
    }
}

$env:HOST = $BindHost
$env:PORT = [string]$Port
$env:OPS_AGENT_BASE_URL = "http://127.0.0.1:$Port"

if ($UseLangGraph) {
    $langGraphHost = Get-ResolvedSetting -Key 'LANGGRAPH_HOST' -Default '127.0.0.1'
    $langGraphPort = Get-ResolvedSetting -Key 'LANGGRAPH_PORT' -Default '8001'
    if (-not (Get-ResolvedSetting -Key 'LANGGRAPH_BASE_URL')) {
        $env:LANGGRAPH_BASE_URL = "http://$langGraphHost`:$langGraphPort"
    }

    Invoke-Step "Starting LangGraph API on http://$langGraphHost`:$langGraphPort" {
        $LangGraphProcess = Start-Process -FilePath $PythonExe -ArgumentList @(
            '-m', 'uvicorn', 'langgraph_runtime.app:app',
            '--host', $langGraphHost,
            '--port', $langGraphPort
        ) -WorkingDirectory $PSScriptRoot -PassThru
        Start-Sleep -Seconds 2
    }
}

Invoke-Step "Starting Go API on http://$BindHost`:$Port" {
    try {
        & $ServerExe
    } finally {
        if ($LangGraphProcess -and -not $LangGraphProcess.HasExited) {
            Stop-Process -Id $LangGraphProcess.Id -Force
        }
    }
}
