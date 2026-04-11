# LLM Provider Setup

This project now supports only two LLM profiles:

- remote `kimi-*` models through Moonshot's compatible endpoint
- local Ollama `gemma4*` models through `http://127.0.0.1:11434/v1`

The project still accepts legacy `OPENAI_*` env vars as a compatibility fallback, but new configuration should use `LLM_*`.

## Remote Kimi profile

- Base URL: `https://api.moonshot.cn/v1`
- Model: `kimi-k2-0905-preview`
- API key source: `.env` or `auth.json`

Root `.env`:

```env
LLM_PROVIDER=kimi
LLM_BASE_URL=https://api.moonshot.cn/v1
LLM_API_KEY=sk-xxx
LLM_MODEL=kimi-k2-0905-preview
LLM_AUTH_FILE=auth.json
```

Root `auth.json`:

```json
{
  "LLM_API_KEY": "sk-xxx"
}
```

Priority order:

- If `LLM_AUTH_FILE` is set, the app loads `LLM_API_KEY` from that file first.
- The auth file reader also accepts legacy `OPENAI_API_KEY` for compatibility.
- If `LLM_AUTH_FILE` is not set, the app tries the root `auth.json` for the Kimi profile.
- If no file key is found, it falls back to `.env`.

## Local Ollama + Gemma4 profile

```env
LLM_PROVIDER=ollama
LLM_BASE_URL=http://127.0.0.1:11434/v1
LLM_API_KEY=ollama-local
LLM_MODEL=gemma4:e4b
LLM_AUTH_FILE=
```

Notes:

- `LLM_PROVIDER=ollama` only accepts local loopback endpoints and `gemma4*` models.
- `LLM_AUTH_FILE` is intentionally empty for Ollama so the app does not load remote Kimi credentials by accident.
- `AGENT_RUNTIME_MODE=llm` forces the planner to use the configured LLM profile.
- `AGENT_RUNTIME_MODE=auto` tries the configured LLM profile first and falls back to the heuristic router on planner failure.
