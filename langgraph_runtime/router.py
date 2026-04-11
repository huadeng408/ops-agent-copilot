from __future__ import annotations

import json
import re
from dataclasses import dataclass
from datetime import date, datetime, timedelta
from typing import Any, Literal

from langchain_openai import ChatOpenAI
from pydantic import BaseModel, Field

from .config import RuntimeConfig

REGIONS = ("北京", "上海", "广州")
CATEGORIES = ("生鲜", "餐饮", "酒店", "到店综合")

TOOL_DEFINITIONS: dict[str, dict[str, Any]] = {
    "query_refund_metrics": {"required": ("start_date", "end_date"), "type": "readonly"},
    "find_refund_anomalies": {"required": ("start_date", "end_date"), "type": "readonly"},
    "analyze_operational_anomaly": {"required": ("date",), "type": "readonly"},
    "list_sla_breached_tickets": {"required": ("date",), "type": "readonly"},
    "get_ticket_detail": {"required": ("ticket_no",), "type": "readonly"},
    "get_ticket_comments": {"required": ("ticket_no",), "type": "readonly"},
    "get_recent_releases": {"required": (), "type": "readonly"},
    "run_readonly_sql": {"required": ("sql",), "type": "readonly"},
    "propose_assign_ticket": {"required": ("ticket_no", "assignee_name", "reason"), "type": "write"},
    "propose_add_ticket_comment": {"required": ("ticket_no", "comment_text", "reason"), "type": "write"},
    "propose_escalate_ticket": {"required": ("ticket_no", "new_priority", "reason"), "type": "write"},
    "generate_report": {"required": (), "type": "report"},
}


@dataclass(slots=True)
class PlannedCall:
    tool_name: str
    arguments: dict[str, Any]
    purpose: str = ""

    def as_dict(self) -> dict[str, Any]:
        payload = {"tool_name": self.tool_name, "arguments": self.arguments}
        if self.purpose:
            payload["purpose"] = self.purpose
        return payload


@dataclass(slots=True)
class RouteAnalysis:
    fast_path: list[PlannedCall]
    hints: list[PlannedCall]
    fallback: list[PlannedCall]
    requires_strong_model: bool


class RouteDecision(BaseModel):
    intent: Literal["query", "analysis", "write", "report", "unknown"] = "unknown"
    tool: str = ""
    args: dict[str, Any] = Field(default_factory=dict)
    confidence: float = 0.0
    need_approval: bool = False


class PlanStepDecision(BaseModel):
    tool: str
    args: dict[str, Any] = Field(default_factory=dict)
    purpose: str = ""


class MultiStepPlanDecision(BaseModel):
    objective: str = ""
    steps: list[PlanStepDecision] = Field(default_factory=list)
    confidence: float = 0.0


