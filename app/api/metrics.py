from fastapi import APIRouter, Response

from app.core.observability import render_metrics


router = APIRouter(tags=['metrics'])


@router.get('/metrics', include_in_schema=False)
async def metrics() -> Response:
    payload, content_type = render_metrics()
    return Response(content=payload, media_type=content_type)
