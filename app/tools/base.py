from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Any

from app.db.models import User
from app.repositories.metric_repo import MetricRepository
from app.repositories.release_repo import ReleaseRepository
from app.repositories.ticket_repo import TicketRepository
from app.schemas.tool import ToolSchema
from app.services.anomaly_service import AnomalyService
from app.services.verifier_service import VerifierService


@dataclass(slots=True)
class ToolContext:
    trace_id: str
    session_id: str
    user: User
    metric_repo: MetricRepository
    ticket_repo: TicketRepository
    release_repo: ReleaseRepository
    verifier: VerifierService
    anomaly_service: AnomalyService | None = None


@dataclass(slots=True)
class ToolResult:
    data: dict[str, Any]
    message: str
    requires_approval: bool = False


class BaseTool(ABC):
    tool_type = 'readonly'

    @property
    @abstractmethod
    def schema(self) -> ToolSchema:
        raise NotImplementedError

    @abstractmethod
    async def execute(self, context: ToolContext, arguments: dict[str, Any]) -> ToolResult:
        raise NotImplementedError
