package app

import (
	"context"
	"database/sql"
	"fmt"
)

type UserRepository struct {
	db DBTX
}

func NewUserRepository(db DBTX) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetByID(ctx context.Context, userID int64) (*User, error) {
	var user User
	if err := r.db.GetContext(ctx, &user, `SELECT id, username, display_name, role, created_at FROM users WHERE id = ?`, userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, NewNotFound(fmt.Sprintf("用户不存在: %d", userID))
		}
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) GetByDisplayName(ctx context.Context, displayName string) (*User, error) {
	var user User
	if err := r.db.GetContext(ctx, &user, `SELECT id, username, display_name, role, created_at FROM users WHERE display_name = ?`, displayName); err != nil {
		if err == sql.ErrNoRows {
			return nil, NewValidation(fmt.Sprintf("找不到分派对象: %s", displayName))
		}
		return nil, err
	}
	return &user, nil
}
