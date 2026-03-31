# OpenAI GPT-5.4 Real-Agent Benchmark (2026-03-31)

## What was validated

1. Real OpenAI-compatible `gpt-5.4` call (Responses API) succeeded.
2. Real agent process succeeded in `AGENT_RUNTIME_MODE=openai`:
   - Chat request processed.
   - Write action produced `approval_required`.
   - Approval API transitioned to `executed`.
   - Ticket assignee changed successfully.
3. Stress test executed against `/api/v1/chat` under real OpenAI planner mode.

## Runtime setup

- Model endpoint: `OPENAI_BASE_URL=https://codex.ai02.cn/v1`
- Model name: `gpt-5.4`
- Runtime mode: `AGENT_RUNTIME_MODE=openai`
- Service endpoint: `http://127.0.0.1:18005`
- DB: local SQLite (`perf_openai_20260331.sqlite3`)
- Message set: `eval/benchmarks/20260331/openai_load_messages.txt`
- Telemetry:
  - `/metrics` enabled
  - `ops_agent_llm_requests_total{endpoint="/responses",success="true"} = 109` observed during test window

## Load test results

### 100 RPS, 10s

Source: `eval/benchmarks/20260331/openai_gpt54_chat_load_100rps_10s.json`

- Requested RPS: `100`
- Achieved RPS: `4.97`
- Success rate: `0.10%` (`1 / 1000`)
- Error rate: `99.90%`
- Latency: `p50 24.23s`, `p95 29.96s`, `p99 30.59s`

### 200 RPS, 10s

Source: `eval/benchmarks/20260331/openai_gpt54_chat_load_200rps_10s.json`

- Requested RPS: `200`
- Achieved RPS: `8.61`
- Success rate: `0.05%` (`1 / 2000`)
- Error rate: `99.95%`
- Latency: `p50 20.34s`, `p95 40.28s`, `p99 40.71s`

## Engineering interpretation

- The bottleneck is external LLM planning under high-concurrency sync request path.
- Current architecture is suitable for low/medium online QPS with strong governance, but not for direct 100/200 RPS synchronous LLM planning.
- To support high QPS:
  - add planner-result cache and semantic dedup,
  - move heavy planning/report generation to async queue,
  - isolate fast-path deterministic router from LLM path,
  - enforce admission control and circuit breaking around LLM calls.

## Resume-ready data statements

- Executed real-agent benchmark with OpenAI-compatible `gpt-5.4` planner and completed end-to-end governed write flow (`approval_required -> executed`) with persisted state transition and idempotent execution.
- Measured synchronous LLM-planner pressure limits at `100/200 RPS` (`4.97/8.61` achieved RPS, `p95 29.96s/40.28s`), and identified remote planning as the primary throughput bottleneck.
- Established observability evidence via Prometheus metrics (`ops_agent_llm_requests_total`, chat latency histogram, tool latency histogram), enabling capacity diagnosis and optimization planning based on measured data rather than assumptions.
