package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"
)

func TestChatApprovalAuditIntegration(t *testing.T) {
	t.Helper()

	cfg := Config{
		AppEnv:                 "test",
		AppName:                "ops-agent-copilot",
		DatabaseURL:            filepath.Join(t.TempDir(), "integration.sqlite3"),
		RedisURL:               "memory",
		AgentRuntimeMode:       "heuristic",
		KeepRecentMessageCount: 8,
		ReadonlySQLLimit:       200,
	}

	db, dialect, err := OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := SeedDemoData(context.Background(), db, dialect); err != nil {
		t.Fatalf("seed demo data: %v", err)
	}

	application := NewApplication(cfg, db)
	server := httptest.NewServer(application.Router)
	defer server.Close()

	client := &http.Client{Timeout: 15 * time.Second}

	metricResponse := mustPostJSON(t, client, server.URL+"/api/v1/chat", map[string]any{
		"session_id": "itest_metric",
		"user_id":    1,
		"message":    "最近7天北京区退款率最高的类目是什么？",
	})
	if got := asString(metricResponse["status"]); got != "completed" {
		t.Fatalf("metric chat status=%s", got)
	}
	if got := firstToolName(metricResponse); got != "query_refund_metrics" {
		t.Fatalf("metric first tool=%s", got)
	}

	writeResponse := mustPostJSON(t, client, server.URL+"/api/v1/chat", map[string]any{
		"session_id": "itest_write",
		"user_id":    1,
		"message":    "把T202603280012 分派给王磊",
	})
	if got := asString(writeResponse["status"]); got != "approval_required" {
		t.Fatalf("write chat status=%s", got)
	}

	traceID := asString(writeResponse["trace_id"])
	if traceID == "" {
		t.Fatal("trace_id should not be empty")
	}

	approvalMap, _ := writeResponse["approval"].(map[string]any)
	approvalNo := asString(approvalMap["approval_no"])
	if approvalNo == "" {
		t.Fatal("approval_no should not be empty")
	}

	approveURL := fmt.Sprintf("%s/api/v1/approvals/%s/approve", server.URL, url.PathEscape(approvalNo))
	approveResponse := mustPostJSON(t, client, approveURL, map[string]any{
		"approver_user_id": 2,
	})
	if got := asString(approveResponse["status"]); got != "executed" {
		t.Fatalf("approve status=%s", got)
	}

	executionResult, _ := approveResponse["execution_result"].(map[string]any)
	if got := asString(executionResult["ticket_no"]); got != "T202603280012" {
		t.Fatalf("execution_result.ticket_no=%s", got)
	}
	if got := asString(executionResult["assignee_name"]); got != "王磊" {
		t.Fatalf("execution_result.assignee_name=%s", got)
	}

	repeatApproveResponse := mustPostJSON(t, client, approveURL, map[string]any{
		"approver_user_id": 2,
	})
	if got := asString(repeatApproveResponse["status"]); got != "executed" {
		t.Fatalf("repeat approve status=%s", got)
	}

	auditResponse := mustGetJSON(t, client, server.URL+"/api/v1/audit?trace_id="+url.QueryEscape(traceID))
	if count := asInt(auditResponse["count"], 0); count <= 0 {
		t.Fatalf("audit count=%d", count)
	}
	for _, expected := range []string{"proposal_created", "approval_approved", "write_executed"} {
		if !hasAuditEvent(auditResponse, expected) {
			t.Fatalf("audit missing event=%s", expected)
		}
	}

	ticketResponse := mustGetJSON(t, client, server.URL+"/api/v1/tickets/T202603280012")
	ticketMap, _ := ticketResponse["ticket"].(map[string]any)
	if got := asString(ticketMap["assignee_name"]); got != "王磊" {
		t.Fatalf("ticket assignee_name=%s", got)
	}
	if !hasTicketAction(ticketResponse, "assign_ticket") {
		t.Fatal("ticket actions should contain assign_ticket")
	}
}

func mustPostJSON(t *testing.T, client *http.Client, endpoint string, payload map[string]any) map[string]any {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", endpoint, err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode post %s: %v", endpoint, err)
	}
	if resp.StatusCode >= 400 {
		t.Fatalf("post %s status=%d body=%v", endpoint, resp.StatusCode, result)
	}
	return result
}

func mustGetJSON(t *testing.T, client *http.Client, endpoint string) map[string]any {
	t.Helper()

	resp, err := client.Get(endpoint)
	if err != nil {
		t.Fatalf("get %s: %v", endpoint, err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode get %s: %v", endpoint, err)
	}
	if resp.StatusCode >= 400 {
		t.Fatalf("get %s status=%d body=%v", endpoint, resp.StatusCode, result)
	}
	return result
}

func firstToolName(response map[string]any) string {
	toolCalls, _ := response["tool_calls"].([]any)
	if len(toolCalls) == 0 {
		return ""
	}
	first, _ := toolCalls[0].(map[string]any)
	return asString(first["tool_name"])
}

func hasAuditEvent(response map[string]any, eventType string) bool {
	logs, _ := response["logs"].([]any)
	for _, item := range logs {
		log, _ := item.(map[string]any)
		if asString(log["event_type"]) == eventType {
			return true
		}
	}
	return false
}

func hasTicketAction(response map[string]any, actionType string) bool {
	actions, _ := response["actions"].([]any)
	for _, item := range actions {
		action, _ := item.(map[string]any)
		if asString(action["action_type"]) == actionType {
			return true
		}
	}
	return false
}
