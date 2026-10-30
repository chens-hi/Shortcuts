package model

import "fmt"

type ErrorCode string

const (
	ErrValidation     ErrorCode = "SC-0001"
	ErrNotFound       ErrorCode = "SC-0002"
	ErrMethod         ErrorCode = "SC-0003"
	ErrResource       ErrorCode = "SC-0004"
	ErrSave           ErrorCode = "SC-1004"
	ErrDeploy         ErrorCode = "SC-2001"
	ErrDeployDisabled ErrorCode = "SC-2002"
	ErrUninstall      ErrorCode = "SC-3001"
	ErrRuntime        ErrorCode = "SC-3002"
	ErrDelete         ErrorCode = "SC-4001"
	ErrTimeout        ErrorCode = "SC-9001"
	ErrService        ErrorCode = "SC-5000"
)

type APIError struct {
	Code     ErrorCode `json:"error"`
	Detail   string    `json:"detail,omitempty"`
	Fallback string    `json:"fallback,omitempty"`
}

func (e *APIError) Error() string {
	if e.Detail == "" {
		return string(e.Code)
	}
	return string(e.Code) + ": " + e.Detail
}

func Errorf(code ErrorCode, format string, args ...any) *APIError {
	return &APIError{Code: code, Detail: errorf(format, args...)}
}

func errorf(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}
