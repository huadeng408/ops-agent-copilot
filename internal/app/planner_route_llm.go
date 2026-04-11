package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type RouteDecision struct {
	Intent       string         `json:"intent"`
	Tool         string         `json:"tool"`
	Args         map[string]any `json:"args"`
	Confidence   float64        `json:"confidence"`
	NeedApproval bool           `json:"need_approval"`
}

func buildRouteSystemPrompt(noThink bool, strongModel bool) string {
	lines := []string{
		"You are the fast routing layer for an operations copilot.",
		"Return exactly one route_decision function call immediately.",
		"Do not answer with prose.",
		"Do not plan multiple steps.",
		"Choose one tool from the candidate tools only.",
		"Copy only argument keys that belong to the chosen tool.",
		"Set confidence to a number between 0 and 1.",
		"Set need_approval=true only for write proposal tools.",
	}
	if noThink {
		lines = append(lines, "/no_think")
	}
	if strongModel {
		lines = append(lines, "This pass is for low-confidence or complex requests. Prefer the best single tool even when the request is more analytical.")
	}
	return strings.Join(lines, "\n")
}

func buildRouteInput(message string, memory map[string]any, hints []PlannedToolCall, recentCount int, includeSummary bool) []map[string]any {
	payload := map[string]any{
		"request":      trimText(strings.TrimSpace(message), 400),
		"memory_state": compactMemoryState(memory),
		"recent_turns": compactRecentTurns(memory, recentCount),
	}
	if includeSummary {
		if summary := strings.TrimSpace(asString(memory["summary"])); summary != "" {
			payload["summary"] = trimText(summary, 240)
		}
	}
	if len(hints) > 0 {
		payload["heuristic_hints"] = hints
	}
	return []map[string]any{
		{
			"role":    "user",
			"content": "Route this request:\n" + MustJSON(payload),
		},
	}
}

func compactMemoryState(memory map[string]any) map[string]any {
	state, _ := memory["memory_state"].(map[string]any)
	if len(state) == 0 {
		return map[string]any{}
	}
	keys := []string{
		"last_ticket_no",
		"last_region",
		"last_category",
		"last_date",
		"last_date_range",
		"last_report_type",
	}
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := state[key]; ok {
			result[key] = value
		}
	}
	return result
}

func compactRecentTurns(memory map[string]any, recentCount int) []map[string]any {
	if recentCount <= 0 {
		recentCount = 2
	}
	messages, _ := memory["messages"].([]map[string]any)
	if len(messages) == 0 {
		return []map[string]any{}
	}
	start := len(messages) - recentCount
	if start < 0 {
		start = 0
	}
	result := make([]map[string]any, 0, len(messages)-start)
	for _, item := range messages[start:] {
		text := trimText(strings.TrimSpace(asString(item["text"])), 160)
		if text == "" {
			continue
		}
		role := strings.TrimSpace(asString(item["role"]))
		if role == "" {
			role = "user"
		}
		result = append(result, map[string]any{
			"role": role,
			"text": text,
		})
	}
	return result
}

func selectSchemasForRouteDecision(message string, hints []PlannedToolCall, schemas []ToolSchema) ([]ToolSchema, bool) {
	includeGenerateReport := isExplicitReportRequest(message)
	includeSQL := strings.Contains(strings.ToLower(message), "sql")

	schemaByName := make(map[string]ToolSchema, len(schemas))
	for _, schema := range schemas {
		schemaByName[schema.Name] = schema
	}

	if len(hints) > 0 {
		selected := make([]ToolSchema, 0, len(hints))
		seen := make(map[string]bool, len(hints))
		for _, hint := range hints {
			if hint.ToolName == "generate_report" {
				includeGenerateReport = true
				continue
			}
			if hint.ToolName == "run_readonly_sql" && !includeSQL {
				continue
			}
			if seen[hint.ToolName] {
				continue
			}
			schema, ok := schemaByName[hint.ToolName]
			if !ok {
				continue
			}
			selected = append(selected, schema)
			seen[hint.ToolName] = true
		}
		if len(selected) > 0 || includeGenerateReport {
			return selected, includeGenerateReport
		}
	}

	selected := make([]ToolSchema, 0, len(schemas))
	for _, schema := range schemas {
		if schema.Name == "run_readonly_sql" && !includeSQL {
			continue
		}
		selected = append(selected, schema)
	}
	return selected, includeGenerateReport
}

func buildRouteDecisionTools(schemas []ToolSchema, includeGenerateReport bool) []map[string]any {
	candidateNames := make([]string, 0, len(schemas)+1)
	catalogLines := make([]string, 0, len(schemas)+1)

	sort.Slice(schemas, func(i int, j int) bool {
		return schemas[i].Name < schemas[j].Name
	})

	for _, schema := range schemas {
		candidateNames = append(candidateNames, schema.Name)
		catalogLines = append(catalogLines, "- "+schema.Name+routeSchemaArgsSummary(schema))
	}
	if includeGenerateReport {
		candidateNames = append(candidateNames, "generate_report")
		catalogLines = append(catalogLines, "- generate_report(report_type)")
	}

	description := "Choose the best tool and arguments quickly.\nCandidate tools:\n" + strings.Join(catalogLines, "\n")
	return []map[string]any{
		{
			"type":        "function",
			"name":        "route_decision",
			"description": description,
			"parameters": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"intent": map[string]any{
						"type": "string",
						"enum": []string{"query", "analysis", "write", "report", "unknown"},
					},
					"tool": map[string]any{
						"type": "string",
						"enum": candidateNames,
					},
					"args": map[string]any{
						"type": "object",
					},
					"confidence": map[string]any{
						"type":    "number",
						"minimum": 0,
						"maximum": 1,
					},
					"need_approval": map[string]any{
						"type": "boolean",
					},
				},
				"required": []string{"intent", "tool", "args", "confidence", "need_approval"},
			},
		},
	}
}

