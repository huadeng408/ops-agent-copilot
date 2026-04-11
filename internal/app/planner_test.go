package app

import (
	"strings"
	"testing"
)

func TestPlannerSystemPromptForOllamaIsStrict(t *testing.T) {
	prompt := plannerSystemPromptFor("ollama")

	for _, fragment := range []string{
		"Return a tool call only",
		"Preferred tool call",
		"heuristic hints",
		"Prefer one tool call",
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("ollama prompt missing fragment %q", fragment)
		}
	}
}

func TestBuildPlannerInputForOllamaIncludesHintAndRequest(t *testing.T) {
	memory := map[string]any{
		"summary": "user often asks about SLA issues",
		"memory_state": map[string]any{
			"last_region": "北京",
		},
		"messages": []map[string]any{
			{"role": "user", "text": "昨天北京区情况如何"},
			{"role": "assistant", "text": "已记录"},
		},
	}
	hints := []PlannedToolCall{
		{
			ToolName: "list_sla_breached_tickets",
			Arguments: map[string]any{
				"date":     "2026-04-03",
				"region":   "北京",
				"group_by": "root_cause",
			},
		},
	}

	items := buildPlannerInputForOllama("北京区昨天超SLA的工单按原因分类", memory, hints)
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}

	systemContent := asString(items[0]["content"])
	if !strings.Contains(systemContent, "Heuristic tool hints") {
		t.Fatal("system content should contain heuristic hints")
	}
	if !strings.Contains(systemContent, "list_sla_breached_tickets") {
		t.Fatal("system content should contain the hinted tool name")
	}

	userContent := asString(items[1]["content"])
	if !strings.Contains(userContent, "Latest user request") {
		t.Fatal("user content should contain latest user request marker")
	}
	if !strings.Contains(userContent, "Return tool call(s) now.") {
		t.Fatal("user content should force immediate tool calling")
	}
}

func TestPlannerToolDescriptionAddsGenerateReportGuard(t *testing.T) {
	got := plannerToolDescription("generate_report", "生成运营日报或周报")
	if !strings.Contains(got, "Never use as fallback") {
		t.Fatalf("generate_report description missing fallback guard: %s", got)
	}
}

func TestSelectSchemasForOllamaPlannerUsesHintToolOnly(t *testing.T) {
	schemas := []ToolSchema{
		{Name: "query_refund_metrics"},
		{Name: "list_sla_breached_tickets"},
		{Name: "analyze_operational_anomaly"},
	}
	hints := []PlannedToolCall{
		{ToolName: "list_sla_breached_tickets", Arguments: map[string]any{"date": "2026-04-03"}},
	}

	selected, includeReport := selectSchemasForOllamaPlanner("北京区昨天超SLA的工单按原因分类", hints, schemas)
	if includeReport {
		t.Fatal("unexpected generate_report inclusion for non-report request")
	}
	if len(selected) != 1 || selected[0].Name != "list_sla_breached_tickets" {
		t.Fatalf("selected = %+v", selected)
	}
}

func TestBuildPlannerToolsForProviderOmitsGenerateReportForGenericOllamaRequest(t *testing.T) {
	schemas := []ToolSchema{
		{Name: "query_refund_metrics", Description: "query", Parameters: map[string]any{"type": "object"}},
		{Name: "list_sla_breached_tickets", Description: "sla", Parameters: map[string]any{"type": "object"}},
	}

	tools := buildPlannerToolsForProvider("ollama", "北京区昨天超SLA的工单按原因分类", []PlannedToolCall{
		{ToolName: "list_sla_breached_tickets", Arguments: map[string]any{"date": "2026-04-03"}},
	}, schemas)

	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}
	if asString(tools[0]["name"]) != "list_sla_breached_tickets" {
		t.Fatalf("tool name = %q", asString(tools[0]["name"]))
	}
}