class HeuristicRouter:
    def analyze(self, message: str, memory: dict[str, Any]) -> RouteAnalysis:
        memory_state = dict(memory.get("memory_state") or {})
        fast_path = self.fast_path(message, memory_state)
        hints = self.hints(message, memory_state)
        fallback = fast_path or hints or [PlannedCall("generate_report", {"report_type": "daily"}, "生成默认日报")]
        return RouteAnalysis(
            fast_path=fast_path,
            hints=hints,
            fallback=fallback,
            requires_strong_model=self.should_analyze_operational_anomaly(message),
        )

    def fast_path(self, message: str, memory_state: dict[str, Any]) -> list[PlannedCall]:
        if self.is_ticket_governance_request(message):
            return self.plan_ticket_governance_request(message, memory_state)
        if self.is_operational_anomaly_request(message):
            return self.plan_operational_anomaly(message, memory_state)
        if self.is_compound_operational_request(message):
            return self.plan_compound_operational_request(message, memory_state)
        if any(token in message.lower() for token in ("日报", "周报", "report")):
            return [PlannedCall("generate_report", {"report_type": "daily"}, "生成运营日报")]
        if "超" in message and "SLA" in message.upper():
            return [self.parse_sla_query(message, memory_state)]

        ticket_no = self.extract_ticket_no(message, memory_state)
        if ticket_no and ("详情" in message or "操作记录" in message):
            return self.plan_ticket_lookup(message, memory_state)
        if "发布" in message and not self.should_analyze_operational_anomaly(message):
            return [PlannedCall("get_recent_releases", {}, "查看近期发布记录")]
        if "退款率" in message and "异常" in message and not self.should_analyze_operational_anomaly(message):
            return [self.parse_refund_anomaly(message, memory_state)]
        if "退款率" in message and not self.should_analyze_operational_anomaly(message):
            return [self.parse_refund_metric(message, memory_state)]
        return []

    def hints(self, message: str, memory_state: dict[str, Any]) -> list[PlannedCall]:
        direct = self.fast_path(message, memory_state)
        if direct:
            return direct
        if self.should_analyze_operational_anomaly(message):
            return self.plan_operational_anomaly(message, memory_state)
        if "退款率" in message and "异常" in message:
            return [self.parse_refund_anomaly(message, memory_state)]
        if "退款率" in message:
            return [self.parse_refund_metric(message, memory_state)]
        return []

    def is_ticket_governance_request(self, message: str) -> bool:
        return any(token in message for token in ("分派给", "升级", "备注"))

    def is_operational_anomaly_request(self, message: str) -> bool:
        return self.should_analyze_operational_anomaly(message)

    def is_compound_operational_request(self, message: str) -> bool:
        upper = message.upper()
        dimensions = 0
        if "退款率" in message:
            dimensions += 1
        if "SLA" in upper or "工单" in message:
            dimensions += 1
        if "发布" in message:
            dimensions += 1
        return dimensions >= 2 and any(token in message for token in ("并", "同时", "结合", "一起"))

    def plan_ticket_lookup(self, message: str, memory_state: dict[str, Any]) -> list[PlannedCall]:
        ticket_no = self.extract_ticket_no(message, memory_state)
        calls = [PlannedCall("get_ticket_detail", {"ticket_no": ticket_no}, "查询工单详情")]
        if "操作记录" in message or "备注" in message:
            calls.append(PlannedCall("get_ticket_comments", {"ticket_no": ticket_no}, "查询最近操作与备注"))
        return calls

    def plan_ticket_governance_request(self, message: str, memory_state: dict[str, Any]) -> list[PlannedCall]:
        calls: list[PlannedCall] = []
        if any(token in message for token in ("详情", "确认", "看下", "查看", "先看")):
            calls.extend(self.plan_ticket_lookup(message, memory_state))
        ticket_no = self.extract_ticket_no(message, memory_state)
        if ticket_no and not any(item.tool_name == "get_ticket_detail" for item in calls):
            calls.append(PlannedCall("get_ticket_detail", {"ticket_no": ticket_no}, "在写前确认工单上下文"))
        if "分派给" in message:
            calls.append(self.parse_assign(message, memory_state))
        if "备注" in message and ("补充" in message or "添加" in message or "备注" in message):
            calls.append(self.parse_add_comment(message, memory_state))
        if "升级" in message and re.search(r"\bP[123]\b", message):
            calls.append(self.parse_escalate(message, memory_state))
        return self.deduplicate_calls(calls)

    def plan_operational_anomaly(self, message: str, memory_state: dict[str, Any]) -> list[PlannedCall]:
        start_date, end_date = self.extract_date_range(message, memory_state)
        calls = [
            PlannedCall(
                "find_refund_anomalies",
                {
                    "start_date": start_date,
                    "end_date": end_date,
                    "region": self.extract_region(message, memory_state),
                    "top_k": self.extract_top_k(message, 5),
                },
                "定位退款率异常类目",
            ),
            PlannedCall(
                "list_sla_breached_tickets",
                {
                    "date": self.extract_single_date(message, memory_state),
                    "region": self.extract_region(message, memory_state),
                    "group_by": "root_cause",
                },
                "查看超 SLA 工单根因分布",
            ),
        ]
        if "类目" in message or "分类" in message:
            calls.append(
                PlannedCall(
                    "list_sla_breached_tickets",
                    {
                        "date": self.extract_single_date(message, memory_state),
                        "region": self.extract_region(message, memory_state),
                        "group_by": "category",
                    },
                    "查看超 SLA 工单类目分布",
                )
            )
        calls.append(PlannedCall("get_recent_releases", {}, "查看异常窗口附近发布记录"))
        if "日报" in message:
            calls.append(PlannedCall("generate_report", {"report_type": "daily"}, "补充日报摘要"))
        return self.deduplicate_calls(calls)

    def plan_compound_operational_request(self, message: str, memory_state: dict[str, Any]) -> list[PlannedCall]:
        calls: list[PlannedCall] = []
        if "退款率" in message:
            calls.append(self.parse_refund_metric(message, memory_state))
        if "SLA" in message.upper() or "工单" in message:
            calls.append(self.parse_sla_query(message, memory_state))
        if "发布" in message:
            calls.append(PlannedCall("get_recent_releases", {}, "查询近期发布记录"))
        if "日报" in message:
            calls.append(PlannedCall("generate_report", {"report_type": "daily"}, "生成日报摘要"))
        return self.deduplicate_calls(calls)

    def should_analyze_operational_anomaly(self, message: str) -> bool:
        upper = message.upper()
        return (
            "归因" in message
            or ("退款率" in message and "超" in message and "SLA" in upper)
            or ("退款率" in message and "发布" in message and "SLA" in upper)
        )

    def parse_refund_anomaly(self, message: str, memory_state: dict[str, Any]) -> PlannedCall:
        start_date, end_date = self.extract_date_range(message, memory_state)
        return PlannedCall(
            "find_refund_anomalies",
            {
                "start_date": start_date,
                "end_date": end_date,
                "region": self.extract_region(message, memory_state),
                "top_k": self.extract_top_k(message, 5),
            },
            "查找退款率异常类目",
        )

    def parse_refund_metric(self, message: str, memory_state: dict[str, Any]) -> PlannedCall:
        start_date, end_date = self.extract_date_range(message, memory_state)
        return PlannedCall(
            "query_refund_metrics",
            {
                "start_date": start_date,
                "end_date": end_date,
                "region": self.extract_region(message, memory_state),
                "category": self.extract_category(message, memory_state),
            },
            "查询退款率指标",
        )

    def parse_sla_query(self, message: str, memory_state: dict[str, Any]) -> PlannedCall:
        group_by = ""
        if "原因" in message:
            group_by = "root_cause"
        elif "优先级" in message:
            group_by = "priority"
        elif "类目" in message or "分类" in message:
            group_by = "category"
        return PlannedCall(
            "list_sla_breached_tickets",
            {
                "date": self.extract_single_date(message, memory_state),
                "region": self.extract_region(message, memory_state),
                "group_by": group_by,
            },
            "查询超 SLA 工单",
        )

    def parse_assign(self, message: str, memory_state: dict[str, Any]) -> PlannedCall:
        ticket_no = self.extract_ticket_no(message, memory_state)
        match = re.search(r"分派给([\u4e00-\u9fa5A-Za-z0-9_]+)", message)
        assignee_name = match.group(1) if match else "待确认"
        return PlannedCall(
            "propose_assign_ticket",
            {
                "ticket_no": ticket_no,
                "assignee_name": assignee_name,
                "reason": f"根据用户指令将{ticket_no}分派给{assignee_name}",
            },
            "生成工单分派 proposal",
        )

    def parse_add_comment(self, message: str, memory_state: dict[str, Any]) -> PlannedCall:
        ticket_no = self.extract_ticket_no(message, memory_state)
        comment_text = ""
        match = re.search(r"备注[:：]?\s*(.+)$", message)
        if match:
            comment_text = match.group(1).strip()
        elif "备注" in message:
            comment_text = message.split("备注", 1)[1].strip(" ：:")
        return PlannedCall(
            "propose_add_ticket_comment",
            {
                "ticket_no": ticket_no,
                "comment_text": comment_text,
                "reason": f"根据用户指令为{ticket_no}增加备注",
            },
            "生成工单备注 proposal",
        )

    def parse_escalate(self, message: str, memory_state: dict[str, Any]) -> PlannedCall:
        ticket_no = self.extract_ticket_no(message, memory_state)
        match = re.search(r"\b(P[123])\b", message)
        priority = match.group(1) if match else ""
        return PlannedCall(
            "propose_escalate_ticket",
            {
                "ticket_no": ticket_no,
                "new_priority": priority,
                "reason": f"根据用户指令将{ticket_no}升级为{priority}",
            },
            "生成工单升级 proposal",
        )

    def extract_ticket_no(self, message: str, memory_state: dict[str, Any]) -> str:
        match = re.search(r"T\d{6,}", message)
        if match:
            return match.group(0)
        if self.is_followup_reference(message):
            return str(memory_state.get("last_ticket_no") or "")
        return ""

    def extract_region(self, message: str, memory_state: dict[str, Any]) -> str | None:
        for region in REGIONS:
            if region in message:
                return region
        if self.is_followup_reference(message):
            value = str(memory_state.get("last_region") or "").strip()
            return value or None
        return None

    def extract_category(self, message: str, memory_state: dict[str, Any]) -> str | None:
        for category in CATEGORIES:
            if category in message:
                return category
        if self.is_followup_reference(message):
            value = str(memory_state.get("last_category") or "").strip()
            return value or None
        return None

    def extract_top_k(self, message: str, default_value: int) -> int:
        match = re.search(r"(\d+)\s*个", message)
        if not match:
            return default_value
        try:
            return int(match.group(1))
        except ValueError:
            return default_value

    def extract_single_date(self, message: str, memory_state: dict[str, Any]) -> str:
        explicit = re.search(r"\d{4}-\d{2}-\d{2}", message)
        if explicit:
            return explicit.group(0)
        today = date.today()
        if "前天" in message:
            return (today - timedelta(days=2)).isoformat()
        if "昨天" in message:
            return (today - timedelta(days=1)).isoformat()
        if "今天" in message:
            return today.isoformat()
        if self.is_followup_reference(message):
            last_date = str(memory_state.get("last_date") or "").strip()
            if last_date:
                return last_date
        return today.isoformat()

    def extract_date_range(self, message: str, memory_state: dict[str, Any]) -> tuple[str, str]:
        matches = re.findall(r"\d{4}-\d{2}-\d{2}", message)
        if len(matches) >= 2:
            return matches[0], matches[1]
        if len(matches) == 1:
            return matches[0], matches[0]
        today = date.today()
        if any(token in message for token in ("最近7天", "近7天", "过去7天")):
            return (today - timedelta(days=6)).isoformat(), today.isoformat()
        if any(token in message for token in ("最近3天", "近3天", "过去3天")):
            return (today - timedelta(days=2)).isoformat(), today.isoformat()
        if any(token in message for token in ("最近14天", "近14天", "过去14天")):
            return (today - timedelta(days=13)).isoformat(), today.isoformat()
        if "昨天" in message:
            yesterday = (today - timedelta(days=1)).isoformat()
            return yesterday, yesterday
        if self.is_followup_reference(message):
            last_range = memory_state.get("last_date_range")
            if isinstance(last_range, dict):
                start_date = str(last_range.get("start_date") or "").strip()
                end_date = str(last_range.get("end_date") or "").strip()
                if start_date and end_date:
                    return start_date, end_date
        return (today - timedelta(days=6)).isoformat(), today.isoformat()

    def is_followup_reference(self, message: str) -> bool:
        return any(token in message for token in ("这个", "那个", "刚才", "刚刚", "继续", "它", "该工单"))

    def deduplicate_calls(self, calls: list[PlannedCall]) -> list[PlannedCall]:
        result: list[PlannedCall] = []
        seen: set[tuple[str, str]] = set()
        for item in calls:
            key = (item.tool_name, json.dumps(item.arguments, sort_keys=True, ensure_ascii=False))
            if key in seen:
                continue
            seen.add(key)
            result.append(item)
        return result


