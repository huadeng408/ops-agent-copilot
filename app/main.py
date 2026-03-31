from contextlib import asynccontextmanager

from fastapi import FastAPI

from app.api.admin import router as admin_router
from app.api.approvals import router as approvals_router
from app.api.audit import router as audit_router
from app.api.chat import router as chat_router
from app.api.health import router as health_router
from app.api.metrics import router as metrics_router
from app.api.tickets import router as tickets_router
from app.core.config import get_settings
from app.core.logging import configure_logging
from app.core.observability import configure_telemetry


@asynccontextmanager
async def lifespan(_: FastAPI):
    configure_logging()
    yield


def create_app() -> FastAPI:
    settings = get_settings()
    app = FastAPI(
        title=settings.app_name,
        version="0.1.0",
        docs_url="/docs",
        redoc_url="/redoc",
        lifespan=lifespan,
    )
    configure_telemetry(app)
    app.include_router(health_router)
    app.include_router(metrics_router)
    app.include_router(admin_router)
    app.include_router(chat_router, prefix="/api/v1")
    app.include_router(approvals_router, prefix="/api/v1")
    app.include_router(audit_router, prefix="/api/v1")
    app.include_router(tickets_router, prefix="/api/v1")
    return app


app = create_app()
