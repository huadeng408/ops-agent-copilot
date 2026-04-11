package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type LangGraphClient struct {
	baseURL string
	client  *http.Client
}

type LangGraphChatRequest struct {
	TraceID   string         `json:"trace_id"`
	SessionID string         `json:"session_id"`
	UserID    int64          `json:"user_id"`
	Message   string         `json:"message"`
	Memory    map[string]any `json:"memory"`
}

type LangGraphChatResponse struct {
	Status           string            `json:"status"`
	Answer           string            `json:"answer"`
	PlanningSource   string            `json:"planning_source,omitempty"`
	PlannerLatencyMS int               `json:"planner_latency_ms,omitempty"`
	PlanCacheHit     bool              `json:"plan_cache_hit"`
	ToolCalls        []ToolCallSummary `json:"tool_calls"`
	Approval         *ApprovalBrief    `json:"approval,omitempty"`
	PlannedCalls     []PlannedToolCall `json:"planned_calls,omitempty"`
}

func NewLangGraphClient(cfg Config) *LangGraphClient {
	timeout := time.Duration(cfg.LangGraphTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &LangGraphClient{
		baseURL: strings.TrimRight(cfg.LangGraphBaseURL, "/"),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *LangGraphClient) Enabled() bool {
	return strings.TrimSpace(c.baseURL) != ""
}

func (c *LangGraphClient) Chat(ctx context.Context, request LangGraphChatRequest) (LangGraphChatResponse, error) {
	if !c.Enabled() {
		return LangGraphChatResponse{}, NewValidation("LangGraph runtime 未配置，请设置 LANGGRAPH_BASE_URL")
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return LangGraphChatResponse{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat", bytes.NewReader(payload))
	if err != nil {
		return LangGraphChatResponse{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-Trace-ID", request.TraceID)

	response, err := c.client.Do(httpRequest)
	if err != nil {
		return LangGraphChatResponse{}, err
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		var remoteErr map[string]any
		_ = json.NewDecoder(response.Body).Decode(&remoteErr)
		detail := strings.TrimSpace(asString(remoteErr["detail"]))
		if detail == "" {
			detail = fmt.Sprintf("LangGraph 服务返回错误状态: %d", response.StatusCode)
		}
		return LangGraphChatResponse{}, errors.New(detail)
	}

	var result LangGraphChatResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return LangGraphChatResponse{}, err
	}
	return result, nil
}
