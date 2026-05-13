package services

import "errors"

var (
	ErrValidation      = errors.New("validation error")
	ErrDuplicateMember = errors.New("registered member already exists")
	ErrNotFound        = errors.New("record not found")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrConflict        = errors.New("conflict")
)

type ServiceError struct {
	Kind    error
	Message string
	Err     error
}

func (e *ServiceError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	if e.Kind != nil {
		return e.Kind.Error()
	}

	return "service error"
}

func (e *ServiceError) Unwrap() error {
	if e.Err != nil {
		return e.Err
	}

	return e.Kind
}

func validationError(message string) error {
	return &ServiceError{Kind: ErrValidation, Message: message}
}

func notFoundError(message string) error {
	return &ServiceError{Kind: ErrNotFound, Message: message}
}

func conflictError(message string, err error) error {
	return &ServiceError{Kind: ErrConflict, Message: message, Err: err}
}
