from __future__ import annotations

from collections import defaultdict
from typing import Any

from .router import CATEGORIES, PlannedCall, widen_date_range


def _rows_from_data(data: dict[str, Any]) -> list[dict[str, Any]]:
    rows = data.get("rows")
    if isinstance(rows, list):
        return [item for item in rows if isinstance(item, dict)]
    return []


def verify_step_result(message: str, step: dict[str, Any], result: dict[str, Any]) -> dict[str, Any]:
    tool_name = step["tool_name"]
    data = dict(result.get("data") or {})
    rows = _rows_from_data(data)
    if tool_name == "generate_report":
        return {"passed": bool(str(result.get("answer") or "").strip()), "reason": "日报内容为空"}
    if tool_name == "get_recent_releases":
        return {"passed": True, "reason": ""}
    if tool_name == "get_ticket_comments":
        actions = data.get("actions") or []
        comments = data.get("comments") or []
        if actions or comments:
            return {"passed": True, "reason": ""}
        return {"passed": True, "reason": "工单没有更多操作记录，继续回答"}
    if tool_name == "get_ticket_detail":
        return {"passed": bool(str(data.get("ticket_no") or "").strip()), "reason": "工单详情为空"}
    if tool_name.startswith("propose_"):
        return {
            "passed": bool(result.get("requires_approval")) and bool(result.get("action_type")) and bool(result.get("target_id")),
            "reason": "proposal 信息不完整",
        }
    if tool_name in {"query_refund_metrics", "find_refund_anomalies", "list_sla_breached_tickets"}:
        return {"passed": bool(rows), "reason": "查询结果为空，需要补充检索"}
    return {"passed": True, "reason": ""}


def build_step_reflection(message: str, step: dict[str, Any], result: dict[str, Any]) -> PlannedCall | None:
    tool_name = step["tool_name"]
    arguments = dict(step.get("arguments") or {})
    if tool_name == "query_refund_metrics":
        if arguments.get("category"):
            arguments.pop("category", None)
            return PlannedCall(tool_name, arguments, "放宽类目条件后重查退款率")
        if arguments.get("region"):
            arguments.pop("region", None)
            return PlannedCall(tool_name, arguments, "放宽区域条件后重查退款率")
        widened = widen_date_range(arguments, 7)
        if widened != arguments:
            return PlannedCall(tool_name, widened, "扩大时间窗口后重查退款率")
        return None
    if tool_name == "find_refund_anomalies":
        if int(arguments.get("top_k") or 0) < 10:
            arguments["top_k"] = 10
            return PlannedCall(tool_name, arguments, "扩大异常候选范围后重查")
        if arguments.get("region"):
            arguments.pop("region", None)
            return PlannedCall(tool_name, arguments, "移除区域限制后重查异常")
        widened = widen_date_range(arguments, 7)
        if widened != arguments:
            return PlannedCall(tool_name, widened, "扩大时间窗口后重查异常")
        return None
    if tool_name == "list_sla_breached_tickets":
        if arguments.get("group_by"):
            arguments["group_by"] = ""
            return PlannedCall(tool_name, arguments, "取消分组限制后重查超 SLA 工单")
        if arguments.get("region"):
            arguments.pop("region", None)
            return PlannedCall(tool_name, arguments, "移除区域限制后重查超 SLA 工单")
        return None
    if tool_name.startswith("propose_"):
        if "待确认" in str(arguments.get("assignee_name") or ""):
            return None
        return None
    return None


