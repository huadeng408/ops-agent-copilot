import argparse
import asyncio
import json
from statistics import mean
import time

import httpx


DEFAULT_MESSAGES = [
    '最近7天退款率异常的类目有哪些？',
    '北京区昨天超SLA的工单按原因分类',
    '查一下 T202603280012 的详情',
    '生成一份今天的运营日报',
]


async def run_load_test(
    *,
    base_url: str,
    rps: int,
    duration_seconds: int,
    concurrency: int,
    user_id: int,
    messages: list[str],
    timeout_seconds: float,
    runtime_mode: str | None,
    llm_percent: int,
) -> dict:
    semaphore = asyncio.Semaphore(concurrency)
    latencies: list[int] = []
    status_codes: list[int] = []
    app_statuses: list[str] = []
    request_runtime_modes: list[str] = []
    planning_sources: list[str] = []
    planner_latencies: list[int] = []
    tool_call_counts: list[int] = []
    started = time.perf_counter()

    async with httpx.AsyncClient(base_url=base_url.rstrip('/'), timeout=timeout_seconds, trust_env=False) as client:
        tasks = []
        total_requests = rps * duration_seconds
        for index in range(total_requests):
            target_offset = index / rps
            tasks.append(
                asyncio.create_task(
                    _one_request(
                        client=client,
                        semaphore=semaphore,
                        index=index,
                        user_id=user_id,
                        messages=messages,
                        target_offset=target_offset,
                        run_started=started,
                        latencies=latencies,
                        status_codes=status_codes,
                        app_statuses=app_statuses,
                        request_runtime_modes=request_runtime_modes,
                        planning_sources=planning_sources,
                        planner_latencies=planner_latencies,
                        tool_call_counts=tool_call_counts,
                        runtime_mode=runtime_mode,
                        llm_percent=llm_percent,
                    )
                )
            )
        await asyncio.gather(*tasks)

    elapsed = max(time.perf_counter() - started, 0.001)
    return _summarize(
        latencies=latencies,
        status_codes=status_codes,
        app_statuses=app_statuses,
        request_runtime_modes=request_runtime_modes,
        planning_sources=planning_sources,
        planner_latencies=planner_latencies,
        tool_call_counts=tool_call_counts,
        requested_rps=rps,
        elapsed_seconds=elapsed,
        timeout_seconds=timeout_seconds,
    )


async def _one_request(
    *,
    client: httpx.AsyncClient,
    semaphore: asyncio.Semaphore,
    index: int,
    user_id: int,
    messages: list[str],
    target_offset: float,
    run_started: float,
    latencies: list[int],
    status_codes: list[int],
    app_statuses: list[str],
    request_runtime_modes: list[str],
    planning_sources: list[str],
    planner_latencies: list[int],
    tool_call_counts: list[int],
    runtime_mode: str | None,
    llm_percent: int,
) -> None:
    delay = target_offset - (time.perf_counter() - run_started)
    if delay > 0:
        await asyncio.sleep(delay)

    message = messages[index % len(messages)]
    payload = {
        'session_id': f'load_{index}',
        'user_id': user_id,
        'message': message,
    }
    effective_runtime_mode = choose_runtime_mode(index=index, runtime_mode=runtime_mode, llm_percent=llm_percent)
    if effective_runtime_mode:
        payload['runtime_mode'] = effective_runtime_mode
        request_runtime_modes.append(effective_runtime_mode)
    else:
        request_runtime_modes.append('default')

    async with semaphore:
        started = time.perf_counter()
        try:
            response = await client.post('/api/v1/chat', json=payload)
            latency_ms = int((time.perf_counter() - started) * 1000)
            latencies.append(latency_ms)
            status_codes.append(response.status_code)
            if 200 <= response.status_code < 300:
                try:
                    body = response.json()
                except Exception:
                    body = {}
                app_statuses.append(str(body.get('status') or 'unknown'))
                planning_sources.append(str(body.get('planning_source') or 'unknown'))
                planner_latency = body.get('planner_latency_ms')
                if isinstance(planner_latency, int):
                    planner_latencies.append(planner_latency)
                tool_calls = body.get('tool_calls')
                if isinstance(tool_calls, list):
                    tool_call_counts.append(len(tool_calls))
                else:
                    tool_call_counts.append(0)
            return
        except Exception:
            latency_ms = int((time.perf_counter() - started) * 1000)
            latencies.append(latency_ms)
            status_codes.append(0)
            app_statuses.append('request_exception')
            planning_sources.append('request_exception')
            tool_call_counts.append(0)


