package errors

import (
	"net/http"
)

type AppError struct {
	HTTPStatus int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *AppError) Error() string {
	return e.Message
}

func NewAppError(status int, code string, msg string) *AppError {
	return &AppError{
		HTTPStatus: status,
		Code:       code,
		Message:    msg,
	}
}

var (
	ErrNotFound            = NewAppError(http.StatusNotFound, "NOT_FOUND", "Resource not found")
	ErrUnauthorized        = NewAppError(http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized access")
	ErrForbidden           = NewAppError(http.StatusForbidden, "FORBIDDEN", "Access forbidden")
	ErrBadRequest          = NewAppError(http.StatusBadRequest, "BAD_REQUEST", "Bad request parameters")
	ErrInternalServer      = NewAppError(http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Internal server error")
	ErrInsufficientBalance = NewAppError(http.StatusBadRequest, "INSUFFICIENT_BALANCE", "Insufficient wallet balance")
	ErrDuplicateEntry      = NewAppError(http.StatusConflict, "DUPLICATE_ENTRY", "Resource already exists")
)

type ErrorResponse struct {
	Success bool     `json:"success"`
	Error   ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func FormatErrorResponse(err error) (int, ErrorResponse) {
	if appErr, ok := err.(*AppError); ok {
		return appErr.HTTPStatus, ErrorResponse{
			Success: false,
			Error: ErrorDetail{
				Code:    appErr.Code,
				Message: appErr.Message,
			},
		}
	}

	return http.StatusInternalServerError, ErrorResponse{
		Success: false,
		Error: ErrorDetail{
			Code:    "INTERNAL_SERVER_ERROR",
			Message: err.Error(),
		},
	}
}
