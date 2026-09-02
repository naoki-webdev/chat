package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *server) handleHealth(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	if postgres, ok := s.repository.(*postgresRepository); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := postgres.pool.Ping(ctx); err != nil {
			observabilityLogger.Error("health_check_failed", "request_id", writer.Header().Get(requestIDHeader), "error", err.Error())
			writeError(writer, http.StatusServiceUnavailable, "service unavailable")
			return
		}
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func writeRepositoryError(writer http.ResponseWriter, err error) {
	var validation validationError
	switch {
	case errors.As(err, &validation):
		writeErrorCode(writer, http.StatusBadRequest, "INVALID_INPUT", validation.Error())
	case errors.Is(err, ErrUnauthorized):
		writeErrorCode(writer, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
	case errors.Is(err, ErrForbidden):
		writeErrorCode(writer, http.StatusForbidden, "MESSAGE_FORBIDDEN", "you can only change your own messages")
	case errors.Is(err, ErrChannelManageForbidden):
		writeErrorCode(writer, http.StatusForbidden, "CHANNEL_MANAGE_FORBIDDEN", "you do not have permission to manage this channel")
	case errors.Is(err, ErrNotMember):
		writeErrorCode(writer, http.StatusForbidden, "CHANNEL_MEMBERSHIP_REQUIRED", "you are not a member of this channel")
	case errors.Is(err, ErrNotFound):
		writeErrorCode(writer, http.StatusNotFound, "NOT_FOUND", "resource not found")
	case errors.Is(err, ErrConflict):
		writeErrorCode(writer, http.StatusConflict, "CONFLICT", "resource already exists")
	default:
		observabilityLogger.Error("repository_error", "request_id", writer.Header().Get(requestIDHeader), "error", err.Error())
		writeErrorCode(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, destination any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(writer, http.StatusRequestEntityTooLarge, "request body is too large")
			return false
		}
		writeError(writer, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(writer, http.StatusBadRequest, "invalid JSON: request body must contain one value")
		return false
	}
	return true
}

func queryLimit(request *http.Request) (int, error) {
	value := request.URL.Query().Get("limit")
	if value == "" {
		return 50, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > 100 {
		return 0, invalidInput("limit must be between 1 and 100")
	}
	return limit, nil
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeErrorCode(writer, status, errorCodeForStatus(status), message)
}

func writeErrorCode(writer http.ResponseWriter, status int, code, message string) {
	payload := struct {
		Error     string `json:"error"`
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id,omitempty"`
	}{Error: message, Code: code, Message: message, RequestID: writer.Header().Get(requestIDHeader)}
	writeJSON(writer, status, payload)
}

func errorCodeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "INVALID_REQUEST"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusMethodNotAllowed:
		return "METHOD_NOT_ALLOWED"
	case http.StatusRequestEntityTooLarge:
		return "PAYLOAD_TOO_LARGE"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusTooManyRequests:
		return "RATE_LIMITED"
	case http.StatusBadGateway:
		return "UPSTREAM_ERROR"
	default:
		return "INTERNAL_ERROR"
	}
}

func methodNotAllowed(writer http.ResponseWriter, methods ...string) {
	writer.Header().Set("Allow", strings.Join(methods, ", "))
	writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
}
