from __future__ import annotations

import hashlib
import json
import time
from typing import Any, TypedDict

from langgraph.graph import END, START, StateGraph

from .backend import BackendClient, BackendServiceError
from .config import RuntimeConfig
from .router import HeuristicRouter, PlannedCall, llm_plan
from .verification import build_answer_reflection, build_step_reflection, synthesize_answer, verify_answer, verify_step_result


class GraphState(TypedDict, total=False):
    trace_id: str
    session_id: str
    user_id: int
    message: str
    memory: dict[str, Any]
    planned_calls: list[dict[str, Any]]
    planning_source: str
    planner_latency_ms: int
    plan_cache_hit: bool
    tool_calls: list[dict[str, Any]]
    step_results: list[dict[str, Any]]
    current_step_index: int
    reflection_count: int
    answer_reflection_count: int
    last_step_verification: dict[str, Any]
    answer_verification: dict[str, Any]
    answer: str
    status: str
    approval: dict[str, Any] | None


class PlannerCache:
    def __init__(self, max_items: int = 256) -> None:
        self._max_items = max_items
        self._items: dict[str, list[dict[str, Any]]] = {}

    def get(self, key: str) -> list[dict[str, Any]] | None:
        return self._items.get(key)

    def set(self, key: str, value: list[dict[str, Any]]) -> None:
        if len(self._items) >= self._max_items:
            oldest_key = next(iter(self._items))
            self._items.pop(oldest_key, None)
        self._items[key] = value


def _hash_plan_key(message: str, memory: dict[str, Any], hints: list[PlannedCall]) -> str:
    payload = {
        "message": message,
        "memory_state": memory.get("memory_state", {}),
        "summary": memory.get("summary", ""),
        "recent_turns": memory.get("messages", [])[-2:],
        "hints": [item.as_dict() for item in hints],
    }
    return hashlib.sha256(json.dumps(payload, sort_keys=True, ensure_ascii=False).encode("utf-8")).hexdigest()


