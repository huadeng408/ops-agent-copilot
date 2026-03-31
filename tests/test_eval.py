from pathlib import Path

from scripts.run_eval import evaluate_dataset


def test_eval_metrics_output():
    report = evaluate_dataset(Path('eval/dataset.jsonl'))
    assert report['sample_count'] == 100
    assert 'tool_selection_accuracy' in report
    assert 'dangerous_action_interception_rate' in report
    assert 'approval_required_precision' in report
    assert 'chat_p95_latency_ms' in report
