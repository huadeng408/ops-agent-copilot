# Chat Load Test Report (2026-03-31)

## Test scope

- Target endpoint: `POST /api/v1/chat`
- Workload mix (round-robin):
  - `最近7天退款率异常的类目有哪些？`
  - `北京区昨天超SLA的工单按原因分类`
  - `查一下 T202603280012 的详情`
  - `生成一份今天的运营日报`
- Runtime mode: `AGENT_RUNTIME_MODE=heuristic`
- Storage: `sqlite+aiosqlite` (local file)
- Telemetry export: disabled (`OTEL_ENABLED=false`)
- Host: `127.0.0.1:18004`

## Environment

- OS: Windows 11 64-bit (`10.0.26200`)
- CPU: Intel i9-13900HX (`24 cores / 32 threads`)
- Memory: 16 GB
- Python: 3.12

## Raw results

### 100 RPS, 30s

Source: `eval/benchmarks/20260331/chat_load_100rps_30s.json`

- Requested RPS: `100`
- Achieved RPS: `74.37`
- Total requests: `3000`
- Success rate: `99.30%` (`2979/3000`)
- Error rate: `0.70%` (`21/3000`)
- Latency: `p50=1120ms`, `p95=2272ms`, `p99=5604ms`

### 200 RPS, 30s

Source: `eval/benchmarks/20260331/chat_load_200rps_30s.json`

- Requested RPS: `200`
- Achieved RPS: `80.64`
- Total requests: `6000`
- Success rate: `99.42%` (`5965/6000`)
- Error rate: `0.58%` (`35/6000`)
- Latency: `p50=2260ms`, `p95=3364ms`, `p99=6728ms`

## Observed bottleneck signal

- Throughput increases from `74.37` to `80.64` when load target rises from `100` to `200` RPS, while latency rises sharply.
- This indicates the current single-instance setup is approaching saturation and queueing under bursty chat traffic.

## Resume-safe phrasing

- Completed real `/api/v1/chat` load tests for the heuristic / deterministic path on a single instance and observed `74.37/80.64` achieved RPS under `100/200` target RPS workloads with `99.3%+` success rate.
- Quantified latency under pressure (`100 RPS: p95 2.27s`, `200 RPS: p95 3.36s`) and identified single-instance saturation and queueing behavior for follow-up scaling work.
- Built reproducible benchmark artifacts (`scripts/run_load_test.py`, JSON reports) and separated deterministic-path baselines from remote-LLM planner bottleneck analysis.

## Repro commands

```powershell
python -m scripts.run_load_test --base-url http://127.0.0.1:18004 --rps 100 --duration 30 --concurrency 100
python -m scripts.run_load_test --base-url http://127.0.0.1:18004 --rps 200 --duration 30 --concurrency 200
```
