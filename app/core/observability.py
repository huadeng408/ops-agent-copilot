from __future__ import annotations

from collections.abc import Iterable

from fastapi import FastAPI
from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.instrumentation.sqlalchemy import SQLAlchemyInstrumentor
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from prometheus_client import CONTENT_TYPE_LATEST, Counter, Histogram, generate_latest

from app.core.config import get_settings
from app.db.session import get_engine


class MetricsRecorder:
    def __init__(self) -> None:
        self.chat_requests_total = Counter(
            'ops_agent_chat_requests_total',
            'Total chat requests grouped by final status.',
            ['status'],
        )
        self.chat_latency_ms = Histogram(
            'ops_agent_chat_latency_ms',
            'Chat request latency in milliseconds.',
            buckets=(10, 25, 50, 100, 200, 400, 800, 1200, 2500, 5000, 10000),
        )
        self.tool_calls_total = Counter(
            'ops_agent_tool_calls_total',
            'Total tool calls grouped by tool name and success flag.',
            ['tool_name', 'success'],
        )
        self.tool_latency_ms = Histogram(
            'ops_agent_tool_latency_ms',
            'Tool call latency in milliseconds.',
            ['tool_name'],
            buckets=(1, 5, 10, 20, 50, 100, 250, 500, 1000, 2500, 5000),
        )
        self.approval_transitions_total = Counter(
            'ops_agent_approval_transitions_total',
            'Approval status transitions.',
            ['from_status', 'to_status'],
        )
        self.approval_turnaround_seconds = Histogram(
            'ops_agent_approval_turnaround_seconds',
            'Seconds from proposal creation to final approval outcome.',
            ['action_type', 'final_status'],
            buckets=(1, 5, 15, 30, 60, 120, 300, 600, 1800, 3600, 7200),
        )
        self.verifier_rejections_total = Counter(
            'ops_agent_verifier_rejections_total',
            'Verifier rejections grouped by stage.',
            ['stage'],
        )
        self.llm_fallback_total = Counter(
            'ops_agent_llm_fallback_total',
            'Number of times the planner fell back away from the primary LLM path.',
            ['reason'],
        )
        self.llm_requests_total = Counter(
            'ops_agent_llm_requests_total',
            'Total outbound LLM requests grouped by endpoint and success flag.',
            ['endpoint', 'success'],
        )

    def record_chat(self, *, status: str, latency_ms: int) -> None:
        self.chat_requests_total.labels(status=status).inc()
        self.chat_latency_ms.observe(latency_ms)

    def record_tool_call(self, *, tool_name: str, success: bool, latency_ms: int) -> None:
        self.tool_calls_total.labels(tool_name=tool_name, success=str(success).lower()).inc()
        self.tool_latency_ms.labels(tool_name=tool_name).observe(latency_ms)

    def record_approval_transition(self, *, from_status: str, to_status: str) -> None:
        self.approval_transitions_total.labels(from_status=from_status, to_status=to_status).inc()

    def record_approval_turnaround(self, *, action_type: str, final_status: str, seconds: float) -> None:
        self.approval_turnaround_seconds.labels(action_type=action_type, final_status=final_status).observe(seconds)

    def record_verifier_rejection(self, *, stage: str) -> None:
        self.verifier_rejections_total.labels(stage=stage).inc()

    def record_llm_fallback(self, *, reason: str) -> None:
        self.llm_fallback_total.labels(reason=reason).inc()

    def record_llm_request(self, *, endpoint: str, success: bool) -> None:
        self.llm_requests_total.labels(endpoint=endpoint, success=str(success).lower()).inc()


metrics = MetricsRecorder()

_telemetry_configured = False
_sqla_instrumented = False
_instrumented_app_ids: set[int] = set()


def get_tracer(name: str = 'ops-agent-copilot'):
    return trace.get_tracer(name)


def render_metrics() -> tuple[bytes, str]:
    return generate_latest(), CONTENT_TYPE_LATEST


def configure_telemetry(app: FastAPI) -> None:
    settings = get_settings()
    if not settings.otel_enabled:
        return

    global _telemetry_configured
    if not _telemetry_configured:
        resource = Resource.create({'service.name': settings.otel_service_name})
        provider = TracerProvider(resource=resource)
        exporter = OTLPSpanExporter(endpoint=_normalize_otlp_traces_endpoint(settings.otel_exporter_otlp_endpoint))
        provider.add_span_processor(BatchSpanProcessor(exporter))
        trace.set_tracer_provider(provider)
        _telemetry_configured = True

    global _sqla_instrumented
    if not _sqla_instrumented:
        SQLAlchemyInstrumentor().instrument(engine=get_engine().sync_engine)
        _sqla_instrumented = True

    app_id = id(app)
    if app_id not in _instrumented_app_ids:
        FastAPIInstrumentor.instrument_app(app)
        _instrumented_app_ids.add(app_id)


def _normalize_otlp_traces_endpoint(endpoint: str) -> str:
    normalized = endpoint.rstrip('/')
    if normalized.endswith('/v1/traces'):
        return normalized
    return f'{normalized}/v1/traces'
