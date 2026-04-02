package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

type LLMService struct {
	cfg     Config
	metrics *MetricsRecorder
	client  *http.Client
}

func NewLLMService(cfg Config, metrics *MetricsRecorder) *LLMService {
	return &LLMService{
		cfg:     cfg,
		metrics: metrics,
		client: NewTracingHTTPClient(&http.Client{
			Timeout: 60 * time.Second,
		}),
	}
}

func (s *LLMService) ResponsesCreate(ctx context.Context, inputItems []map[string]any, tools []map[string]any, instructions string, parallelToolCalls bool) (map[string]any, error) {
	ctx, span := StartSpan(ctx, "llm.responses_create")
	defer span.End()
	span.SetAttributes(
		attribute.String("openai.model", s.cfg.OpenAIModel),
		attribute.Int("llm.input_items", len(inputItems)),
		attribute.Int("llm.tool_count", len(tools)),
		attribute.Bool("llm.parallel_tool_calls", parallelToolCalls),
	)

	payload := map[string]any{
		"model": s.cfg.OpenAIModel,
		"input": inputItems,
	}
	if instructions != "" {
		payload["instructions"] = instructions
	}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	payload["parallel_tool_calls"] = parallelToolCalls

	result, err := s.postJSON(ctx, "/responses", payload)
	if err == nil {
		s.metrics.RecordLLMRequest("/responses", true)
		span.SetAttributes(
			attribute.String("llm.endpoint", "/responses"),
			attribute.Bool("llm.fallback", false),
		)
		return result, nil
	}
	s.metrics.RecordLLMRequest("/responses", false)
	if !shouldFallbackToChatCompletions(err) {
		RecordSpanError(span, err)
		return nil, err
	}

	span.AddEvent("fallback_to_chat_completions")
	fallbackPayload := buildChatCompletionsPayload(s.cfg.OpenAIModel, inputItems, tools, instructions, parallelToolCalls)
	fallback, fallbackErr := s.postJSON(ctx, "/chat/completions", fallbackPayload)
	if fallbackErr != nil {
		s.metrics.RecordLLMRequest("/chat/completions", false)
		RecordSpanError(span, fallbackErr)
		return nil, fallbackErr
	}
	s.metrics.RecordLLMRequest("/chat/completions", true)
	span.SetAttributes(
		attribute.String("llm.endpoint", "/chat/completions"),
		attribute.Bool("llm.fallback", true),
	)
	return convertChatCompletionResponse(fallback), nil
}

func (s *LLMService) postJSON(ctx context.Context, endpoint string, payload map[string]any) (map[string]any, error) {
	baseURL, err := url.Parse(strings.TrimRight(s.cfg.OpenAIBaseURL, "/"))
	if err != nil {
		return nil, err
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + endpoint
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.OpenAIAPIKey)
	req.Header.Set("Content-Type", "application/json")
	if traceID := BusinessTraceIDFromContext(ctx); traceID != "" {
		req.Header.Set("X-Trace-ID", traceID)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(raw))
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func buildChatCompletionsPayload(model string, inputItems []map[string]any, tools []map[string]any, instructions string, parallelToolCalls bool) map[string]any {
	messages := make([]map[string]any, 0, len(inputItems)+1)
	if instructions != "" {
		messages = append(messages, map[string]any{"role": "system", "content": instructions})
	}
	for _, item := range inputItems {
		role, _ := item["role"].(string)
		if role == "" {
			role = "user"
		}
		content := item["content"]
		if content == nil {
			content = item["text"]
		}
		messages = append(messages, map[string]any{"role": role, "content": content})
	}
	payload := map[string]any{
		"model":    model,
		"messages": messages,
	}
	if len(tools) > 0 {
		converted := make([]map[string]any, 0, len(tools))
		for _, tool := range tools {
			converted = append(converted, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        tool["name"],
					"description": tool["description"],
					"parameters":  tool["parameters"],
				},
			})
		}
		payload["tools"] = converted
		payload["tool_choice"] = "auto"
		payload["parallel_tool_calls"] = parallelToolCalls
	}
	return payload
}

func convertChatCompletionResponse(response map[string]any) map[string]any {
	output := make([]map[string]any, 0)
	choices, _ := response["choices"].([]any)
	for _, choiceItem := range choices {
		choice, _ := choiceItem.(map[string]any)
		message, _ := choice["message"].(map[string]any)
		toolCalls, _ := message["tool_calls"].([]any)
		for _, toolCallItem := range toolCalls {
			toolCall, _ := toolCallItem.(map[string]any)
			function, _ := toolCall["function"].(map[string]any)
			output = append(output, map[string]any{
				"type":      "function_call",
				"name":      function["name"],
				"arguments": function["arguments"],
			})
		}
	}
	return map[string]any{"output": output, "raw": response}
}

func shouldFallbackToChatCompletions(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "/responses") && (strings.Contains(message, "404") || strings.Contains(message, "not found") || strings.Contains(message, "url.not_found"))
}