def build_answer_reflection(message: str, planned_calls: list[dict[str, Any]], step_results: list[dict[str, Any]]) -> list[PlannedCall]:
    existing_tools = {item.get("tool_name") for item in planned_calls}
    completed_tools = {item.get("tool_name") for item in step_results}
    known_tools = existing_tools | completed_tools
    additions: list[PlannedCall] = []

    def add_once(call: PlannedCall) -> None:
        if call.tool_name in known_tools:
            return
        known_tools.add(call.tool_name)
        additions.append(call)

    if "归因" in message:
        if "find_refund_anomalies" not in known_tools:
            add_once(PlannedCall("find_refund_anomalies", {"start_date": "", "end_date": "", "top_k": 5}, "补查退款异常证据"))
        if "list_sla_breached_tickets" not in known_tools:
            add_once(PlannedCall("list_sla_breached_tickets", {"date": "", "group_by": "root_cause"}, "补查超 SLA 根因证据"))
        if "get_recent_releases" not in known_tools:
            add_once(PlannedCall("get_recent_releases", {}, "补查发布记录证据"))

    if "操作记录" in message and "get_ticket_comments" not in known_tools:
        ticket_no = _latest_ticket_no(step_results)
        if ticket_no:
            add_once(PlannedCall("get_ticket_comments", {"ticket_no": ticket_no}, "补查工单操作记录"))
    if "详情" in message and "get_ticket_detail" not in known_tools:
        ticket_no = _latest_ticket_no(step_results)
        if ticket_no:
            add_once(PlannedCall("get_ticket_detail", {"ticket_no": ticket_no}, "补查工单详情"))

    normalized: list[PlannedCall] = []
    for item in additions:
        if item.tool_name == "find_refund_anomalies" and not item.arguments.get("start_date"):
            reference = _latest_date_range(step_results)
            if reference:
                item.arguments.update(reference)
        if item.tool_name == "list_sla_breached_tickets" and not item.arguments.get("date"):
            date_value = _latest_date(step_results)
            if date_value:
                item.arguments["date"] = date_value
        if item.tool_name == "get_ticket_comments" and item.arguments.get("ticket_no"):
            normalized.append(item)
        elif item.tool_name == "get_ticket_detail" and item.arguments.get("ticket_no"):
            normalized.append(item)
        elif item.tool_name == "find_refund_anomalies" and item.arguments.get("start_date") and item.arguments.get("end_date"):
            normalized.append(item)
        elif item.tool_name == "list_sla_breached_tickets" and item.arguments.get("date"):
            normalized.append(item)
        elif item.tool_name == "get_recent_releases":
            normalized.append(item)
    return normalized


def verify_answer(message: str, answer: str, step_results: list[dict[str, Any]], approval: dict[str, Any] | None) -> dict[str, Any]:
    issues: list[str] = []
    stripped = answer.strip()
    if not stripped:
        issues.append("answer_empty")
    if any(token in message for token in ("分派给", "升级", "备注")) and not approval:
        issues.append("missing_approval_handoff")
    if "归因" in message:
        evidence_hits = 0
        if "退款" in stripped:
            evidence_hits += 1
        if "SLA" in stripped or "工单" in stripped:
            evidence_hits += 1
        if "发布" in stripped:
            evidence_hits += 1
        if evidence_hits < 2:
            issues.append("insufficient_anomaly_evidence")
    if ("最高" in message or "Top" in message or "top" in message) and "类目" in message:
        if not any(category in stripped for category in CATEGORIES):
            issues.append("missing_top_category")
    if "操作记录" in message and "操作记录" not in stripped and "最近备注" not in stripped:
        issues.append("missing_operation_history")
    if "详情" in message and "工单" not in stripped:
        issues.append("missing_ticket_detail")

    return {"passed": not issues, "issues": issues}


def synthesize_answer(message: str, step_results: list[dict[str, Any]], status: str, approval: dict[str, Any] | None) -> str:
    if status == "approval_required":
        return "已生成写操作 proposal，需审批后执行。"

    tool_map: dict[str, list[dict[str, Any]]] = defaultdict(list)
    rendered_parts: list[str] = []
    for item in step_results:
        tool_map[item["tool_name"]].append(item)
        rendered = str(item.get("rendered_answer") or "").strip()
        if rendered:
            rendered_parts.append(rendered)

    if "归因" in message and (
        tool_map.get("find_refund_anomalies") or tool_map.get("query_refund_metrics")
    ) and tool_map.get("list_sla_breached_tickets"):
        return _render_anomaly_answer(tool_map)

    if ("最高" in message or "Top" in message or "top" in message) and "类目" in message and tool_map.get("query_refund_metrics"):
        top_answer = _render_top_category_answer(tool_map["query_refund_metrics"][0])
        if top_answer:
            return top_answer

    if tool_map.get("get_ticket_detail") and tool_map.get("get_ticket_comments"):
        detail = str(tool_map["get_ticket_detail"][0].get("rendered_answer") or "").strip()
        comments = str(tool_map["get_ticket_comments"][0].get("rendered_answer") or "").strip()
        return "\n\n".join(part for part in (detail, comments) if part)

    if rendered_parts:
        return "\n\n".join(rendered_parts)
    return "已处理请求。"