def _summarize(
    *,
    latencies: list[int],
    status_codes: list[int],
    app_statuses: list[str],
    request_runtime_modes: list[str],
    planning_sources: list[str],
    planner_latencies: list[int],
    tool_call_counts: list[int],
    requested_rps: int,
    elapsed_seconds: float,
    timeout_seconds: float,
) -> dict:
    latencies_sorted = sorted(latencies)
    p50_index = max(0, int(len(latencies_sorted) * 0.50) - 1)
    p95_index = max(0, int(len(latencies_sorted) * 0.95) - 1)
    p99_index = max(0, int(len(latencies_sorted) * 0.99) - 1)
    success_count = sum(1 for code in status_codes if 200 <= code < 300)
    error_count = len(status_codes) - success_count
    return {
        'requested_rps': requested_rps,
        'timeout_seconds': timeout_seconds,
        'achieved_rps': round(len(status_codes) / elapsed_seconds, 2),
        'total_requests': len(status_codes),
        'success_count': success_count,
        'error_count': error_count,
        'error_rate': round(error_count / len(status_codes), 4) if status_codes else 0,
        'avg_latency_ms': round(mean(latencies), 2) if latencies else 0,
        'p50_latency_ms': latencies_sorted[p50_index] if latencies_sorted else 0,
        'p95_latency_ms': latencies_sorted[p95_index] if latencies_sorted else 0,
        'p99_latency_ms': latencies_sorted[p99_index] if latencies_sorted else 0,
        'avg_planner_latency_ms': round(mean(planner_latencies), 2) if planner_latencies else 0,
        'max_planner_latency_ms': max(planner_latencies) if planner_latencies else 0,
        'status_code_histogram': _histogram(status_codes),
        'app_status_histogram': _histogram_strings(app_statuses),
        'request_runtime_mode_histogram': _histogram_strings(request_runtime_modes),
        'planning_source_histogram': _histogram_strings(planning_sources),
        'tool_call_count_histogram': _histogram(tool_call_counts),
    }


def _histogram(status_codes: list[int]) -> dict[str, int]:
    histogram: dict[str, int] = {}
    for code in status_codes:
        key = str(code)
        histogram[key] = histogram.get(key, 0) + 1
    return histogram


def _histogram_strings(values: list[str]) -> dict[str, int]:
    histogram: dict[str, int] = {}
    for value in values:
        histogram[value] = histogram.get(value, 0) + 1
    return histogram


def choose_runtime_mode(*, index: int, runtime_mode: str | None, llm_percent: int) -> str | None:
    if runtime_mode:
        return runtime_mode
    if llm_percent <= 0:
        return 'heuristic'
    if llm_percent >= 100:
        return 'llm'
    previous_bucket = (index * llm_percent) // 100
    current_bucket = ((index + 1) * llm_percent) // 100
    if current_bucket > previous_bucket:
        return 'llm'
    return 'heuristic'


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument('--base-url', type=str, default='http://127.0.0.1:18000')
    parser.add_argument('--rps', type=int, default=100)
    parser.add_argument('--duration', type=int, default=20, help='Duration in seconds.')
    parser.add_argument('--concurrency', type=int, default=50)
    parser.add_argument('--user-id', type=int, default=1)
    parser.add_argument('--timeout', type=float, default=20.0, help='Per-request timeout in seconds.')
    parser.add_argument('--runtime-mode', type=str, default=None, help='Force one runtime mode for all requests: heuristic or llm.')
    parser.add_argument('--llm-percent', type=int, default=0, help='When runtime-mode is not set, send this percentage of requests with runtime_mode=llm and the rest with runtime_mode=heuristic.')
    parser.add_argument('--message-file', type=str, default=None, help='Optional text file with one message per line.')
    args = parser.parse_args()

    messages = DEFAULT_MESSAGES
    if args.message_file:
        from pathlib import Path

        path = Path(args.message_file)
        loaded = [line.strip() for line in path.read_text(encoding='utf-8-sig').splitlines() if line.strip()]
        if loaded:
            messages = loaded

    report = asyncio.run(
        run_load_test(
            base_url=args.base_url,
            rps=args.rps,
            duration_seconds=args.duration,
            concurrency=args.concurrency,
            user_id=args.user_id,
            messages=messages,
            timeout_seconds=args.timeout,
            runtime_mode=args.runtime_mode,
            llm_percent=max(0, min(args.llm_percent, 100)),
        )
    )
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
