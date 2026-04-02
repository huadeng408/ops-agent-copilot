package app

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsRecorder struct {
	registry                  *prometheus.Registry
	chatRequestsTotal         *prometheus.CounterVec
	chatLatencyMS             prometheus.Histogram
	toolCallsTotal            *prometheus.CounterVec
	toolLatencyMS             *prometheus.HistogramVec
	approvalTransitionsTotal  *prometheus.CounterVec
	approvalTurnaroundSeconds *prometheus.HistogramVec
	verifierRejectionsTotal   *prometheus.CounterVec
	llmFallbackTotal          *prometheus.CounterVec
	llmRequestsTotal          *prometheus.CounterVec
	plannerRequestsTotal      *prometheus.CounterVec
	plannerLatencyMS          *prometheus.HistogramVec
	plannerCacheTotal         *prometheus.CounterVec
}

func NewMetricsRecorder(registry *prometheus.Registry) *MetricsRecorder {
	recorder := &MetricsRecorder{
		registry: registry,
		chatRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "ops_agent_chat_requests_total", Help: "Total chat requests grouped by final status."},
			[]string{"status"},
		),
		chatLatencyMS: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "ops_agent_chat_latency_ms",
				Help:    "Chat request latency in milliseconds.",
				Buckets: []float64{10, 25, 50, 100, 200, 400, 800, 1200, 2500, 5000, 10000},
			},
		),
		toolCallsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "ops_agent_tool_calls_total", Help: "Total tool calls grouped by tool name and success flag."},
			[]string{"tool_name", "success"},
		),
		toolLatencyMS: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ops_agent_tool_latency_ms",
				Help:    "Tool call latency in milliseconds.",
				Buckets: []float64{1, 5, 10, 20, 50, 100, 250, 500, 1000, 2500, 5000},
			},
			[]string{"tool_name"},
		),
		approvalTransitionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "ops_agent_approval_transitions_total", Help: "Approval status transitions."},
			[]string{"from_status", "to_status"},
		),
		approvalTurnaroundSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ops_agent_approval_turnaround_seconds",
				Help:    "Seconds from proposal creation to final approval outcome.",
				Buckets: []float64{1, 5, 15, 30, 60, 120, 300, 600, 1800, 3600, 7200},
			},
			[]string{"action_type", "final_status"},
		),
		verifierRejectionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "ops_agent_verifier_rejections_total", Help: "Verifier rejections grouped by stage."},
			[]string{"stage"},
		),
		llmFallbackTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "ops_agent_llm_fallback_total", Help: "Number of times the planner fell back away from the primary LLM path."},
			[]string{"reason"},
		),
		llmRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "ops_agent_llm_requests_total", Help: "Total outbound LLM requests grouped by endpoint and success flag."},
			[]string{"endpoint", "success"},
		),
		plannerRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "ops_agent_planner_requests_total", Help: "Planner requests grouped by planning source."},
			[]string{"source"},
		),
		plannerLatencyMS: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ops_agent_planner_latency_ms",
				Help:    "Planner latency in milliseconds.",
				Buckets: []float64{1, 5, 10, 20, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
			},
			[]string{"source"},
		),
		plannerCacheTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "ops_agent_planner_cache_total", Help: "Planner cache usage grouped by hit flag."},
			[]string{"hit"},
		),
	}

	registry.MustRegister(
		recorder.chatRequestsTotal,
		recorder.chatLatencyMS,
		recorder.toolCallsTotal,
		recorder.toolLatencyMS,
		recorder.approvalTransitionsTotal,
		recorder.approvalTurnaroundSeconds,
		recorder.verifierRejectionsTotal,
		recorder.llmFallbackTotal,
		recorder.llmRequestsTotal,
		recorder.plannerRequestsTotal,
		recorder.plannerLatencyMS,
		recorder.plannerCacheTotal,
	)
	return recorder
}

func (m *MetricsRecorder) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *MetricsRecorder) RecordChat(status string, latencyMS int) {
	m.chatRequestsTotal.WithLabelValues(status).Inc()
	m.chatLatencyMS.Observe(float64(latencyMS))
}

func (m *MetricsRecorder) RecordToolCall(toolName string, success bool, latencyMS int) {
	m.toolCallsTotal.WithLabelValues(toolName, strconv.FormatBool(success)).Inc()
	m.toolLatencyMS.WithLabelValues(toolName).Observe(float64(latencyMS))
}

func (m *MetricsRecorder) RecordApprovalTransition(fromStatus string, toStatus string) {
	m.approvalTransitionsTotal.WithLabelValues(fromStatus, toStatus).Inc()
}

func (m *MetricsRecorder) RecordApprovalTurnaround(actionType string, finalStatus string, seconds float64) {
	m.approvalTurnaroundSeconds.WithLabelValues(actionType, finalStatus).Observe(seconds)
}

func (m *MetricsRecorder) RecordVerifierRejection(stage string) {
	m.verifierRejectionsTotal.WithLabelValues(stage).Inc()
}

func (m *MetricsRecorder) RecordLLMFallback(reason string) {
	m.llmFallbackTotal.WithLabelValues(reason).Inc()
}

func (m *MetricsRecorder) RecordLLMRequest(endpoint string, success bool) {
	m.llmRequestsTotal.WithLabelValues(endpoint, strconv.FormatBool(success)).Inc()
}

func (m *MetricsRecorder) RecordPlanner(source string, latencyMS int) {
	m.plannerRequestsTotal.WithLabelValues(source).Inc()
	m.plannerLatencyMS.WithLabelValues(source).Observe(float64(latencyMS))
}

func (m *MetricsRecorder) RecordPlannerCache(hit bool) {
	m.plannerCacheTotal.WithLabelValues(strconv.FormatBool(hit)).Inc()
}
