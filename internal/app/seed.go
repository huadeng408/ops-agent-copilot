package app

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/jmoiron/sqlx"
)

func SeedDemoData(ctx context.Context, db *sqlx.DB, dialect string) error {
	if err := EnsureSchema(ctx, db, dialect); err != nil {
		return err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := clearTables(ctx, tx, dialect); err != nil {
		return err
	}
	userIDs, err := seedUsers(ctx, tx)
	if err != nil {
		return err
	}
	if err := seedMetrics(ctx, tx); err != nil {
		return err
	}
	if err := seedReleases(ctx, tx); err != nil {
		return err
	}
	if err := seedTickets(ctx, tx, userIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func clearTables(ctx context.Context, tx *sqlx.Tx, dialect string) error {
	statements := []string{
		`DELETE FROM tool_call_logs`,
		`DELETE FROM audit_logs`,
		`DELETE FROM ticket_actions`,
		`DELETE FROM ticket_comments`,
		`DELETE FROM approvals`,
		`DELETE FROM agent_messages`,
		`DELETE FROM agent_sessions`,
		`DELETE FROM tickets`,
		`DELETE FROM releases`,
		`DELETE FROM metric_refund_daily`,
		`DELETE FROM users`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if dialect == "mysql" {
		for _, tableName := range []string{"users", "metric_refund_daily", "releases", "tickets", "agent_sessions", "agent_messages", "approvals", "ticket_comments", "ticket_actions", "audit_logs", "tool_call_logs"} {
			if _, err := tx.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s AUTO_INCREMENT = 1`, tableName)); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sqlite_sequence`); err != nil {
		return err
	}
	return nil
}

func seedUsers(ctx context.Context, tx *sqlx.Tx) (map[string]int64, error) {
	users := []struct {
		Key         string
		Username    string
		DisplayName string
		Role        string
	}{
		{"admin", "admin", "管理员", "admin"},
		{"approver", "approver", "审批人", "approver"},
		{"wanglei", "wanglei", "王磊", "ops"},
		{"zhaomin", "zhaomin", "赵敏", "ops"},
		{"lina", "lina", "李娜", "support"},
		{"chenjie", "chenjie", "陈杰", "support"},
		{"sunyu", "sunyu", "孙宇", "ops"},
		{"duty", "duty_manager", "值班负责人", "manager"},
	}
	result := make(map[string]int64, len(users))
	for _, user := range users {
		res, err := tx.ExecContext(ctx, `INSERT INTO users (username, display_name, role) VALUES (?, ?, ?)`, user.Username, user.DisplayName, user.Role)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		result[user.Key] = id
	}
	return result, nil
}

func seedMetrics(ctx context.Context, tx *sqlx.Tx) error {
	rng := rand.New(rand.NewSource(7))
	today := time.Now().In(time.Local).Truncate(24 * time.Hour)
	regions := []string{"北京", "上海", "广州"}
	categories := []string{"生鲜", "餐饮", "酒店", "到店综合"}
	anomalyDay := today.AddDate(0, 0, -2)
	for offset := 59; offset >= 0; offset-- {
		dt := today.AddDate(0, 0, -offset)
		for regionIndex, region := range regions {
			for categoryIndex, category := range categories {
				ordersCnt := 180 + rng.Intn(121)
				baseRate := 0.018 + float64(regionIndex)*0.004 + float64(categoryIndex)*0.003
				if sameDay(dt, anomalyDay) && region == "北京" && category == "生鲜" {
					baseRate = 0.112
				}
				if sameDay(dt, today) && region == "北京" && category == "生鲜" {
					baseRate = 0.085
				}
				refundRate := roundFloat(baseRate+rng.Float64()*0.01, 4)
				refundOrdersCnt := maxInt(1, int(float64(ordersCnt)*refundRate))
				gmv := roundFloat(float64(ordersCnt)*(40+rng.Float64()*80), 2)
				if _, err := tx.ExecContext(
					ctx,
					`INSERT INTO metric_refund_daily (dt, region, category, orders_cnt, refund_orders_cnt, refund_rate, gmv) VALUES (?, ?, ?, ?, ?, ?, ?)`,
					dt.Format("2006-01-02"),
					region,
					category,
					ordersCnt,
					refundOrdersCnt,
					refundRate,
					gmv,
				); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func seedReleases(ctx context.Context, tx *sqlx.Tx) error {
	now := time.Now()
	for i := 0; i < 10; i++ {
		releaseTime := now.Add(time.Duration(-i*48-2) * time.Hour)
		summary := fmt.Sprintf("常规发布批次 %d", i+1)
		if i == 0 {
			summary = "修复履约状态同步"
		}
		serviceName := "refund-analytics"
		if i%2 == 0 {
			serviceName = "ops-ticket-service"
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO releases (service_name, release_version, release_time, operator_name, change_summary) VALUES (?, ?, ?, ?, ?)`,
			serviceName,
			fmt.Sprintf("v1.0.%d", 20+i),
			releaseTime,
			"审批人",
			summary,
		); err != nil {
			return err
		}
	}
	return nil
}

func seedTickets(ctx context.Context, tx *sqlx.Tx, userIDs map[string]int64) error {
	rng := rand.New(rand.NewSource(13))
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	rootCauses := []string{"商家超时", "配送异常", "系统发布故障", "风控误拦截"}
	statuses := []string{"open", "processing", "resolved", "closed"}
	categories := []string{"生鲜", "餐饮", "酒店", "到店综合"}
	regions := []string{"北京", "上海", "广州"}
	assignees := []int64{userIDs["wanglei"], userIDs["zhaomin"], userIDs["lina"], userIDs["chenjie"], userIDs["sunyu"]}
	reporters := []int64{userIDs["wanglei"], userIDs["zhaomin"], userIDs["lina"], userIDs["chenjie"], userIDs["sunyu"]}

	var targetTicketID int64
	for i := 0; i < 80; i++ {
		createdAt := now.Add(-time.Duration(rng.Intn(15*24)) * time.Hour)
		region := regions[i%len(regions)]
		category := categories[i%len(categories)]
		rootCause := rootCauses[i%len(rootCauses)]
		status := statuses[i%len(statuses)]
		priority := []string{"P1", "P2", "P3"}[i%3]
		ticketNo := fmt.Sprintf("T%s%04d", createdAt.Format("20060102"), i+1)
		slaDeadline := createdAt.Add(8 * time.Hour)
		if i < 16 {
			region = "北京"
			status = "open"
			if i%2 == 0 {
				priority = "P1"
			} else {
				priority = "P2"
			}
			if i < 8 {
				rootCause = "系统发布故障"
			}
			createdAt = yesterday.Add(time.Duration(9+i%8) * time.Hour)
			slaDeadline = createdAt.Add(4 * time.Hour)
			ticketNo = fmt.Sprintf("T%s%04d", yesterday.Format("20060102"), i+1)
		}
		if i == 11 {
			ticketNo = "T202603280012"
		}
		title := fmt.Sprintf("%s%s运营工单 %d", region, category, i+1)
		description := fmt.Sprintf("%s 导致的运营异常，需要跟进处理。", rootCause)
		assigneeID := assignees[i%len(assignees)]
		reporterID := reporters[(i+1)%len(reporters)]
		resolvedAt := any(nil)
		if status == "resolved" || status == "closed" {
			resolvedAt = createdAt.Add(5 * time.Hour)
		}
		if i == 11 {
			title = "北京生鲜履约退款异常"
			description = "3 月 28 日上午发布后退款率和超 SLA 工单明显上升。"
			status = "open"
			priority = "P2"
			rootCause = "系统发布故障"
			assigneeID = userIDs["lina"]
			reporterID = userIDs["duty"]
			createdAt = yesterday.Add(10*time.Hour + 30*time.Minute)
			slaDeadline = yesterday.Add(15 * time.Hour)
			resolvedAt = nil
		}
		result, err := tx.ExecContext(
			ctx,
			`INSERT INTO tickets (ticket_no, region, category, title, description, status, priority, root_cause, assignee_id, reporter_id, sla_deadline, created_at, updated_at, resolved_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ticketNo, region, category, title, description, status, priority, rootCause, assigneeID, reporterID, slaDeadline, createdAt, createdAt.Add(time.Hour), resolvedAt,
		)
		if err != nil {
			return err
		}
		id, _ := result.LastInsertId()
		if ticketNo == "T202603280012" {
			targetTicketID = id
		}
	}

	if targetTicketID == 0 {
		return fmt.Errorf("target ticket not seeded")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO ticket_comments (ticket_id, comment_text, created_by) VALUES (?, ?, ?)`, targetTicketID, "已联系商家待回执", userIDs["lina"]); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO ticket_comments (ticket_id, comment_text, created_by) VALUES (?, ?, ?)`, targetTicketID, "确认与上午发布版本时间接近", userIDs["duty"]); err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO ticket_actions (ticket_id, action_type, old_value, new_value, operator_id, approval_id, trace_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		targetTicketID,
		"sync_context",
		nil,
		MustJSON(map[string]any{"note": "系统自动补充上下文"}),
		userIDs["admin"],
		nil,
		"seed_trace_ticket_001",
	); err != nil {
		return err
	}
	return nil
}

func sameDay(left time.Time, right time.Time) bool {
	return left.Format("2006-01-02") == right.Format("2006-01-02")
}

func roundFloat(value float64, digits int) float64 {
	shift := math.Pow10(digits)
	return math.Round(value*shift) / shift
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
