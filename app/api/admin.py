from fastapi import APIRouter
from fastapi.responses import HTMLResponse


router = APIRouter(tags=['admin'])


ADMIN_HTML = """<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ops-agent-copilot admin</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f4f7fb;
      --panel: rgba(255, 255, 255, 0.94);
      --line: #d7e0eb;
      --text: #16202a;
      --muted: #617284;
      --accent: #0d63c8;
      --accent-soft: #eaf3ff;
      --ok: #0e8b4b;
      --warn: #b56a00;
      --danger: #c24040;
      --shadow: 0 12px 36px rgba(31, 52, 84, 0.08);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      font-family: "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
      color: var(--text);
      background:
        radial-gradient(circle at top left, rgba(13, 99, 200, 0.08), transparent 30%),
        linear-gradient(180deg, #eef4fb 0%, var(--bg) 100%);
    }
    header {
      padding: 22px 24px 18px;
      border-bottom: 1px solid var(--line);
      background: rgba(255, 255, 255, 0.86);
      backdrop-filter: blur(12px);
      position: sticky;
      top: 0;
      z-index: 20;
    }
    h1 {
      margin: 0 0 8px;
      font-size: 26px;
      letter-spacing: 0.2px;
    }
    .sub {
      color: var(--muted);
      font-size: 14px;
    }
    .wrap {
      padding: 22px 24px 28px;
      display: grid;
      grid-template-columns: minmax(640px, 1.2fr) minmax(420px, 1fr);
      gap: 20px;
      align-items: start;
    }
    .card {
      background: var(--panel);
      border: 1px solid rgba(215, 224, 235, 0.9);
      border-radius: 18px;
      overflow: hidden;
      box-shadow: var(--shadow);
    }
    .card-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      padding: 16px 18px;
      border-bottom: 1px solid var(--line);
      background: rgba(255, 255, 255, 0.7);
    }
    .card-head h2 {
      margin: 0;
      font-size: 18px;
    }
    .tools, .filters {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      align-items: center;
    }
    .body {
      padding: 16px 18px;
    }
    input, select, button {
      font: inherit;
      border-radius: 10px;
      border: 1px solid var(--line);
      padding: 8px 10px;
      background: #fff;
      color: inherit;
    }
    input, select {
      min-width: 130px;
    }
    button {
      cursor: pointer;
      transition: transform 0.15s ease, box-shadow 0.15s ease, border-color 0.15s ease;
    }
    button:hover {
      transform: translateY(-1px);
      box-shadow: 0 8px 18px rgba(22, 32, 42, 0.08);
    }
    .primary {
      background: var(--accent);
      color: #fff;
      border-color: var(--accent);
    }
    .ghost {
      background: #f7f9fc;
    }
    .danger {
      background: #fff3f3;
      color: var(--danger);
      border-color: #efc5c5;
    }
    .linkish {
      background: var(--accent-soft);
      color: var(--accent);
      border-color: #cfe1fb;
    }
    .table-wrap {
      overflow-x: auto;
    }
    .table {
      width: 100%;
      border-collapse: collapse;
      font-size: 13px;
    }
    .table th, .table td {
      text-align: left;
      padding: 11px 8px;
      border-bottom: 1px solid #edf2f7;
      vertical-align: top;
    }
    .table th {
      color: var(--muted);
      font-weight: 600;
      white-space: nowrap;
    }
    .mono {
      font-family: Consolas, "SFMono-Regular", monospace;
      font-size: 12px;
      word-break: break-all;
    }
    .status {
      display: inline-flex;
      align-items: center;
      padding: 3px 9px;
      border-radius: 999px;
      font-size: 12px;
      background: #edf4ff;
      color: #2f5fab;
    }
    .status.approved {
      background: #e8f8ef;
      color: var(--ok);
    }
    .status.rejected {
      background: #fff1f1;
      color: var(--danger);
    }
    .status.execution_failed {
      background: #fff1f1;
      color: var(--danger);
    }
    .status.pending {
      background: #fff5e7;
      color: var(--warn);
    }
    .stack {
      display: grid;
      gap: 12px;
    }
    .hint {
      color: var(--muted);
      font-size: 13px;
    }
    .log-item, .modal-section {
      border: 1px solid #ebf0f6;
      border-radius: 14px;
      padding: 12px;
      background: #fbfcfe;
    }
    .meta {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      margin-bottom: 8px;
      color: var(--muted);
      font-size: 12px;
    }
    .pill {
      display: inline-flex;
      align-items: center;
      padding: 2px 8px;
      border-radius: 999px;
      background: #f0f4f9;
      color: #486071;
    }
    .ticket-button-group {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
    }
    .kv-grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 10px;
    }
    .kv {
      border: 1px solid #edf2f7;
      border-radius: 12px;
      padding: 10px;
      background: #fff;
    }
    .kv .label {
      color: var(--muted);
      font-size: 12px;
      margin-bottom: 6px;
    }
    .kv .value {
      font-size: 13px;
      line-height: 1.5;
      word-break: break-word;
    }
    .modal-mask {
      position: fixed;
      inset: 0;
      display: none;
      align-items: center;
      justify-content: center;
      padding: 20px;
      background: rgba(11, 18, 28, 0.45);
      z-index: 50;
    }
    .modal-mask.open {
      display: flex;
    }
    .modal {
      width: min(920px, 100%);
      max-height: calc(100vh - 40px);
      overflow: auto;
      background: #fff;
      border-radius: 18px;
      border: 1px solid rgba(215, 224, 235, 0.9);
      box-shadow: 0 24px 60px rgba(11, 18, 28, 0.22);
    }
    .modal-head {
      position: sticky;
      top: 0;
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 12px;
      padding: 16px 18px;
      background: rgba(255, 255, 255, 0.94);
      border-bottom: 1px solid var(--line);
      backdrop-filter: blur(10px);
    }
    .modal-head h3 {
      margin: 0;
      font-size: 18px;
    }
    .modal-body {
      padding: 16px 18px 18px;
      display: grid;
      gap: 12px;
    }
    .section-title {
      margin: 0 0 10px;
      font-size: 14px;
    }
    pre {
      margin: 0;
      white-space: pre-wrap;
      word-break: break-word;
      font-family: Consolas, "SFMono-Regular", monospace;
      font-size: 12px;
      line-height: 1.55;
    }
    @media (max-width: 1180px) {
      .wrap {
        grid-template-columns: 1fr;
      }
    }
    @media (max-width: 760px) {
      header, .wrap {
        padding-left: 16px;
        padding-right: 16px;
      }
      .kv-grid {
        grid-template-columns: 1fr;
      }
      .card-head {
        align-items: flex-start;
      }
    }
  </style>
</head>
<body>
  <header>
    <h1>ops-agent-copilot 管理页</h1>
    <div class="sub">浏览审批单、查看工单详情、按 trace_id 或事件类型筛选审计日志，并直接完成审批操作。</div>
  </header>
  <main class="wrap">
    <section class="card">
      <div class="card-head">
        <h2>审批列表</h2>
        <div class="tools">
          <select id="approval-status">
            <option value="">全部状态</option>
            <option value="pending" selected>pending</option>
            <option value="approved">approved</option>
            <option value="rejected">rejected</option>
            <option value="executed">executed</option>
            <option value="execution_failed">execution_failed</option>
          </select>
          <input id="approver-user-id" type="number" value="2" min="1" />
          <button class="primary" id="refresh-approvals">刷新</button>
        </div>
      </div>
      <div class="body table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th>审批单</th>
              <th>动作</th>
              <th>目标</th>
              <th>状态</th>
              <th>trace_id</th>
              <th>创建时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody id="approval-body"></tbody>
        </table>
      </div>
    </section>
    <section class="card">
      <div class="card-head">
        <h2>审计日志</h2>
        <div class="filters">
          <input id="trace-id-input" placeholder="trace_id" />
          <select id="event-type-select">
            <option value="">全部事件</option>
          </select>
          <button class="primary" id="load-audit">查询</button>
          <button class="ghost" id="load-recent-audit">最近日志</button>
        </div>
      </div>
      <div class="body stack">
        <div class="hint" id="audit-summary">默认展示最近审计日志，可进一步按 trace_id 或事件类型过滤。</div>
        <div id="audit-list" class="stack"></div>
      </div>
    </section>
  </main>

  <div id="ticket-modal-mask" class="modal-mask">
    <div class="modal">
      <div class="modal-head">
        <div>
          <h3 id="ticket-modal-title">工单详情</h3>
          <div class="hint" id="ticket-modal-subtitle">加载中...</div>
        </div>
        <button class="ghost" id="ticket-modal-close">关闭</button>
      </div>
      <div class="modal-body" id="ticket-modal-body"></div>
    </div>
  </div>

  <script>
    const approvalBody = document.getElementById('approval-body');
    const auditList = document.getElementById('audit-list');
    const auditSummary = document.getElementById('audit-summary');
    const traceIdInput = document.getElementById('trace-id-input');
    const approverUserIdInput = document.getElementById('approver-user-id');
    const approvalStatusInput = document.getElementById('approval-status');
    const eventTypeSelect = document.getElementById('event-type-select');
    const ticketModalMask = document.getElementById('ticket-modal-mask');
    const ticketModalTitle = document.getElementById('ticket-modal-title');
    const ticketModalSubtitle = document.getElementById('ticket-modal-subtitle');
    const ticketModalBody = document.getElementById('ticket-modal-body');

    function pretty(value) {
      return JSON.stringify(value ?? {}, null, 2);
    }

    function escapeHtml(value) {
      return String(value ?? '')
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;')
        .replaceAll('"', '&quot;')
        .replaceAll("'", '&#39;');
    }

    function setEventTypeOptions(values, selectedValue = '') {
      const existing = eventTypeSelect.value;
      const targetValue = selectedValue || existing || '';
      eventTypeSelect.innerHTML = '<option value="">全部事件</option>';
      for (const item of values || []) {
        const option = document.createElement('option');
        option.value = item;
        option.textContent = item;
        if (item === targetValue) option.selected = true;
        eventTypeSelect.appendChild(option);
      }
      if (![...eventTypeSelect.options].some((option) => option.value === targetValue)) {
        eventTypeSelect.value = '';
      }
    }

    async function requestJson(url, options) {
      const response = await fetch(url, options);
      const data = await response.json();
      if (!response.ok) {
        throw new Error(data.detail || JSON.stringify(data));
      }
      return data;
    }

    function formatList(items, renderItem, emptyText) {
      if (!items || !items.length) {
        return `<div class="hint">${escapeHtml(emptyText)}</div>`;
      }
      return items.map(renderItem).join('');
    }

    async function loadApprovals() {
      try {
        approvalBody.innerHTML = '<tr><td colspan="7">加载中...</td></tr>';
        const params = new URLSearchParams({ limit: '20' });
        if (approvalStatusInput.value) {
          params.set('status', approvalStatusInput.value);
        }
        const data = await requestJson(`/api/v1/approvals?${params.toString()}`);
        approvalBody.innerHTML = '';
        if (!data.items.length) {
          approvalBody.innerHTML = '<tr><td colspan="7">暂无审批记录</td></tr>';
          return;
        }
        for (const item of data.items) {
          const tr = document.createElement('tr');
          const ticketButtons = item.target_id
            ? `<div class="ticket-button-group">
                <button class="ghost mono" onclick="showTicketDetail('${escapeHtml(item.target_id)}')">${escapeHtml(item.target_id)}</button>
              </div>`
            : '<span class="hint">-</span>';
          const actionButtons = item.status === 'pending'
            ? `<button class="primary" onclick="approveItem('${escapeHtml(item.approval_no)}')">通过</button>
               <button class="danger" onclick="rejectItem('${escapeHtml(item.approval_no)}')">拒绝</button>`
            : '<span class="hint">已结束</span>';
          tr.innerHTML = `
            <td class="mono">${escapeHtml(item.approval_no)}</td>
            <td>${escapeHtml(item.action_type)}</td>
            <td>${ticketButtons}</td>
            <td><span class="status ${escapeHtml(item.status)}">${escapeHtml(item.status)}</span></td>
            <td><button class="linkish mono" onclick="loadAuditByTrace('${escapeHtml(item.trace_id)}')">${escapeHtml(item.trace_id)}</button></td>
            <td>${escapeHtml(item.created_at)}</td>
            <td>${actionButtons}</td>
          `;
          approvalBody.appendChild(tr);
        }
      } catch (error) {
        approvalBody.innerHTML = `<tr><td colspan="7">加载失败: ${escapeHtml(error.message)}</td></tr>`;
      }
    }

    async function approveItem(approvalNo) {
      const approverUserId = Number(approverUserIdInput.value || 2);
      await requestJson(`/api/v1/approvals/${approvalNo}/approve`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ approver_user_id: approverUserId })
      });
      await loadApprovals();
    }

    async function rejectItem(approvalNo) {
      const approverUserId = Number(approverUserIdInput.value || 2);
      const reason = window.prompt('请输入拒绝原因', '需要重新确认执行对象');
      if (!reason) return;
      await requestJson(`/api/v1/approvals/${approvalNo}/reject`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ approver_user_id: approverUserId, reason })
      });
      await loadApprovals();
    }

    async function loadAudit(traceId = '') {
      try {
        const queryTraceId = traceId || traceIdInput.value.trim();
        const params = new URLSearchParams();
        if (queryTraceId) {
          params.set('trace_id', queryTraceId);
        } else {
          params.set('limit', '30');
        }
        if (eventTypeSelect.value) {
          params.set('event_type', eventTypeSelect.value);
        }
        const data = await requestJson(`/api/v1/audit?${params.toString()}`);
        renderAudit(data);
      } catch (error) {
        auditSummary.textContent = `日志加载失败: ${error.message}`;
        auditList.innerHTML = '';
      }
    }

    function renderAudit(data) {
      setEventTypeOptions(data.available_event_types || [], data.event_type || '');
      auditList.innerHTML = '';

      const summaryParts = [];
      if (data.trace_id) {
        summaryParts.push(`trace_id=${data.trace_id}`);
      } else {
        summaryParts.push('最近日志');
      }
      if (data.event_type) {
        summaryParts.push(`事件类型=${data.event_type}`);
      }
      summaryParts.push(`共 ${data.count} 条审计事件`);
      auditSummary.textContent = summaryParts.join('，');

      if (!data.logs.length) {
        auditList.innerHTML = '<div class="hint">暂无日志</div>';
        return;
      }

      for (const log of data.logs) {
        const card = document.createElement('div');
        card.className = 'log-item';
        card.innerHTML = `
          <div class="meta">
            <span>${escapeHtml(log.created_at)}</span>
            <span class="mono">${escapeHtml(log.trace_id)}</span>
            <span class="pill">${escapeHtml(log.event_type)}</span>
            <span>session=${escapeHtml(log.session_id || '-')}</span>
            <span>user=${escapeHtml(log.user_id || '-')}</span>
          </div>
          <pre>${escapeHtml(pretty(log.event_data))}</pre>
        `;
        auditList.appendChild(card);
      }

      if (data.tool_calls && data.tool_calls.length) {
        const toolTitle = document.createElement('div');
        toolTitle.className = 'hint';
        toolTitle.textContent = `工具调用 ${data.tool_calls.length} 条`;
        auditList.appendChild(toolTitle);
        for (const item of data.tool_calls) {
          const card = document.createElement('div');
          card.className = 'log-item';
          card.innerHTML = `
            <div class="meta">
              <span>${escapeHtml(item.created_at)}</span>
              <span class="pill">${escapeHtml(item.tool_name)}</span>
              <span>${escapeHtml(item.tool_type)}</span>
              <span>${item.success ? 'success' : 'failed'}</span>
              <span>${escapeHtml(item.latency_ms)} ms</span>
            </div>
            <pre>${escapeHtml(pretty({ input: item.input_payload, output: item.output_payload, error: item.error_message }))}</pre>
          `;
          auditList.appendChild(card);
        }
      }
    }

    function renderKvGrid(ticket) {
      const fields = [
        ['工单号', ticket.ticket_no],
        ['区域', ticket.region],
        ['类目', ticket.category],
        ['状态', ticket.status],
        ['优先级', ticket.priority],
        ['根因', ticket.root_cause || '-'],
        ['当前处理人', ticket.assignee_name || '-'],
        ['提单人', ticket.reporter_name || '-'],
        ['SLA 截止', ticket.sla_deadline],
        ['创建时间', ticket.created_at],
        ['更新时间', ticket.updated_at],
        ['解决时间', ticket.resolved_at || '-']
      ];
      return fields.map(([label, value]) => `
        <div class="kv">
          <div class="label">${escapeHtml(label)}</div>
          <div class="value">${escapeHtml(value)}</div>
        </div>
      `).join('');
    }

    async function showTicketDetail(ticketNo) {
      ticketModalMask.classList.add('open');
      ticketModalTitle.textContent = `工单详情 ${ticketNo}`;
      ticketModalSubtitle.textContent = '加载中...';
      ticketModalBody.innerHTML = '<div class="hint">正在加载工单详情...</div>';

      try {
        const data = await requestJson(`/api/v1/tickets/${encodeURIComponent(ticketNo)}`);
        const ticket = data.ticket;
        ticketModalTitle.textContent = `${ticket.ticket_no} · ${ticket.title}`;
        ticketModalSubtitle.textContent = `${ticket.region} / ${ticket.category} / ${ticket.priority}`;
        ticketModalBody.innerHTML = `
          <section class="modal-section">
            <h4 class="section-title">基础信息</h4>
            <div class="kv-grid">${renderKvGrid(ticket)}</div>
          </section>
          <section class="modal-section">
            <h4 class="section-title">问题描述</h4>
            <pre>${escapeHtml(ticket.description)}</pre>
          </section>
          <section class="modal-section">
            <h4 class="section-title">最近评论</h4>
            ${formatList(
              data.comments,
              (item) => `
                <div class="log-item">
                  <div class="meta">
                    <span>${escapeHtml(item.created_at)}</span>
                    <span>${escapeHtml(item.created_by || '-')}</span>
                  </div>
                  <pre>${escapeHtml(item.comment_text)}</pre>
                </div>
              `,
              '暂无评论'
            )}
          </section>
          <section class="modal-section">
            <h4 class="section-title">最近动作</h4>
            ${formatList(
              data.actions,
              (item) => `
                <div class="log-item">
                  <div class="meta">
                    <span>${escapeHtml(item.created_at)}</span>
                    <span class="pill">${escapeHtml(item.action_type)}</span>
                    <button class="linkish mono" onclick="loadAuditByTrace('${escapeHtml(item.trace_id)}')">${escapeHtml(item.trace_id)}</button>
                  </div>
                  <pre>${escapeHtml(pretty({ old_value: item.old_value, new_value: item.new_value }))}</pre>
                </div>
              `,
              '暂无动作记录'
            )}
          </section>
        `;
      } catch (error) {
        ticketModalSubtitle.textContent = '加载失败';
        ticketModalBody.innerHTML = `<div class="hint">加载失败: ${escapeHtml(error.message)}</div>`;
      }
    }

    function closeTicketModal() {
      ticketModalMask.classList.remove('open');
    }

    function loadAuditByTrace(traceId) {
      traceIdInput.value = traceId;
      loadAudit(traceId);
    }

    document.getElementById('refresh-approvals').addEventListener('click', loadApprovals);
    document.getElementById('load-audit').addEventListener('click', () => loadAudit());
    document.getElementById('load-recent-audit').addEventListener('click', () => {
      traceIdInput.value = '';
      loadAudit('');
    });
    document.getElementById('ticket-modal-close').addEventListener('click', closeTicketModal);
    ticketModalMask.addEventListener('click', (event) => {
      if (event.target === ticketModalMask) {
        closeTicketModal();
      }
    });

    window.approveItem = approveItem;
    window.rejectItem = rejectItem;
    window.loadAuditByTrace = loadAuditByTrace;
    window.showTicketDetail = showTicketDetail;

    loadApprovals();
    loadAudit('');
  </script>
</body>
</html>
"""


@router.get('/admin', response_class=HTMLResponse, include_in_schema=False)
async def admin_page() -> HTMLResponse:
    return HTMLResponse(ADMIN_HTML)
