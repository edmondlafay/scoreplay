package handler

import (
	"encoding/json"
	"net/http"
)

// ErrorCode is a stable machine-readable identifier for an error condition.
type ErrorCode string

const (
	CodeInvalidRequest   ErrorCode = "INVALID_REQUEST"
	CodeValidationError  ErrorCode = "VALIDATION_ERROR"
	CodeTagNotFound      ErrorCode = "TAG_NOT_FOUND"
	CodeTagsNotFound     ErrorCode = "TAGS_NOT_FOUND"
	CodeMediaNotFound    ErrorCode = "MEDIA_NOT_FOUND"
	CodeUnsupportedFormat ErrorCode = "UNSUPPORTED_FORMAT"
	CodeFileTooLarge     ErrorCode = "FILE_TOO_LARGE"
	CodeAPIKeyRequired   ErrorCode = "API_KEY_REQUIRED"
	CodeInvalidAPIKey    ErrorCode = "INVALID_API_KEY"
	CodeInternalError    ErrorCode = "INTERNAL_ERROR"
)

type errorDetail struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

type errorResponse struct {
	Error errorDetail `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, code ErrorCode, msg string) {
	writeJSON(w, status, errorResponse{Error: errorDetail{Code: code, Message: msg}})
}
