package app

import "errors"

var (
	ErrNotFound         = errors.New("not found")
	ErrPermissionDenied = errors.New("permission denied")
	ErrValidation       = errors.New("validation failed")
	ErrConflict         = errors.New("conflict")
)

type AppError struct {
	Kind    error
	Message string
}

func (e *AppError) Error() string {
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Kind
}

func NewNotFound(message string) error {
	return &AppError{Kind: ErrNotFound, Message: message}
}

func NewPermissionDenied(message string) error {
	return &AppError{Kind: ErrPermissionDenied, Message: message}
}

func NewValidation(message string) error {
	return &AppError{Kind: ErrValidation, Message: message}
}

func NewConflict(message string) error {
	return &AppError{Kind: ErrConflict, Message: message}
}
