package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
)

// SuccessResponse defines the standard success envelope {"data": ...}.
type SuccessResponse[T any] struct {
	Data T `json:"data"`
}

// ErrorResponse defines the standard error envelope {"code": "...", "message": "...", "request_id": "..."}.
type ErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// PaginatedData holds standard pagination items and metadata.
type PaginatedData[T any] struct {
	Items    []T `json:"items"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

const (
	DefaultPageSize = 50
	MaxPageSize     = 100
)

// WriteJSON sends a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

// WriteSuccess writes a standard {"data": ...} response.
func WriteSuccess(w http.ResponseWriter, r *http.Request, statusCode int, data any) {
	WriteJSON(w, statusCode, SuccessResponse[any]{Data: data})
}

// sanitizeErrorMessage redacts sensitive information and scrubs internal database or runtime artifacts.
func sanitizeErrorMessage(msg string) string {
	redacted := domain.RedactSensitiveInfo(msg)

	// Clean database details if they appear
	if strings.Contains(redacted, "sqlite3:") || strings.Contains(redacted, "SQL logic error") {
		return "A database constraint or integrity error occurred"
	}
	return redacted
}

// WriteError writes a standard {"code": "...", "message": "...", "request_id": "..."} response.
// Message is automatically redacted to prevent secret leaks.
func WriteError(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
	reqID := GetRequestID(r.Context())
	if reqID == "" {
		reqID = r.Header.Get(RequestIDHeader)
	}

	sanitizedMsg := sanitizeErrorMessage(message)

	WriteJSON(w, statusCode, ErrorResponse{
		Code:      code,
		Message:   sanitizedMsg,
		RequestID: reqID,
	})
}

// WriteDomainError maps domain errors to standard HTTP status codes and responses.
func WriteDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var domErr *domain.DomainError
	if errors.As(err, &domErr) {
		statusCode := http.StatusInternalServerError
		switch domErr.Category {
		case domain.CategoryValidation:
			statusCode = http.StatusUnprocessableEntity
		case domain.CategoryNotFound:
			statusCode = http.StatusNotFound
		case domain.CategoryConflict:
			statusCode = http.StatusConflict
		case domain.CategoryUnauthorized:
			statusCode = http.StatusUnauthorized
		case domain.CategoryForbidden, domain.CategorySecurity:
			statusCode = http.StatusForbidden
		case domain.CategoryInternal:
			statusCode = http.StatusInternalServerError
		}
		WriteError(w, r, statusCode, domErr.Code, domErr.Message)
		return
	}

	var capErr *compiler.CapabilityError
	if errors.As(err, &capErr) {
		WriteError(w, r, http.StatusUnprocessableEntity, "unsupported_target_capability", capErr.Error())
		return
	}

	// Unknown error: treat as internal server error and avoid leaking raw error details
	WriteError(w, r, http.StatusInternalServerError, "internal_error", "An internal error occurred")
}

// WritePaginated writes a paginated items envelope inside the data wrapper.
func WritePaginated[T any](w http.ResponseWriter, r *http.Request, items []T, page, pageSize, total int) {
	if items == nil {
		items = make([]T, 0)
	}
	WriteSuccess(w, r, http.StatusOK, PaginatedData[T]{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	})
}

// ParsePagination extracts page and page_size from request query parameters.
// Defaults: page = 1, page_size = 50. Maximum page_size = 100.
// If page_size > 100, returns a validation domain error.
func ParsePagination(r *http.Request) (int, int, error) {
	page := 1
	pageSize := DefaultPageSize

	q := r.URL.Query()
	if pageStr := q.Get("page"); pageStr != "" {
		p, err := strconv.Atoi(pageStr)
		if err != nil || p < 1 {
			return 0, 0, domain.NewValidationError("invalid_page", "page must be a positive integer greater than or equal to 1")
		}
		page = p
	}

	if pageSizeStr := q.Get("page_size"); pageSizeStr != "" {
		ps, err := strconv.Atoi(pageSizeStr)
		if err != nil || ps < 1 {
			return 0, 0, domain.NewValidationError("invalid_page_size", "page_size must be a positive integer")
		}
		if ps > MaxPageSize {
			return 0, 0, domain.NewValidationError("invalid_page_size", fmt.Sprintf("page_size exceeds maximum allowed limit of %d", MaxPageSize))
		}
		pageSize = ps
	}

	return page, pageSize, nil
}
