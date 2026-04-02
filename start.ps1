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
$AuthFile = Join-Path $PSScriptRoot 'auth.json'

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

function Get-ResolvedOpenAIKey {
    $key = Get-ResolvedSetting -Key 'OPENAI_API_KEY'
    if ($key) {
        return $key
    }

    if (Test-Path $AuthFile) {
        try {
            $payload = Get-Content $AuthFile -Raw | ConvertFrom-Json
            if ($payload.OPENAI_API_KEY) {
                return [string]$payload.OPENAI_API_KEY
            }
        } catch {
        }
    }

    return ''
}

function Test-RealOpenAIKey {
    param([string]$ApiKey)

    $placeholders = @('', 'sk-test', 'sk-xxx', 'your_openai_api_key_here', 'your-openai-api-key')
    return -not ($placeholders -contains $ApiKey.Trim())
}

function Test-LLMConnectivity {
    $mode = Get-ResolvedSetting -Key 'AGENT_RUNTIME_MODE' -Default 'auto'
    if ($mode -eq 'heuristic') {
        Write-Host 'Skipping LLM preflight because AGENT_RUNTIME_MODE=heuristic.' -ForegroundColor Yellow
        return
    }

    $baseUrl = Get-ResolvedSetting -Key 'OPENAI_BASE_URL' -Default 'https://api.moonshot.cn/v1'
    $model = Get-ResolvedSetting -Key 'OPENAI_MODEL' -Default 'kimi-k2-0905-preview'
    $apiKey = Get-ResolvedOpenAIKey

    if (-not (Test-RealOpenAIKey -ApiKey $apiKey)) {
        throw 'LLM preflight failed: OPENAI_API_KEY is still a placeholder. Fix .env or auth.json, or rerun with -SkipLLMCheck.'
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

if ($SkipInstall) {
    Write-Host '-SkipInstall is now a no-op for the Go main service. Python is only needed for offline eval/load scripts.' -ForegroundColor Yellow
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

Invoke-Step "Starting Go API on http://$BindHost`:$Port" {
    & $ServerExe
}

