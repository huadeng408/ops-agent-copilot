param(
    [string]$BaseUrl = 'http://127.0.0.1:18001',
    [string[]]$Messages = @(),
    [switch]$ShowRaw,
    [switch]$SkipMetrics
)

$ErrorActionPreference = 'Stop'

if ($Messages.Count -eq 0) {
    $Messages = @(
        [regex]::Unescape('\u5317\u4eac\u533a\u6628\u5929\u8d85SLA\u7684\u5de5\u5355\u6309\u539f\u56e0\u5206\u7c7b'),
        [regex]::Unescape('\u6700\u8fd17\u5929\u5317\u4eac\u533a\u9000\u6b3e\u7387\u6700\u9ad8\u7684\u7c7b\u76ee\u662f\u4ec0\u4e48\uff1f'),
        [regex]::Unescape('\u5317\u4eac\u533a\u6628\u5929\u9000\u6b3e\u7387\u5f02\u5e38\u548c\u8d85SLA\u5de5\u5355\u505a\u4e00\u4e0b\u5f52\u56e0\u5206\u6790')
    )
}

function Read-ExceptionBody {
    param([System.Exception]$Exception)

    $response = $Exception.Response
    if ($null -eq $response) {
        return ''
    }

    try {
        $stream = $response.GetResponseStream()
        if ($null -eq $stream) {
            return ''
        }
        $reader = New-Object System.IO.StreamReader($stream)
        try {
            return $reader.ReadToEnd()
        } finally {
            $reader.Close()
        }
    } catch {
        return ''
    }
}

function Convert-BodyToObject {
    param([string]$Body)

    if ([string]::IsNullOrWhiteSpace($Body)) {
        return $null
    }

    try {
        return $Body | ConvertFrom-Json -ErrorAction Stop
    } catch {
        return $Body
    }
}

function Invoke-JsonGet {
    param([string]$Uri)

    try {
        $response = Invoke-WebRequest -Method Get -Uri $Uri -UseBasicParsing
        return [pscustomobject]@{
            StatusCode = [int]$response.StatusCode
            Body       = Convert-BodyToObject -Body $response.Content
        }
    } catch {
        $body = Read-ExceptionBody -Exception $_.Exception
        $statusCode = 0
        if ($_.Exception.Response -and $_.Exception.Response.StatusCode) {
            $statusCode = [int]$_.Exception.Response.StatusCode
        }
        return [pscustomobject]@{
            StatusCode = $statusCode
            Body       = Convert-BodyToObject -Body $body
        }
    }
}

function Invoke-JsonPost {
    param(
        [string]$Uri,
        [string]$BodyJson
    )

    try {
        $response = Invoke-WebRequest -Method Post -Uri $Uri -ContentType 'application/json; charset=utf-8' -Body $BodyJson -UseBasicParsing
        return [pscustomobject]@{
            StatusCode = [int]$response.StatusCode
            Body       = Convert-BodyToObject -Body $response.Content
        }
    } catch {
        $body = Read-ExceptionBody -Exception $_.Exception
        $statusCode = 0
        if ($_.Exception.Response -and $_.Exception.Response.StatusCode) {
            $statusCode = [int]$_.Exception.Response.StatusCode
        }
        return [pscustomobject]@{
            StatusCode = $statusCode
            Body       = Convert-BodyToObject -Body $body
        }
    }
}

function Format-ToolNames {
    param($ToolCalls)

    if ($null -eq $ToolCalls) {
        return ''
    }

    $names = @()
    foreach ($toolCall in $ToolCalls) {
        if ($toolCall.tool_name) {
            $names += [string]$toolCall.tool_name
        }
    }
    return ($names -join ', ')
}

Write-Host "==> Health" -ForegroundColor Cyan
$health = Invoke-JsonGet -Uri "$BaseUrl/healthz"
$health | ConvertTo-Json -Depth 8

if ($health.StatusCode -lt 200 -or $health.StatusCode -ge 300) {
    throw "Health check failed for $BaseUrl"
}

$results = @()

for ($index = 0; $index -lt $Messages.Count; $index++) {
    $message = $Messages[$index]
    $sessionId = "gemma_manual_$([DateTimeOffset]::Now.ToUnixTimeMilliseconds())_$index"
    $payload = @{
        session_id = $sessionId
        user_id    = 1
        message    = $message
    } | ConvertTo-Json -Depth 6

    Write-Host "==> Chat $($index + 1)" -ForegroundColor Cyan
    Write-Host $message -ForegroundColor Yellow

    $response = Invoke-JsonPost -Uri "$BaseUrl/api/v1/chat" -BodyJson $payload
    $body = $response.Body

    $summary = [pscustomobject]@{
        index              = $index + 1
        http_status        = $response.StatusCode
        session_id         = if ($body -and $body.session_id) { $body.session_id } else { $sessionId }
        status             = if ($body -and $body.status) { $body.status } else { $null }
        planning_source    = if ($body -and $body.planning_source) { $body.planning_source } else { $null }
        planner_latency_ms = if ($body -and $body.planner_latency_ms) { $body.planner_latency_ms } else { $null }
        tool_names         = if ($body -and $body.tool_calls) { Format-ToolNames -ToolCalls $body.tool_calls } else { '' }
        answer_preview     = if ($body -and $body.answer) { ([string]$body.answer).Substring(0, [Math]::Min(120, ([string]$body.answer).Length)) } else { $null }
        error_detail       = if ($body -is [string]) { $body } elseif ($body -and $body.detail) { $body.detail } else { $null }
    }

    $results += $summary
    $summary | Format-List

    if ($ShowRaw) {
        Write-Host '-- raw response --' -ForegroundColor DarkGray
        $body | ConvertTo-Json -Depth 12
    }
}

Write-Host "==> Summary" -ForegroundColor Cyan
$results | Format-Table -AutoSize

if (-not $SkipMetrics) {
    Write-Host "==> Metrics" -ForegroundColor Cyan
    $metricsResponse = Invoke-JsonGet -Uri "$BaseUrl/metrics"
    if ($metricsResponse.StatusCode -ge 200 -and $metricsResponse.StatusCode -lt 300 -and $metricsResponse.Body -is [string]) {
        ($metricsResponse.Body -split "`n") | Where-Object {
            $_ -match 'ops_agent_chat_requests_total' -or
            $_ -match 'ops_agent_planner_requests_total' -or
            $_ -match 'ops_agent_llm_requests_total' -or
            $_ -match 'ops_agent_llm_fallback_total'
        }
    } else {
        $metricsResponse | ConvertTo-Json -Depth 8
    }
}