def _tool_summary(name: str) -> str:
    required = TOOL_DEFINITIONS[name]["required"]
    return f"{name}({','.join(required)})" if required else f"{name}()"


def _sanitize_args(tool_name: str, args: dict[str, Any], hints: list[PlannedCall], user_message: str) -> dict[str, Any]:
    result = dict(args or {})
    for hint in hints:
        if hint.tool_name != tool_name:
            continue
        for key, value in hint.arguments.items():
            if key not in result or result[key] in ("", None):
                result[key] = value
        break
    if TOOL_DEFINITIONS[tool_name]["type"] == "write" and not str(result.get("reason") or "").strip():
        result["reason"] = f"根据用户请求生成 proposal: {user_message.strip()[:80]}"
    return result


def _has_required_args(tool_name: str, args: dict[str, Any]) -> bool:
    for key in TOOL_DEFINITIONS[tool_name]["required"]:
        value = args.get(key)
        if value is None:
            return False
        if isinstance(value, str) and not value.strip():
            return False
    return True


def _candidate_tools(include_report: bool) -> list[str]:
    names = [name for name in TOOL_DEFINITIONS.keys() if name != "generate_report"]
    if include_report:
        names.append("generate_report")
    return names


def _parse_llm_steps(steps: list[PlanStepDecision], hints: list[PlannedCall], user_message: str) -> list[PlannedCall]:
    result: list[PlannedCall] = []
    for step in steps[:4]:
        tool_name = step.tool.strip()
        if tool_name not in TOOL_DEFINITIONS:
            continue
        args = _sanitize_args(tool_name, step.args, hints, user_message)
        if not _has_required_args(tool_name, args):
            continue
        result.append(PlannedCall(tool_name, args, step.purpose.strip()))
    return result


