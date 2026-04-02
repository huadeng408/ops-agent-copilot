package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type GuardResult struct {
	Passed   bool   `json:"passed"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type SQLGuard struct {
	AllowedObjects    map[string]struct{}
	ForbiddenPatterns []*regexp.Regexp
}

func NewSQLGuard() *SQLGuard {
	patterns := []string{
		`\binsert\b`,
		`\bupdate\b`,
		`\bdelete\b`,
		`\bdrop\b`,
		`\balter\b`,
		`\btruncate\b`,
		`\bcreate\b`,
		`\bunion\b`,
		`\boutfile\b`,
		`\binformation_schema\b`,
		`\bmysql\b`,
		`\bsleep\s*\(`,
		`\bbenchmark\s*\(`,
		`--`,
		`/\*`,
		`\*/`,
		`#`,
		`;`,
	}
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		compiled = append(compiled, regexp.MustCompile(pattern))
	}
	return &SQLGuard{
		AllowedObjects: map[string]struct{}{
			"v_refund_metrics_daily": {},
			"v_ticket_sla":           {},
			"v_ticket_detail":        {},
			"v_recent_releases":      {},
		},
		ForbiddenPatterns: compiled,
	}
}

func (g *SQLGuard) Validate(sql string, maxLimit int) GuardResult {
	candidate := strings.TrimSpace(sql)
	lowered := strings.ToLower(candidate)
	if !strings.HasPrefix(lowered, "select") {
		return GuardResult{Passed: false, Severity: "error", Message: "SQL 只允许 SELECT"}
	}
	for _, pattern := range g.ForbiddenPatterns {
		if pattern.MatchString(lowered) {
			return GuardResult{Passed: false, Severity: "error", Message: fmt.Sprintf("SQL 含有禁用内容: %s", pattern.String())}
		}
	}
	limitPattern := regexp.MustCompile(`\blimit\s+(\d+)\b`)
	matches := limitPattern.FindStringSubmatch(lowered)
	if len(matches) < 2 {
		return GuardResult{Passed: false, Severity: "error", Message: "SQL 必须带 LIMIT"}
	}
	limit, _ := strconv.Atoi(matches[1])
	if limit > maxLimit {
		return GuardResult{Passed: false, Severity: "error", Message: fmt.Sprintf("LIMIT 最大为 %d", maxLimit)}
	}
	objects := extractSQLObjects(candidate)
	if len(objects) == 0 {
		return GuardResult{Passed: false, Severity: "error", Message: "SQL 未识别到合法查询对象"}
	}
	disallowed := make([]string, 0)
	for _, object := range objects {
		if _, ok := g.AllowedObjects[object]; !ok {
			disallowed = append(disallowed, object)
		}
	}
	if len(disallowed) > 0 {
		return GuardResult{Passed: false, Severity: "error", Message: fmt.Sprintf("SQL 访问了非白名单对象: %s", strings.Join(disallowed, ", "))}
	}
	return GuardResult{Passed: true, Severity: "info", Message: "SQL 校验通过"}
}

func extractSQLObjects(sql string) []string {
	pattern := regexp.MustCompile(`(?i)\b(?:from|join)\s+([` + "`" + `"\[]?[a-zA-Z_][\w$]*(?:[` + "`" + `"\]]?\.[` + "`" + `"\[]?[a-zA-Z_][\w$]*[` + "`" + `"\]]?)?)`)
	matches := pattern.FindAllStringSubmatch(sql, -1)
	seen := make(map[string]struct{})
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		cleaned := strings.NewReplacer("`", "", `"`, "", "[", "", "]", "").Replace(match[1])
		parts := strings.Split(cleaned, ".")
		name := parts[len(parts)-1]
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result
}
