package app

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestSelectSchemasForRouteDecisionUsesHintTools(t *testing.T) {
	schemas := []ToolSchema{
		{Name: "query_refund_metrics"},
		{Name: "list_sla_breached_tickets"},
		{Name: "analyze_operational_anomaly"},
	}
	hints := []PlannedToolCall{
		{ToolName: "list_sla_breached_tickets", Arguments: map[string]any{"date": "2026-04-03"}},
	}

	selected, includeReport := selectSchemasForRouteDecision("北京区昨天超SLA工单按原因分类", hints, schemas)
	if includeReport {
		t.Fatal("unexpected report inclusion")
	}
	if len(selected) != 1 || selected[0].Name != "list_sla_breached_tickets" {
		t.Fatalf("selected = %+v", selected)
	}
}

func TestRouteDecisionToPlannedCallsSanitizesAndMergesHints(t *testing.T) {
	registry := BuildDefaultToolRegistry(NewAuditService(&AuditRepository{}), NewMetricsRecorder(prometheus.NewRegistry()))
	decision := RouteDecision{
		Tool:       "propose_assign_ticket",
		Confidence: 0.91,
		Args: map[string]any{
			"ticket_no":     "T202603280012",
			"assignee_name": "王磊",
			"extra":         "drop-me",
		},
	}
	hints := []PlannedToolCall{
		{
			ToolName: "propose_assign_ticket",
			Arguments: map[string]any{
				"reason": "根据用户指令将 T202603280012 分派给王磊",
			},
		},
	}

	calls, ok := routeDecisionToPlannedCalls(decision, registry, hints, "把T202603280012 分派给王磊")
	if !ok || len(calls) != 1 {
		t.Fatalf("calls=%+v ok=%v", calls, ok)
	}
	if calls[0].ToolName != "propose_assign_ticket" {
		t.Fatalf("tool = %q", calls[0].ToolName)
	}
	if _, ok := calls[0].Arguments["extra"]; ok {
		t.Fatal("unexpected extra argument after sanitization")
	}
	if calls[0].Arguments["reason"] == "" {
		t.Fatal("expected hint reason to be merged")
	}
}
