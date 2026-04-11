package app

import "testing"

func TestRouteSmoke(t *testing.T) {
	router := &MessageRouter{}
	cases := []struct {
		message string
		want    string
	}{
		{"最近7天北京区退款率最高的类目是什么？", "query_refund_metrics"},
		{"北京区昨天退款率异常和超SLA工单做一下归因分析", "analyze_operational_anomaly"},
		{"把T202603280012 分派给王磊", "propose_assign_ticket"},
	}
	for _, tc := range cases {
		got := router.Route(tc.message, map[string]any{})
		if len(got) == 0 || got[0].ToolName != tc.want {
			t.Fatalf("message=%q got=%+v want=%s", tc.message, got, tc.want)
		}
	}
}

func TestAnalyzeSeparatesFastPathFromStrongModel(t *testing.T) {
	router := &MessageRouter{}

	fast := router.Analyze("把T202603280012 分派给王磊", map[string]any{})
	if len(fast.FastPath) != 1 || fast.FastPath[0].ToolName != "propose_assign_ticket" {
		t.Fatalf("unexpected fast path: %+v", fast.FastPath)
	}
	if fast.RequiresStrongModel {
		t.Fatal("write proposal request should not require strong model")
	}

	strong := router.Analyze("北京区昨天退款率异常和超SLA工单做一下归因分析", map[string]any{})
	if !strong.RequiresStrongModel {
		t.Fatal("expected anomaly correlation request to require strong model")
	}
	if len(strong.FastPath) != 0 {
		t.Fatalf("unexpected fast path for strong request: %+v", strong.FastPath)
	}
	if len(strong.Hints) != 1 || strong.Hints[0].ToolName != "analyze_operational_anomaly" {
		t.Fatalf("unexpected hints for strong request: %+v", strong.Hints)
	}
}
