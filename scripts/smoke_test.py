import asyncio

import httpx


async def main() -> None:
    async with httpx.AsyncClient(base_url='http://127.0.0.1:8000', timeout=20) as client:
        health = await client.get('/healthz')
        print('healthz:', health.status_code, health.json())
        chat = await client.post(
            '/api/v1/chat',
            json={'session_id': 'smoke_sess_001', 'user_id': 1, 'message': '北京区昨天超SLA的工单按原因分类'},
        )
        print('chat:', chat.status_code, chat.json())


if __name__ == '__main__':
    asyncio.run(main())
