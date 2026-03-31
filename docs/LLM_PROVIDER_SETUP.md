# LLM Provider Setup

This project is configured by default to use a domestic OpenAI-compatible model endpoint.

## Default route

- Base URL: `https://api.moonshot.cn/v1`
- Model: `kimi-k2-0905-preview`
- API key source: `.env` or `auth.json`

## Where to fill in the key

1. Root `.env`

```env
OPENAI_BASE_URL=https://api.moonshot.cn/v1
OPENAI_API_KEY=sk-xxx
OPENAI_MODEL=kimi-k2-0905-preview
OPENAI_AUTH_FILE=auth.json
```

2. Root `auth.json`

```json
{
  "OPENAI_API_KEY": "sk-xxx"
}
```

## Priority order

- If `OPENAI_AUTH_FILE` is set, the app loads `OPENAI_API_KEY` from that file first.
- If `OPENAI_AUTH_FILE` is not set, the app tries the root `auth.json`.
- If no file key is found, it falls back to `.env`.

## Notes

- If your proxy interferes with the provider connection, set `OPENAI_TRUST_ENV=false`.
- The code keeps the `OPENAI_*` variable names for compatibility, but the endpoint and model are now set for the domestic route.
