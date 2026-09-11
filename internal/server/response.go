package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// ErrorResponse represents standard error output compatible with frontend expectations.
type ErrorResponse struct {
	Error  string `json:"error"`
	Detail any    `json:"detail"`
	Status int    `json:"status"`
}

func renderJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func renderError(w http.ResponseWriter, status int, msg string, details ...any) {
	var detailVal any = msg
	if len(details) > 0 && details[0] != nil {
		detailVal = details[0]
	}
	resp := ErrorResponse{
		Error:  msg,
		Detail: detailVal,
		Status: status,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("missing request body")
	}
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 10*1024*1024)) // 10MB limit
	return dec.Decode(v)
}

func parseIDParam(r *http.Request, paramName string) (int64, error) {
	val := chi.URLParam(r, paramName)
	if val == "" {
		return 0, fmt.Errorf("missing parameter %s", paramName)
	}
	id, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer parameter %s: %w", paramName, err)
	}
	return id, nil
}
