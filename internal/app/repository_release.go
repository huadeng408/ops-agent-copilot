package app

import (
	"context"
	"time"
)

type ReleaseRepository struct {
	db DBTX
}

func NewReleaseRepository(db DBTX) *ReleaseRepository {
	return &ReleaseRepository{db: db}
}

func (r *ReleaseRepository) GetRecentReleases(ctx context.Context, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 10
	}
	var rows []Release
	if err := r.db.SelectContext(
		ctx,
		&rows,
		`SELECT id, service_name, release_version, release_time, operator_name, change_summary, created_at
		 FROM releases
		 ORDER BY release_time DESC
		 LIMIT ?`,
		limit,
	); err != nil {
		return nil, err
	}
	return renderReleaseRows(rows), nil
}

func (r *ReleaseRepository) GetReleasesBetween(ctx context.Context, startTime string, endTime string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 10
	}
	var rows []Release
	if err := r.db.SelectContext(
		ctx,
		&rows,
		`SELECT id, service_name, release_version, release_time, operator_name, change_summary, created_at
		 FROM releases
		 WHERE release_time >= ? AND release_time <= ?
		 ORDER BY release_time DESC
		 LIMIT ?`,
		startTime,
		endTime,
		limit,
	); err != nil {
		return nil, err
	}
	return renderReleaseRows(rows), nil
}

func renderReleaseRows(rows []Release) []map[string]any {
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		result = append(result, map[string]any{
			"service_name":    row.ServiceName,
			"release_version": row.ReleaseVersion,
			"release_time":    row.ReleaseTime.Format(time.RFC3339),
			"operator_name":   row.OperatorName,
			"change_summary":  row.ChangeSummary,
		})
	}
	return result
}