def llm_plan(
    *,
    config: RuntimeConfig,
    message: str,
    memory: dict[str, Any],
    hints: list[PlannedCall],
    requires_strong_model: bool,
) -> tuple[list[PlannedCall], str]:
    if not config.has_llm:
        return [], ""

    include_report = any(token in message.lower() for token in ("日报", "周报", "report"))
    candidate_tools = _candidate_tools(include_report)
    catalog = "\n".join(f"- {_tool_summary(name)}" for name in candidate_tools)
    hints_payload = [item.as_dict() for item in hints]
    memory_payload = {
        "summary": memory.get("summary", ""),
        "memory_state": memory.get("memory_state", {}),
        "recent_turns": memory.get("messages", [])[-2:],
        "heuristic_hints": hints_payload,
    }
    prompt = "\n".join(
        [
            "You are the planner for an enterprise operations copilot.",
            "Return a short ordered execution plan as structured output.",
            "You may output 1 to 4 steps.",
            "Prefer reusing heuristic hints when they already fit the latest request.",
            "Use write proposal tools only for write intents. Never execute business writes directly.",
            "If the request needs evidence from multiple data sources, decompose it into multiple steps.",
            "Candidate tools:",
            catalog,
            "",
            "Plan this request:",
            json.dumps({"request": message, "memory": memory_payload}, ensure_ascii=False),
        ]
    )
    models = [config.planner_primary_model]
    if requires_strong_model and config.planner_fallback_model != config.planner_primary_model:
        models = [config.planner_fallback_model]
    elif config.planner_fallback_model != config.planner_primary_model:
        models.append(config.planner_fallback_model)

    for index, model_name in enumerate(models):
        model = ChatOpenAI(
            model=model_name,
            base_url=config.llm_base_url,
            api_key=config.llm_api_key or "ollama-local",
            temperature=0,
        )
        try:
            decision = model.with_structured_output(MultiStepPlanDecision).invoke(prompt)
        except Exception:
            continue
        steps = _parse_llm_steps(decision.steps, hints, message)
        if not steps:
            continue
        if float(decision.confidence or 0.0) < config.router_confidence_cutoff and index < len(models) - 1:
            continue
        return steps, f"langgraph_llm_planner:{model_name}"
    return [], ""


def widen_date_range(arguments: dict[str, Any], extra_days: int = 7) -> dict[str, Any]:
    result = dict(arguments)
    start_date = str(result.get("start_date") or "").strip()
    end_date = str(result.get("end_date") or "").strip()
    if not start_date or not end_date:
        return result
    try:
        start = datetime.strptime(start_date, "%Y-%m-%d").date()
        end = datetime.strptime(end_date, "%Y-%m-%d").date()
    except ValueError:
        return result
    widened_start = start - timedelta(days=extra_days)
    if widened_start > end:
        widened_start = end
    result["start_date"] = widened_start.isoformat()
    result["end_date"] = end.isoformat()
    return result
