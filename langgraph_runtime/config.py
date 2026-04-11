from __future__ import annotations

import os
from dataclasses import dataclass


def _getenv(key: str, default: str = "") -> str:
    value = os.getenv(key, "").strip()
    return value or default


def _getenv_int(key: str, default: int) -> int:
    raw = os.getenv(key, "").strip()
    if not raw:
        return default
    try:
        return int(raw)
    except ValueError:
        return default


def _getenv_float(key: str, default: float) -> float:
    raw = os.getenv(key, "").strip()
    if not raw:
        return default
    try:
        return float(raw)
    except ValueError:
        return default


@dataclass(slots=True)
class RuntimeConfig:
    ops_agent_base_url: str
    internal_api_key: str
    llm_base_url: str
    llm_api_key: str
    llm_model: str
    planner_primary_model: str
    planner_fallback_model: str
    router_confidence_cutoff: float
    max_step_reflections: int
    max_answer_reflections: int
    langgraph_host: str
    langgraph_port: int

    @classmethod
    def from_env(cls) -> "RuntimeConfig":
        llm_model = _getenv("LLM_MODEL", "kimi-k2-0905-preview")
        return cls(
            ops_agent_base_url=_getenv("OPS_AGENT_BASE_URL", "http://127.0.0.1:18000"),
            internal_api_key=_getenv("INTERNAL_API_KEY", ""),
            llm_base_url=_getenv("LLM_BASE_URL", "https://api.moonshot.cn/v1"),
            llm_api_key=_getenv("LLM_API_KEY", ""),
            llm_model=llm_model,
            planner_primary_model=_getenv("ROUTER_PRIMARY_MODEL", llm_model),
            planner_fallback_model=_getenv("ROUTER_FALLBACK_MODEL", llm_model),
            router_confidence_cutoff=_getenv_float("ROUTER_CONFIDENCE_CUTOFF", 0.72),
            max_step_reflections=_getenv_int("LANGGRAPH_MAX_STEP_REFLECTIONS", 2),
            max_answer_reflections=_getenv_int("LANGGRAPH_MAX_ANSWER_REFLECTIONS", 1),
            langgraph_host=_getenv("LANGGRAPH_HOST", "127.0.0.1"),
            langgraph_port=_getenv_int("LANGGRAPH_PORT", 8001),
        )

    @property
    def has_llm(self) -> bool:
        return bool(self.llm_base_url and self.llm_model)
