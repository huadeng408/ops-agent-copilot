param(
    [string]$BaseUrl = 'http://127.0.0.1:18000'
)

$ErrorActionPreference = 'Stop'

function Invoke-JsonPost {
    param(
        [string]$Uri,
        [string]$BodyJson
    )

    return Invoke-RestMethod -Method Post -Uri $Uri -ContentType 'application/json; charset=utf-8' -Body $BodyJson
}

Write-Host "==> Health" -ForegroundColor Cyan
$health = Invoke-RestMethod -Method Get -Uri "$BaseUrl/healthz"
$health | ConvertTo-Json -Depth 5

Write-Host "==> Readonly chat query" -ForegroundColor Cyan
$metricResponse = Invoke-JsonPost -Uri "$BaseUrl/api/v1/chat" -BodyJson '{"session_id":"interview_metric","user_id":1,"message":"\u6700\u8fd17\u5929\u5317\u4eac\u533a\u9000\u6b3e\u7387\u6700\u9ad8\u7684\u7c7b\u76ee\u662f\u4ec0\u4e48\uff1f"}'
$metricResponse | ConvertTo-Json -Depth 8

Write-Host "==> Write request becomes proposal" -ForegroundColor Cyan
$writeResponse = Invoke-JsonPost -Uri "$BaseUrl/api/v1/chat" -BodyJson '{"session_id":"interview_write","user_id":1,"message":"\u628aT202603280012 \u5206\u6d3e\u7ed9\u738b\u78ca"}'
$writeResponse | ConvertTo-Json -Depth 8

$approvalNo = $writeResponse.approval.approval_no
if (-not $approvalNo) {
    throw 'approval_no not found in write response'
}

Write-Host "==> Approve proposal" -ForegroundColor Cyan
$approveResponse = Invoke-JsonPost -Uri "$BaseUrl/api/v1/approvals/$approvalNo/approve" -BodyJson '{"approver_id":2}'
$approveResponse | ConvertTo-Json -Depth 8

$traceId = $writeResponse.trace_id

Write-Host "==> Audit by trace_id" -ForegroundColor Cyan
$auditResponse = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/audit?trace_id=$traceId"
$auditResponse | ConvertTo-Json -Depth 8

Write-Host "==> Ticket detail after approval" -ForegroundColor Cyan
$ticketResponse = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/tickets/T202603280012"
$ticketResponse | ConvertTo-Json -Depth 8