def build_graph(config: RuntimeConfig):
    router = HeuristicRouter()
    backend = BackendClient(config)
    planner_cache = PlannerCache()

    def plan_request(state: GraphState) -> GraphState:
        started = time.perf_counter()
        message = state["message"]
        memory = state.get("memory", {})
        analysis = router.analyze(message, memory)
        fast_path = [item.as_dict() for item in analysis.fast_path]
        if fast_path:
            return {
                "planned_calls": fast_path,
                "planning_source": "langgraph_step_planner:rule",
                "planner_latency_ms": int((time.perf_counter() - started) * 1000),
                "plan_cache_hit": False,
                "tool_calls": [],
                "step_results": [],
                "current_step_index": 0,
                "reflection_count": 0,
                "answer_reflection_count": 0,
                "status": "completed",
                "approval": None,
            }

        cache_key = _hash_plan_key(message, memory, analysis.hints)
        cached = planner_cache.get(cache_key)
        if cached:
            return {
                "planned_calls": cached,
                "planning_source": "langgraph_step_planner:cache",
                "planner_latency_ms": int((time.perf_counter() - started) * 1000),
                "plan_cache_hit": True,
                "tool_calls": [],
                "step_results": [],
                "current_step_index": 0,
                "reflection_count": 0,
                "answer_reflection_count": 0,
                "status": "completed",
                "approval": None,
            }

        llm_calls, source = llm_plan(
            config=config,
            message=message,
            memory=memory,
            hints=analysis.hints,
            requires_strong_model=analysis.requires_strong_model,
        )
        if llm_calls:
            rendered = [item.as_dict() for item in llm_calls]
            planner_cache.set(cache_key, rendered)
            return {
                "planned_calls": rendered,
                "planning_source": source,
                "planner_latency_ms": int((time.perf_counter() - started) * 1000),
                "plan_cache_hit": False,
                "tool_calls": [],
                "step_results": [],
                "current_step_index": 0,
                "reflection_count": 0,
                "answer_reflection_count": 0,
                "status": "completed",
                "approval": None,
            }

        fallback = [item.as_dict() for item in analysis.fallback]
        planner_cache.set(cache_key, fallback)
        return {
            "planned_calls": fallback,
            "planning_source": "langgraph_step_planner:heuristic",
            "planner_latency_ms": int((time.perf_counter() - started) * 1000),
            "plan_cache_hit": False,
            "tool_calls": [],
            "step_results": [],
            "current_step_index": 0,
            "reflection_count": 0,
            "answer_reflection_count": 0,
            "status": "completed",
            "approval": None,
        }

    def route_after_plan(state: GraphState) -> str:
        if not state.get("planned_calls"):
            return "synthesize_answer"
        return "execute_step"

    def execute_step(state: GraphState) -> GraphState:
        planned_calls = list(state.get("planned_calls") or [])
        current_step_index = int(state.get("current_step_index") or 0)
        if current_step_index >= len(planned_calls):
            return {}

        step = dict(planned_calls[current_step_index])
        step_results = list(state.get("step_results") or [])
        tool_calls = list(state.get("tool_calls") or [])
        approval = state.get("approval")
        status = state.get("status") or "completed"

        if step["tool_name"] == "generate_report":
            try:
                report = backend.generate_daily_report()
                step_result = {
                    "tool_name": "generate_report",
                    "arguments": dict(step.get("arguments") or {}),
                    "success": True,
                    "data": report,
                    "rendered_answer": str(report.get("content") or "已生成日报。"),
                    "requires_approval": False,
                    "message": "已生成运营日报",
                }
                tool_calls.append({"tool_name": "generate_report", "success": True, "tool_type": "report", "latency_ms": 0})
            except BackendServiceError as exc:
                step_result = {
                    "tool_name": "generate_report",
                    "arguments": dict(step.get("arguments") or {}),
                    "success": False,
                    "data": {},
                    "rendered_answer": "",
                    "requires_approval": False,
                    "message": exc.detail,
                    "error": exc.detail,
                }
                tool_calls.append({"tool_name": "generate_report", "success": False, "tool_type": "report", "latency_ms": 0})
        else:
            try:
                tool_result = backend.invoke_tool(
                    trace_id=state["trace_id"],
                    session_id=state["session_id"],
                    user_id=state["user_id"],
                    tool_name=step["tool_name"],
                    arguments=dict(step.get("arguments") or {}),
                )
                step_result = {
                    "tool_name": step["tool_name"],
                    "arguments": dict(step.get("arguments") or {}),
                    "success": bool(tool_result.get("success", True)),
                    "data": dict(tool_result.get("data") or {}),
                    "rendered_answer": str(tool_result.get("rendered_answer") or tool_result.get("message") or "").strip(),
                    "requires_approval": bool(tool_result.get("requires_approval")),
                    "message": str(tool_result.get("message") or ""),
                    "action_type": str(tool_result.get("action_type") or ""),
                    "target_type": str(tool_result.get("target_type") or ""),
                    "target_id": str(tool_result.get("target_id") or ""),
                    "proposal_payload": dict(tool_result.get("proposal_payload") or {}),
                    "proposal_reason": str(tool_result.get("proposal_reason") or ""),
                }
                tool_calls.append(
                    {
                        "tool_name": tool_result.get("tool_name", step["tool_name"]),
                        "success": bool(tool_result.get("success", True)),
                        "tool_type": tool_result.get("tool_type", ""),
                        "latency_ms": int(tool_result.get("latency_ms", 0)),
                    }
                )
                if step_result["requires_approval"]:
                    proposal = backend.create_proposal(
                        trace_id=state["trace_id"],
                        session_id=state["session_id"],
                        user_id=state["user_id"],
                        action_type=step_result["action_type"],
                        target_type=step_result["target_type"],
                        target_id=step_result["target_id"],
                        payload=step_result["proposal_payload"],
                        reason=step_result["proposal_reason"],
                    )
                    approval = {
                        "approval_no": proposal.get("approval_no"),
                        "action_type": proposal.get("action_type"),
                        "target_id": proposal.get("target_id"),
                        "payload": proposal.get("payload") or {},
                    }
                    status = "approval_required"
            except BackendServiceError as exc:
                step_result = {
                    "tool_name": step["tool_name"],
                    "arguments": dict(step.get("arguments") or {}),
                    "success": False,
                    "data": {},
                    "rendered_answer": "",
                    "requires_approval": False,
                    "message": exc.detail,
                    "error": exc.detail,
                }
                tool_calls.append({"tool_name": step["tool_name"], "success": False, "tool_type": "readonly", "latency_ms": 0})

        step_results.append(step_result)
        return {
            "tool_calls": tool_calls,
            "step_results": step_results,
            "approval": approval,
            "status": status,
        }

    def route_after_execute(state: GraphState) -> str:
        if state.get("status") == "approval_required":
            return "synthesize_answer"
        return "verify_step"

    def verify_step(state: GraphState) -> GraphState:
        step_results = list(state.get("step_results") or [])
        if not step_results:
            return {"last_step_verification": {"passed": True, "reason": ""}}
        current_step_index = int(state.get("current_step_index") or 0)
        planned_calls = list(state.get("planned_calls") or [])
        current_step = dict(planned_calls[current_step_index])
        latest = dict(step_results[-1])

        if not latest.get("success", True):
            return {"last_step_verification": {"passed": False, "reason": str(latest.get("message") or "tool execution failed")}}

        verification = verify_step_result(state["message"], current_step, latest)
        if verification.get("passed"):
            return {
                "last_step_verification": verification,
                "current_step_index": current_step_index + 1,
            }
        return {"last_step_verification": verification}

    def route_after_step_verification(state: GraphState) -> str:
        verification = dict(state.get("last_step_verification") or {})
        current_step_index = int(state.get("current_step_index") or 0)
        planned_calls = list(state.get("planned_calls") or [])
        if verification.get("passed"):
            if current_step_index >= len(planned_calls):
                return "synthesize_answer"
            return "execute_step"
        if int(state.get("reflection_count") or 0) < config.max_step_reflections:
            latest = list(state.get("step_results") or [])[-1] if state.get("step_results") else {}
            reflection = build_step_reflection(state["message"], planned_calls[current_step_index], latest)
            if reflection is not None:
                return "reflect_step"
        return "synthesize_answer"

    def reflect_step(state: GraphState) -> GraphState:
        current_step_index = int(state.get("current_step_index") or 0)
        planned_calls = list(state.get("planned_calls") or [])
        latest = list(state.get("step_results") or [])[-1]
        reflection = build_step_reflection(state["message"], planned_calls[current_step_index], latest)
        if reflection is None:
            return {}
        planned_calls[current_step_index] = reflection.as_dict()
        return {
            "planned_calls": planned_calls,
            "reflection_count": int(state.get("reflection_count") or 0) + 1,
        }

    def synthesize_answer_node(state: GraphState) -> GraphState:
        answer = synthesize_answer(
            state["message"],
            list(state.get("step_results") or []),
            str(state.get("status") or "completed"),
            state.get("approval"),
        )
        return {"answer": answer}

    def verify_answer_node(state: GraphState) -> GraphState:
        verification = verify_answer(
            state["message"],
            str(state.get("answer") or ""),
            list(state.get("step_results") or []),
            state.get("approval"),
        )
        return {"answer_verification": verification}

    def route_after_answer_verification(state: GraphState) -> str:
        verification = dict(state.get("answer_verification") or {})
        if verification.get("passed"):
            return "finalize"
        if int(state.get("answer_reflection_count") or 0) >= config.max_answer_reflections:
            return "finalize"
        if state.get("status") == "approval_required":
            return "finalize"
        additions = build_answer_reflection(
            state["message"],
            list(state.get("planned_calls") or []),
            list(state.get("step_results") or []),
        )
        if additions:
            return "reflect_answer"
        return "finalize"

    def reflect_answer(state: GraphState) -> GraphState:
        additions = build_answer_reflection(
            state["message"],
            list(state.get("planned_calls") or []),
            list(state.get("step_results") or []),
        )
        if not additions:
            return {}
        planned_calls = list(state.get("planned_calls") or [])
        planned_calls.extend(item.as_dict() for item in additions)
        return {
            "planned_calls": planned_calls,
            "answer_reflection_count": int(state.get("answer_reflection_count") or 0) + 1,
        }

    def finalize(state: GraphState) -> GraphState:
        answer = str(state.get("answer") or "").strip() or "已处理请求。"
        return {
            "answer": answer,
            "status": str(state.get("status") or "completed"),
        }

    graph = StateGraph(GraphState)
    graph.add_node("plan_request", plan_request)
    graph.add_node("execute_step", execute_step)
    graph.add_node("verify_step", verify_step)
    graph.add_node("reflect_step", reflect_step)
    graph.add_node("synthesize_answer", synthesize_answer_node)
    graph.add_node("verify_answer", verify_answer_node)
    graph.add_node("reflect_answer", reflect_answer)
    graph.add_node("finalize", finalize)

    graph.add_edge(START, "plan_request")
    graph.add_conditional_edges("plan_request", route_after_plan, {"execute_step": "execute_step", "synthesize_answer": "synthesize_answer"})
    graph.add_conditional_edges("execute_step", route_after_execute, {"verify_step": "verify_step", "synthesize_answer": "synthesize_answer"})
    graph.add_conditional_edges(
        "verify_step",
        route_after_step_verification,
        {
            "execute_step": "execute_step",
            "reflect_step": "reflect_step",
            "synthesize_answer": "synthesize_answer",
        },
    )
    graph.add_edge("reflect_step", "execute_step")
    graph.add_edge("synthesize_answer", "verify_answer")
    graph.add_conditional_edges(
        "verify_answer",
        route_after_answer_verification,
        {
            "reflect_answer": "reflect_answer",
            "finalize": "finalize",
        },
    )
    graph.add_edge("reflect_answer", "execute_step")
    graph.add_edge("finalize", END)
    return graph.compile()
