package app

import (
	"context"
	"strings"
	"time"
)

type MetricRepository struct {
	db DBTX
}

func NewMetricRepository(db DBTX) *MetricRepository {
	return &MetricRepository{db: db}
}

func (r *MetricRepository) QueryRefundMetrics(ctx context.Context, startDate string, endDate string, region string, category string) ([]map[string]any, error) {
	query := `SELECT dt, region, category, orders_cnt, refund_orders_cnt, refund_rate, gmv
		FROM metric_refund_daily
		WHERE dt >= ? AND dt <= ?`
	args := []any{startDate, endDate}
	if strings.TrimSpace(region) != "" {
		query += ` AND region = ?`
		args = append(args, region)
	}
	if strings.TrimSpace(category) != "" {
		query += ` AND category = ?`
		args = append(args, category)
	}
	query += ` ORDER BY dt ASC, refund_rate DESC`

	var rows []MetricRefundDaily
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		result = append(result, map[string]any{
			"dt":                row.DT,
			"region":            row.Region,
			"category":          row.Category,
			"orders_cnt":        row.OrdersCnt,
			"refund_orders_cnt": row.RefundOrdersCnt,
			"refund_rate":       row.RefundRate,
			"gmv":               row.GMV,
		})
	}
	return result, nil
}

func (r *MetricRepository) FindRefundAnomalies(ctx context.Context, startDate string, endDate string, region string, topK int) ([]map[string]any, error) {
	if topK <= 0 {
		topK = 5
	}
	query := `SELECT category,
		AVG(refund_rate) AS avg_refund_rate,
		SUM(refund_orders_cnt) AS refund_orders_cnt,
		SUM(orders_cnt) AS orders_cnt
		FROM metric_refund_daily
		WHERE dt >= ? AND dt <= ?`
	args := []any{startDate, endDate}
	if strings.TrimSpace(region) != "" {
		query += ` AND region = ?`
		args = append(args, region)
	}
	query += ` GROUP BY category ORDER BY avg_refund_rate DESC LIMIT ?`
	args = append(args, topK)

	var rows []struct {
		Category        string  `db:"category"`
		AvgRefundRate   float64 `db:"avg_refund_rate"`
		RefundOrdersCnt int     `db:"refund_orders_cnt"`
		OrdersCnt       int     `db:"orders_cnt"`
	}
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		result = append(result, map[string]any{
			"category":          row.Category,
			"avg_refund_rate":   row.AvgRefundRate,
			"refund_orders_cnt": row.RefundOrdersCnt,
			"orders_cnt":        row.OrdersCnt,
		})
	}
	return result, nil
}

func (r *MetricRepository) GetRefundSnapshot(ctx context.Context, targetDate string, region string) ([]map[string]any, error) {
	query := `SELECT dt, region, category, orders_cnt, refund_orders_cnt, refund_rate, gmv
		FROM metric_refund_daily
		WHERE dt = ?`
	args := []any{targetDate}
	if strings.TrimSpace(region) != "" {
		query += ` AND region = ?`
		args = append(args, region)
	}
	query += ` ORDER BY refund_rate DESC LIMIT 10`
	var rows []MetricRefundDaily
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		result = append(result, map[string]any{
			"category":    row.Category,
			"region":      row.Region,
			"refund_rate": row.RefundRate,
			"orders_cnt":  row.OrdersCnt,
		})
	}
	return result, nil
}

func (r *MetricRepository) FindRefundSpikeCandidates(ctx context.Context, targetDate string, region string, lookbackDays int, limit int) ([]map[string]any, error) {
	if lookbackDays <= 0 {
		lookbackDays = 7
	}
	if limit <= 0 {
		limit = 5
	}
	target, err := time.Parse("2006-01-02", targetDate)
	if err != nil {
		return nil, err
	}
	baselineStart := target.AddDate(0, 0, -lookbackDays).Format("2006-01-02")
	baselineEnd := target.AddDate(0, 0, -1).Format("2006-01-02")

	query := `SELECT target.region,
		target.category,
		target.refund_rate AS target_refund_rate,
		target.refund_orders_cnt,
		target.orders_cnt,
		baseline.baseline_refund_rate
		FROM metric_refund_daily AS target
		JOIN (
			SELECT region, category, AVG(refund_rate) AS baseline_refund_rate
			FROM metric_refund_daily
			WHERE dt >= ? AND dt <= ?`
	args := []any{baselineStart, baselineEnd}
	if strings.TrimSpace(region) != "" {
		query += ` AND region = ?`
		args = append(args, region)
	}
	query += ` GROUP BY region, category
		) AS baseline
		ON target.region = baseline.region AND target.category = baseline.category
		WHERE target.dt = ?`
	args = append(args, targetDate)
	if strings.TrimSpace(region) != "" {
		query += ` AND target.region = ?`
		args = append(args, region)
	}
	query += ` ORDER BY (target.refund_rate - baseline.baseline_refund_rate) DESC LIMIT ?`
	args = append(args, limit)

	var rows []struct {
		Region             string  `db:"region"`
		Category           string  `db:"category"`
		TargetRefundRate   float64 `db:"target_refund_rate"`
		RefundOrdersCnt    int     `db:"refund_orders_cnt"`
		OrdersCnt          int     `db:"orders_cnt"`
		BaselineRefundRate float64 `db:"baseline_refund_rate"`
	}
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		result = append(result, map[string]any{
			"region":               row.Region,
			"category":             row.Category,
			"target_refund_rate":   row.TargetRefundRate,
			"baseline_refund_rate": row.BaselineRefundRate,
			"delta_refund_rate":    row.TargetRefundRate - row.BaselineRefundRate,
			"refund_orders_cnt":    row.RefundOrdersCnt,
			"orders_cnt":           row.OrdersCnt,
		})
	}
	return result, nil
}
