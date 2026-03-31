import argparse

import uvicorn

from app.core.config import get_settings


def main() -> None:
    parser = argparse.ArgumentParser(description='Run ops-agent-copilot API locally.')
    parser.add_argument('--host', default=None, help='Override host from settings.')
    parser.add_argument('--port', type=int, default=None, help='Override port from settings.')
    parser.add_argument('--reload', action='store_true', help='Enable uvicorn reload.')
    args = parser.parse_args()

    settings = get_settings()
    uvicorn.run(
        'app.main:app',
        host=args.host or settings.host,
        port=args.port or settings.port,
        reload=args.reload,
    )


if __name__ == '__main__':
    main()