def _render_top_category_answer(step_result: dict[str, Any]) -> str:
    rows = _rows_from_data(dict(step_result.get("data") or {}))
    if not rows:
        return ""
    buckets: dict[str, dict[str, float]] = defaultdict(lambda: {"rate_sum": 0.0, "count": 0.0, "refund_orders_cnt": 0.0})
    region = ""
    for row in rows:
        category = str(row.get("category") or "").strip()
        if not category:
            continue
        region = region or str(row.get("region") or "").strip()
        buckets[category]["rate_sum"] += float(row.get("refund_rate") or 0.0)
        buckets[category]["count"] += 1
        buckets[category]["refund_orders_cnt"] += float(row.get("refund_orders_cnt") or 0.0)
    if not buckets:
        return ""
    scored = sorted(
        (
            (
                category,
                values["rate_sum"] / max(values["count"], 1.0),
                int(values["refund_orders_cnt"]),
            )
            for category, values in buckets.items()
        ),
        key=lambda item: item[1],
        reverse=True,
    )
    top_category, avg_rate, refund_cnt = scored[0]
    lines = [
        f"按区间平均退款率看，{region or '该区域'}最高的类目是 {top_category}。",
        f"- 平均退款率：{avg_rate * 100:.2f}%",
        f"- 区间累计退款单量：{refund_cnt}",
    ]
    if len(scored) > 1:
        second_category, second_rate, _ = scored[1]
        lines.append(f"- 次高类目：{second_category}（{second_rate * 100:.2f}%）")
    return "\n".join(lines)


def _render_anomaly_answer(tool_map: dict[str, list[dict[str, Any]]]) -> str:
    refund_rows: list[dict[str, Any]] = []
    if tool_map.get("find_refund_anomalies"):
        refund_rows = _rows_from_data(dict(tool_map["find_refund_anomalies"][0].get("data") or {}))
    elif tool_map.get("query_refund_metrics"):
        refund_rows = _rows_from_data(dict(tool_map["query_refund_metrics"][0].get("data") or {}))
    sla_rows = _rows_from_data(dict(tool_map["list_sla_breached_tickets"][0].get("data") or {}))
    release_rows = _rows_from_data(dict(tool_map.get("get_recent_releases", [{}])[0].get("data") or {}))

    lines = ["异常归因结论：", "", "1. 退款异常"]
    if refund_rows:
        for row in refund_rows[:3]:
            if "avg_refund_rate" in row:
                lines.append(f"- {row.get('category')} 平均退款率 {float(row.get('avg_refund_rate') or 0.0) * 100:.2f}%")
            else:
                lines.append(
                    f"- {row.get('region')}-{row.get('category')} 退款率 {float(row.get('refund_rate') or 0.0) * 100:.2f}%"
                )
    else:
        lines.append("- 暂未识别到明显退款异常")

    lines.extend(["", "2. 超 SLA 工单"])
    if sla_rows:
        for row in sla_rows[:4]:
            if "group_key" in row:
                lines.append(f"- {row.get('group_key') or '未归类'}：{int(row.get('ticket_count') or 0)} 单")
            else:
                lines.append(f"- {row.get('ticket_no')} | {row.get('priority')} | {row.get('root_cause') or '待定'}")
    else:
        lines.append("- 未查询到超 SLA 工单")

    lines.extend(["", "3. 近期发布"])
    if release_rows:
        for row in release_rows[:3]:
            lines.append(f"- {row.get('release_time')} | {row.get('service_name')} {row.get('release_version')} | {row.get('change_summary')}")
    else:
        lines.append("- 异常窗口附近无明显发布记录")

    lines.extend(["", "4. 综合判断"])
    if refund_rows and sla_rows and release_rows:
        lines.append("- 退款异常、超 SLA 工单与近期发布同时出现，建议优先排查发布影响和履约链路。")
    elif refund_rows and sla_rows:
        lines.append("- 退款异常与超 SLA 工单同时上升，建议先按根因分布排查主因并回溯相关发布。")
    elif sla_rows:
        lines.append("- 当前更明显的异常信号来自超 SLA 工单，建议按主因和类目继续下钻。")
    else:
        lines.append("- 现有证据不足，建议扩大时间窗口或补充区域/类目限定。")
    return "\n".join(lines)


def _latest_ticket_no(step_results: list[dict[str, Any]]) -> str:
    for item in reversed(step_results):
        data = dict(item.get("data") or {})
        ticket_no = str(data.get("ticket_no") or item.get("target_id") or "").strip()
        if ticket_no:
            return ticket_no
        payload = dict(item.get("proposal_payload") or {})
        ticket_no = str(payload.get("ticket_no") or "").strip()
        if ticket_no:
            return ticket_no
    return ""


def _latest_date(step_results: list[dict[str, Any]]) -> str:
    for item in reversed(step_results):
        arguments = dict(item.get("arguments") or {})
        value = str(arguments.get("date") or arguments.get("end_date") or "").strip()
        if value:
            return value
    return ""


def _latest_date_range(step_results: list[dict[str, Any]]) -> dict[str, str] | None:
    for item in reversed(step_results):
        arguments = dict(item.get("arguments") or {})
        start_date = str(arguments.get("start_date") or "").strip()
        end_date = str(arguments.get("end_date") or "").strip()
        if start_date and end_date:
            return {"start_date": start_date, "end_date": end_date}
    return None
