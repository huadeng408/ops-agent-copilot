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
