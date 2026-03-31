param(
    [string]$BindHost = '127.0.0.1',
    [int]$Port = 18000,
    [switch]$NoReload,
    [switch]$SkipSeed,
    [switch]$SkipMigrate,
    [switch]$SkipDocker,
    [switch]$SkipInstall,
    [switch]$SkipLLMCheck
)

$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

$VenvDir = Join-Path $PSScriptRoot '.venv'
$PythonExe = Join-Path $VenvDir 'Scripts\python.exe'
$ActivateScript = Join-Path $VenvDir 'Scripts\Activate.ps1'
$RequirementsFile = Join-Path $PSScriptRoot 'requirements.txt'
$DepsStamp = Join-Path $VenvDir '.deps_installed'
$SystemPython = 'python'

function Invoke-Step {
    param(
        [string]$Name,
        [scriptblock]$Action
    )

    Write-Host "==> $Name" -ForegroundColor Cyan
    & $Action
}

function Test-VenvHealthy {
    if (-not (Test-Path $PythonExe)) {
        return $false
    }

    if (-not (Test-Path $ActivateScript)) {
        return $false
    }

    & $PythonExe -m pip --version *> $null
    return $LASTEXITCODE -eq 0
}

function Test-DependenciesInstalled {
    if (-not (Test-Path $PythonExe)) {
        return $false
    }

    & $PythonExe -c "import fastapi, uvicorn, sqlalchemy, alembic, redis, httpx" *> $null
    return $LASTEXITCODE -eq 0
}

function Test-LLMConnectivity {
    $llmCheckScript = @'
import asyncio
import sys

from app.core.config import Settings
from app.services.llm_service import LLMService


async def main() -> int:
    settings = Settings()
    api_key = settings.openai_api_key.strip()
    placeholders = {'', 'sk-test', 'sk-xxx', 'your_openai_api_key_here', 'your-openai-api-key'}
    if api_key in placeholders:
        print(f'LLM_KEY_MISSING: OPENAI_API_KEY is still a placeholder: {api_key!r}')
        return 2

    service = LLMService(settings)
    try:
        result = await service.responses_create(
            input_items=[{'role': 'user', 'content': 'ping'}],
            instructions='Return a tiny acknowledgement.',
        )
    except Exception as exc:
        print(f'LLM_CHECK_FAILED: {exc}')
        return 3

    print('LLM_CHECK_OK')
    if isinstance(result, dict):
        print(f'LLM_CHECK_RESULT_KEYS: {sorted(result.keys())}')
    return 0


raise SystemExit(asyncio.run(main()))
'@

    & $PythonExe -c $llmCheckScript
    return $LASTEXITCODE -eq 0
}

if (-not (Test-VenvHealthy)) {
    Invoke-Step 'Bootstrapping virtual environment' {
        & $SystemPython -m venv .venv --upgrade-deps
    }
}

if (-not $SkipInstall) {
    $needInstall = -not (Test-DependenciesInstalled)

    if (-not $needInstall -and (Test-Path $DepsStamp)) {
        $needInstall = (Get-Item $RequirementsFile).LastWriteTimeUtc -gt (Get-Item $DepsStamp).LastWriteTimeUtc
    } elseif (-not $needInstall) {
        Set-Content -Path $DepsStamp -Value (Get-Date).ToString('o')
    }

    if ($needInstall) {
        Invoke-Step 'Installing Python dependencies' {
            & $PythonExe -m pip install -r $RequirementsFile
        }

        Set-Content -Path $DepsStamp -Value (Get-Date).ToString('o')
    }
}

if (-not $SkipLLMCheck) {
    Invoke-Step 'Checking LLM key and provider connectivity' {
        if (-not (Test-LLMConnectivity)) {
            throw 'LLM preflight check failed. Fix OPENAI_API_KEY / OPENAI_BASE_URL, or rerun with -SkipLLMCheck.'
        }
    }
}

if (-not $SkipDocker) {
    Invoke-Step 'Checking Docker Desktop' {
        docker version | Out-Null
    }

    Invoke-Step 'Starting mysql / redis / adminer' {
        docker compose up -d
    }
}

if (-not $SkipMigrate) {
    Invoke-Step 'Running alembic migrations' {
        & $PythonExe -m alembic upgrade head
    }
}

if (-not $SkipSeed) {
    Invoke-Step 'Seeding demo data' {
        & $PythonExe -m scripts.init_demo_data
    }
}

$pythonArgs = @('-m', 'scripts.run_api', '--host', $BindHost, '--port', [string]$Port)
if (-not $NoReload) {
    $pythonArgs += '--reload'
}

Invoke-Step "Starting API on http://$BindHost`:$Port" {
    & $PythonExe @pythonArgs
}
