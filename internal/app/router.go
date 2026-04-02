package app

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var regions = []string{"北京", "上海", "广州"}
var categories = []string{"生鲜", "餐饮", "酒店", "到店综合"}

type PlannedToolCall struct {
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments"`
}

type MessageRouter struct{}

func (r *MessageRouter) Route(message string, memory map[string]any) []PlannedToolCall {
	memoryState, _ := memory["memory_state"].(map[string]any)
	if r.shouldAnalyzeOperationalAnomaly(message) {
		return []PlannedToolCall{r.parseOperationalAnomaly(message, memoryState)}
	}
	if strings.Contains(message, "分派给") {
		return []PlannedToolCall{r.parseAssign(message, memoryState)}
	}
	if strings.Contains(message, "备注") && (strings.Contains(message, "补充") || strings.Contains(message, "添加")) {
		return []PlannedToolCall{r.parseAddComment(message, memoryState)}
	}
	if strings.Contains(message, "升级") && regexp.MustCompile(`\bP[123]\b`).MatchString(message) {
		return []PlannedToolCall{r.parseEscalate(message, memoryState)}
	}
	if strings.Contains(message, "日报") || strings.Contains(message, "周报") {
		return []PlannedToolCall{{ToolName: "generate_report", Arguments: map[string]any{"report_type": "daily"}}}
	}
	if strings.Contains(message, "超") && strings.Contains(strings.ToUpper(message), "SLA") {
		return []PlannedToolCall{r.parseSLAQuery(message, memoryState)}
	}
	ticketNo := r.extractTicketNo(message, memoryState)
	if ticketNo != "" && (strings.Contains(message, "详情") || strings.Contains(message, "操作记录")) {
		calls := []PlannedToolCall{{ToolName: "get_ticket_detail", Arguments: map[string]any{"ticket_no": ticketNo}}}
		if strings.Contains(message, "操作记录") || strings.Contains(message, "备注") {
			calls = append(calls, PlannedToolCall{ToolName: "get_ticket_comments", Arguments: map[string]any{"ticket_no": ticketNo}})
		}
		return calls
	}
	if strings.Contains(message, "发布") {
		return []PlannedToolCall{{ToolName: "get_recent_releases", Arguments: map[string]any{}}}
	}
	if strings.Contains(message, "退款率") && strings.Contains(message, "异常") {
		return []PlannedToolCall{r.parseRefundAnomaly(message, memoryState)}
	}
	if strings.Contains(message, "退款率") {
		return []PlannedToolCall{r.parseRefundMetric(message, memoryState)}
	}
	return []PlannedToolCall{{ToolName: "generate_report", Arguments: map[string]any{"report_type": "daily"}}}
}

func (r *MessageRouter) parseRefundAnomaly(message string, memoryState map[string]any) PlannedToolCall {
	startDate, endDate := r.extractDateRange(message, memoryState)
	return PlannedToolCall{
		ToolName: "find_refund_anomalies",
		Arguments: map[string]any{
			"start_date": startDate,
			"end_date":   endDate,
			"region":     r.extractRegion(message, memoryState),
			"top_k":      r.extractTopK(message, 5),
		},
	}
}

func (r *MessageRouter) parseRefundMetric(message string, memoryState map[string]any) PlannedToolCall {
	startDate, endDate := r.extractDateRange(message, memoryState)
	return PlannedToolCall{
		ToolName: "query_refund_metrics",
		Arguments: map[string]any{
			"start_date": startDate,
			"end_date":   endDate,
			"region":     r.extractRegion(message, memoryState),
			"category":   r.extractCategory(message, memoryState),
		},
	}
}

func (r *MessageRouter) parseOperationalAnomaly(message string, memoryState map[string]any) PlannedToolCall {
	return PlannedToolCall{
		ToolName: "analyze_operational_anomaly",
		Arguments: map[string]any{
			"date":   r.extractSingleDate(message, memoryState),
			"region": r.extractRegion(message, memoryState),
		},
	}
}

func (r *MessageRouter) parseSLAQuery(message string, memoryState map[string]any) PlannedToolCall {
	groupBy := ""
	if strings.Contains(message, "原因") {
		groupBy = "root_cause"
	}
	if strings.Contains(message, "优先级") {
		groupBy = "priority"
	}
	if groupBy == "" && (strings.Contains(message, "类目") || strings.Contains(message, "分类")) {
		groupBy = "category"
	}
	return PlannedToolCall{
		ToolName: "list_sla_breached_tickets",
		Arguments: map[string]any{
			"date":     r.extractSingleDate(message, memoryState),
			"region":   r.extractRegion(message, memoryState),
			"group_by": groupBy,
		},
	}
}

func (r *MessageRouter) parseAssign(message string, memoryState map[string]any) PlannedToolCall {
	ticketNo := r.extractTicketNo(message, memoryState)
	match := regexp.MustCompile(`分派给([\p{Han}A-Za-z0-9_]+)`).FindStringSubmatch(message)
	assigneeName := "待确认"
	if len(match) > 1 {
		assigneeName = match[1]
	}
	return PlannedToolCall{
		ToolName: "propose_assign_ticket",
		Arguments: map[string]any{
			"ticket_no":     ticketNo,
			"assignee_name": assigneeName,
			"reason":        "根据用户指令将 " + ticketNo + " 分派给 " + assigneeName,
		},
	}
}

