# LLM Relay Setup

This project uses OpenAI-compatible chat and responses APIs. The default configuration now points to a domestic model provider.

## Recommended configuration

```env
OPENAI_BASE_URL=https://api.moonshot.cn/v1
OPENAI_MODEL=kimi-k2-0905-preview
OPENAI_TRUST_ENV=true
OPENAI_AUTH_FILE=auth.json
```

## Key file

Create a root-level `auth.json`:

```json
{
  "OPENAI_API_KEY": "sk-xxx"
}
```

## Loading order

- If `OPENAI_AUTH_FILE` is set, the app reads `OPENAI_API_KEY` from that file first.
- If `OPENAI_AUTH_FILE` is not set, the app tries root `auth.json`.
- If no file key is found, the app falls back to the `.env` value.

## Proxy note

- If your system proxy interferes with the provider, set `OPENAI_TRUST_ENV=false`.

## Startup check

After restarting the API, verify:

- `GET /healthz`
- `POST /api/v1/chat`

If the key and model are valid, `/api/v1/chat` will enter the planner path first and then tool execution or approval flow.
