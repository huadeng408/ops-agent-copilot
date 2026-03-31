import argparse
import asyncio
import json
from pathlib import Path
from statistics import mean
import time

import httpx

from app.agents.router import MessageRouter


DEFAULT_DATASET_PATH = Path(__file__).resolve().parent.parent / 'eval' / 'dataset.jsonl'


def load_dataset(path: Path) -> list[dict]:
    return [json.loads(line) for line in path.read_text(encoding='utf-8-sig').splitlines() if line.strip()]


def evaluate_dataset(dataset_path: Path = DEFAULT_DATASET_PATH) -> dict:
    router = MessageRouter()
    rows = load_dataset(dataset_path)
    tool_hits = []
    task_hits = []
    danger_hits = []
    approval_predictions = []
    latencies = []

    for row in rows:
        started = time.perf_counter()
        planned = router.route(row['input'])
        latency_ms = int((time.perf_counter() - started) * 1000)
        latencies.append(latency_ms)

        predicted_tools = [item.tool_name for item in planned]
        expected_tools = row.get('expected_tools', [])
        dangerous = row.get('dangerous_action', False)
        requires_approval = any(tool.startswith('propose_') for tool in predicted_tools)

        tool_hit = any(tool in predicted_tools for tool in expected_tools)
        tool_hits.append(tool_hit)
        approval_predictions.append((requires_approval, dangerous))
        danger_hits.append((not dangerous) or requires_approval)

        keywords = row.get('expected_keywords', [])
        keyword_hit = bool(keywords) and any(keyword in row['input'] for keyword in keywords)
        task_hits.append(tool_hit and keyword_hit)

    return _build_report(
        rows=rows,
        latencies=latencies,
        tool_hits=tool_hits,
        task_hits=task_hits,
        danger_hits=danger_hits,
        approval_predictions=approval_predictions,
        mode='offline_router',
    )


async def evaluate_live_api(
    *,
    base_url: str,
    dataset_path: Path = DEFAULT_DATASET_PATH,
    user_id: int = 1,
) -> dict:
    rows = load_dataset(dataset_path)
    tool_hits = []
    task_hits = []
    danger_hits = []
    approval_predictions = []
    latencies = []

    async with httpx.AsyncClient(base_url=base_url.rstrip('/'), timeout=60.0) as client:
        for row in rows:
            payload = {
                'session_id': f"eval_{row['id']}",
                'user_id': user_id,
                'message': row['input'],
            }
            started = time.perf_counter()
            response = await client.post('/api/v1/chat', json=payload)
            latency_ms = int((time.perf_counter() - started) * 1000)
            latencies.append(latency_ms)

            response.raise_for_status()
            body = response.json()
            predicted_tools = [item['tool_name'] for item in body.get('tool_calls', [])]
            expected_tools = row.get('expected_tools', [])
            dangerous = row.get('dangerous_action', False)
            requires_approval = body.get('status') == 'approval_required'

            tool_hit = any(tool in predicted_tools for tool in expected_tools)
            tool_hits.append(tool_hit)
            approval_predictions.append((requires_approval, dangerous))
            danger_hits.append((not dangerous) or requires_approval)

            answer = str(body.get('answer') or '')
            keywords = row.get('expected_keywords', [])
            keyword_hit = all(keyword in answer or keyword in row['input'] for keyword in keywords)
            task_hits.append(tool_hit and keyword_hit)

    return _build_report(
        rows=rows,
        latencies=latencies,
        tool_hits=tool_hits,
        task_hits=task_hits,
        danger_hits=danger_hits,
        approval_predictions=approval_predictions,
        mode='live_api',
    )


def _build_report(
    *,
    rows: list[dict],
    latencies: list[int],
    tool_hits: list[bool],
    task_hits: list[bool],
    danger_hits: list[bool],
    approval_predictions: list[tuple[bool, bool]],
    mode: str,
) -> dict:
    latencies_sorted = sorted(latencies)
    p95_index = max(0, int(len(latencies_sorted) * 0.95) - 1)
    predicted_approval_count = sum(1 for predicted, _ in approval_predictions if predicted)
    true_positive_approvals = sum(1 for predicted, dangerous in approval_predictions if predicted and dangerous)
    dangerous_count = sum(1 for row in rows if row.get('dangerous_action'))

    return {
        'mode': mode,
        'sample_count': len(rows),
        'tool_selection_accuracy': round(sum(tool_hits) / len(rows), 4) if rows else 0,
        'task_success_rate': round(sum(task_hits) / len(rows), 4) if rows else 0,
        'dangerous_action_interception_rate': round(sum(danger_hits) / len(rows), 4) if rows else 0,
        'approval_required_precision': round(true_positive_approvals / predicted_approval_count, 4) if predicted_approval_count else 0,
        'human_override_rate': round(true_positive_approvals / dangerous_count, 4) if dangerous_count else 0,
        'chat_p95_latency_ms': latencies_sorted[p95_index] if latencies_sorted else 0,
        'p95_latency_ms': latencies_sorted[p95_index] if latencies_sorted else 0,
        'avg_token_cost': None,
        'avg_latency_ms': round(mean(latencies), 2) if latencies else 0,
        'predicted_approval_count': predicted_approval_count,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument('--dataset', type=Path, default=DEFAULT_DATASET_PATH)
    parser.add_argument('--base-url', type=str, default=None, help='Optional API base URL for live evaluation.')
    parser.add_argument('--user-id', type=int, default=1)
    args = parser.parse_args()

    if args.base_url:
        report = asyncio.run(evaluate_live_api(base_url=args.base_url, dataset_path=args.dataset, user_id=args.user_id))
    else:
        report = evaluate_dataset(args.dataset)
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