func (r *MessageRouter) parseAddComment(message string, memoryState map[string]any) PlannedToolCall {
	ticketNo := r.extractTicketNo(message, memoryState)
	commentText := strings.TrimSpace(strings.Trim(strings.SplitN(message, "备注", 2)[1], " ：:"))
	match := regexp.MustCompile(`备注[:：]\s*(.+)$`).FindStringSubmatch(message)
	if len(match) > 1 {
		commentText = strings.TrimSpace(match[1])
	}
	return PlannedToolCall{
		ToolName: "propose_add_ticket_comment",
		Arguments: map[string]any{
			"ticket_no":    ticketNo,
			"comment_text": commentText,
			"reason":       "根据用户指令为 " + ticketNo + " 增加备注",
		},
	}
}

func (r *MessageRouter) parseEscalate(message string, memoryState map[string]any) PlannedToolCall {
	ticketNo := r.extractTicketNo(message, memoryState)
	match := regexp.MustCompile(`\b(P[123])\b`).FindStringSubmatch(message)
	priority := ""
	if len(match) > 1 {
		priority = match[1]
	}
	return PlannedToolCall{
		ToolName: "propose_escalate_ticket",
		Arguments: map[string]any{
			"ticket_no":    ticketNo,
			"new_priority": priority,
			"reason":       "根据用户指令将 " + ticketNo + " 升级为 " + priority,
		},
	}
}

func (r *MessageRouter) extractTicketNo(message string, memoryState map[string]any) string {
	match := regexp.MustCompile(`T\d{6,}`).FindString(message)
	if match != "" {
		return match
	}
	if r.isFollowupReference(message) {
		if value, ok := memoryState["last_ticket_no"].(string); ok {
			return value
		}
	}
	return ""
}

func (r *MessageRouter) extractRegion(message string, memoryState map[string]any) any {
	for _, region := range regions {
		if strings.Contains(message, region) {
			return region
		}
	}
	if r.isFollowupReference(message) {
		if value, ok := memoryState["last_region"].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return nil
}

func (r *MessageRouter) extractCategory(message string, memoryState map[string]any) any {
	for _, category := range categories {
		if strings.Contains(message, category) {
			return category
		}
	}
	if r.isFollowupReference(message) {
		if value, ok := memoryState["last_category"].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return nil
}

func (r *MessageRouter) extractTopK(message string, defaultValue int) int {
	match := regexp.MustCompile(`(\d+)\s*个`).FindStringSubmatch(message)
	if len(match) < 2 {
		return defaultValue
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return defaultValue
	}
	return value
}

func (r *MessageRouter) extractDateRange(message string, memoryState map[string]any) (string, string) {
	today := time.Now().Format("2006-01-02")
	now, _ := time.Parse("2006-01-02", today)
	if strings.Contains(message, "最近7天") || strings.Contains(message, "最近 7 天") {
		return now.AddDate(0, 0, -6).Format("2006-01-02"), now.Format("2006-01-02")
	}
	if strings.Contains(message, "上周") {
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		end := now.AddDate(0, 0, -weekday)
		start := end.AddDate(0, 0, -6)
		return start.Format("2006-01-02"), end.Format("2006-01-02")
	}
	if strings.Contains(message, "昨天") {
		yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
		return yesterday, yesterday
	}
	if r.isFollowupReference(message) {
		if dateRange, ok := memoryState["last_date_range"].(map[string]any); ok {
			startDate, _ := dateRange["start_date"].(string)
			endDate, _ := dateRange["end_date"].(string)
			if startDate != "" && endDate != "" {
				return startDate, endDate
			}
		}
	}
	return now.AddDate(0, 0, -6).Format("2006-01-02"), now.Format("2006-01-02")
}

func (r *MessageRouter) extractSingleDate(message string, memoryState map[string]any) string {
	today := time.Now().Format("2006-01-02")
	now, _ := time.Parse("2006-01-02", today)
	if strings.Contains(message, "昨天") {
		return now.AddDate(0, 0, -1).Format("2006-01-02")
	}
	if strings.Contains(message, "今天") {
		return now.Format("2006-01-02")
	}
	if r.isFollowupReference(message) {
		if value, ok := memoryState["last_date"].(string); ok && value != "" {
			return value
		}
	}
	return now.Format("2006-01-02")
}

func (r *MessageRouter) shouldAnalyzeOperationalAnomaly(message string) bool {
	keywords := []string{"归因", "关联分析", "关联一下", "异常原因", "发布影响"}
	for _, keyword := range keywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return strings.Contains(message, "退款率") && strings.Contains(strings.ToUpper(message), "SLA")
}

func (r *MessageRouter) isFollowupReference(message string) bool {
	keywords := []string{"他", "它", "这个工单", "该工单", "刚才", "刚刚", "上一个", "那个", "继续"}
	for _, keyword := range keywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}