func routeSchemaArgsSummary(schema ToolSchema) string {
	properties, _ := schema.Parameters["properties"].(map[string]any)
	if len(properties) == 0 {
		return "()"
	}
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return "(" + strings.Join(keys, ",") + ")"
}

func parseRouteDecision(response map[string]any) (RouteDecision, bool) {
	for _, planned := range parsePlannedCalls(response) {
		if planned.ToolName != "route_decision" {
			continue
		}
		decision := RouteDecision{
			Intent:     asString(planned.Arguments["intent"]),
			Tool:       asString(planned.Arguments["tool"]),
			Confidence: asFloat(planned.Arguments["confidence"]),
		}
		if decision.Args == nil {
			decision.Args = map[string]any{}
		}
		if args, ok := planned.Arguments["args"].(map[string]any); ok {
			decision.Args = args
		}
		switch typed := planned.Arguments["need_approval"].(type) {
		case bool:
			decision.NeedApproval = typed
		case string:
			decision.NeedApproval = strings.EqualFold(strings.TrimSpace(typed), "true")
		}
		return decision, strings.TrimSpace(decision.Tool) != ""
	}
	return RouteDecision{}, false
}

func routeDecisionToPlannedCalls(decision RouteDecision, registry *ToolRegistry, hints []PlannedToolCall, userMessage string) ([]PlannedToolCall, bool) {
	toolName := strings.TrimSpace(decision.Tool)
	if toolName == "" {
		return nil, false
	}
	if toolName == "generate_report" {
		args := map[string]any{"report_type": "daily"}
		if supplied, ok := decision.Args["report_type"]; ok && strings.TrimSpace(asString(supplied)) != "" {
			args["report_type"] = asString(supplied)
		}
		return []PlannedToolCall{{ToolName: "generate_report", Arguments: args}}, true
	}

	schema, ok := registry.SchemaByName(toolName)
	if !ok {
		return nil, false
	}
	args := sanitizeArgumentsForSchema(schema, decision.Args)
	args = mergeHintArguments(toolName, args, hints)
	if toolType, ok := registry.ToolTypeByName(toolName); ok && toolType == "write" && strings.TrimSpace(asString(args["reason"])) == "" {
		args["reason"] = "根据用户请求生成 proposal: " + trimText(strings.TrimSpace(userMessage), 80)
	}
	if hasMissingRequiredArguments(schema, args) {
		return nil, false
	}
	return []PlannedToolCall{{ToolName: toolName, Arguments: args}}, true
}

func sanitizeArgumentsForSchema(schema ToolSchema, args map[string]any) map[string]any {
	if args == nil {
		args = map[string]any{}
	}
	properties, _ := schema.Parameters["properties"].(map[string]any)
	if len(properties) == 0 {
		return map[string]any{}
	}
	result := make(map[string]any, len(properties))
	for key := range properties {
		if value, ok := args[key]; ok {
			result[key] = value
		}
	}
	return result
}

func mergeHintArguments(toolName string, args map[string]any, hints []PlannedToolCall) map[string]any {
	if args == nil {
		args = map[string]any{}
	}
	for _, hint := range hints {
		if hint.ToolName != toolName {
			continue
		}
		for key, value := range hint.Arguments {
			if _, ok := args[key]; !ok || isBlankRouteValue(args[key]) {
				args[key] = value
			}
		}
		break
	}
	return args
}

func hasMissingRequiredArguments(schema ToolSchema, args map[string]any) bool {
	for _, key := range requiredKeysForSchema(schema) {
		value, ok := args[key]
		if !ok || isBlankRouteValue(value) {
			return true
		}
	}
	return false
}

func requiredKeysForSchema(schema ToolSchema) []string {
	raw, ok := schema.Parameters["required"]
	if !ok {
		return []string{}
	}
	switch typed := raw.(type) {
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if key := strings.TrimSpace(asString(item)); key != "" {
				result = append(result, key)
			}
		}
		return result
	default:
		return []string{}
	}
}

func isBlankRouteValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func shouldEscalateToStrongModel(decision RouteDecision, cfg Config, routeCalls []PlannedToolCall) bool {
	if len(routeCalls) == 0 {
		return true
	}
	return decision.Confidence < cfg.RouterConfidenceCutoff
}

func trimText(value string, maxLen int) string {
	if maxLen <= 0 || len([]rune(value)) <= maxLen {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxLen])
}

func prettyJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func routeTraceLabel(model string) string {
	lower := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(lower, "qwen3"):
		return "l1_qwen_router"
	case strings.HasPrefix(lower, "gemma4"):
		return "l2_gemma_router"
	default:
		return "llm_router"
	}
}

func describeRouteFailure(decision RouteDecision, routeCalls []PlannedToolCall) string {
	if len(routeCalls) == 0 {
		return "empty_or_invalid_route"
	}
	return fmt.Sprintf("low_confidence_%.2f", decision.Confidence)
}
