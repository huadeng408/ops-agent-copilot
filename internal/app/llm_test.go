package app

import "testing"

func TestBuildChatCompletionsPayloadPinsSingleToolChoice(t *testing.T) {
	payload := buildChatCompletionsPayload(
		"gemma4:e4b",
		[]map[string]any{{"role": "user", "content": "ping"}},
		[]map[string]any{{
			"name":        "list_sla_breached_tickets",
			"description": "sla",
			"parameters":  map[string]any{"type": "object"},
		}},
		"instructions",
		false,
	)

	toolChoice, ok := payload["tool_choice"].(map[string]any)
	if !ok {
		t.Fatalf("tool_choice type = %T, want map[string]any", payload["tool_choice"])
	}
	function, ok := toolChoice["function"].(map[string]any)
	if !ok {
		t.Fatalf("tool_choice.function type = %T", toolChoice["function"])
	}
	if got := asString(function["name"]); got != "list_sla_breached_tickets" {
		t.Fatalf("tool_choice.function.name = %q", got)
	}
}

func TestRelaxedPlannerSchemasMatchExecutableDefaults(t *testing.T) {
	refundRequired, _ := queryRefundMetricsTool{}.Schema().Parameters["required"].([]string)
	if len(refundRequired) != 2 || refundRequired[0] != "start_date" || refundRequired[1] != "end_date" {
		t.Fatalf("query_refund_metrics required = %+v", refundRequired)
	}

	slaRequired, _ := listSLABreachedTicketsTool{}.Schema().Parameters["required"].([]string)
	if len(slaRequired) != 1 || slaRequired[0] != "date" {
		t.Fatalf("list_sla_breached_tickets required = %+v", slaRequired)
	}
}

func TestParsePlannedCallsAcceptsConvertedChatCompletionShape(t *testing.T) {
	response := map[string]any{
		"output": []map[string]any{
			{
				"type":      "function_call",
				"name":      "list_sla_breached_tickets",
				"arguments": `{"date":"2026-04-03","group_by":"root_cause","region":"北京"}`,
			},
		},
	}

	planned := parsePlannedCalls(response)
	if len(planned) != 1 {
		t.Fatalf("len(planned) = %d, want 1", len(planned))
	}
	if planned[0].ToolName != "list_sla_breached_tickets" {
		t.Fatalf("ToolName = %q", planned[0].ToolName)
	}
	if asString(planned[0].Arguments["group_by"]) != "root_cause" {
		t.Fatalf("group_by = %q", asString(planned[0].Arguments["group_by"]))
	}
}
