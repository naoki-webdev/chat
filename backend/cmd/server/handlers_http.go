package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
)

func (s *server) handleHealth(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func writeRepositoryError(writer http.ResponseWriter, err error) {
	var validation validationError
	switch {
	case errors.As(err, &validation):
		writeError(writer, http.StatusBadRequest, validation.Error())
	case errors.Is(err, ErrUnauthorized):
		writeError(writer, http.StatusUnauthorized, "authentication required")
	case errors.Is(err, ErrForbidden):
		writeError(writer, http.StatusForbidden, "you can only change your own messages")
	case errors.Is(err, ErrNotMember):
		writeError(writer, http.StatusForbidden, "you are not a member of this channel")
	case errors.Is(err, ErrNotFound):
		writeError(writer, http.StatusNotFound, "resource not found")
	case errors.Is(err, ErrConflict):
		writeError(writer, http.StatusConflict, "resource already exists")
	default:
		log.Printf("repository error: %v", err)
		writeError(writer, http.StatusInternalServerError, "internal server error")
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
	writeJSON(writer, status, map[string]string{"error": message})
}

func methodNotAllowed(writer http.ResponseWriter) {
	writer.Header().Set("Allow", "GET, POST, PATCH, DELETE, OPTIONS")
	writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
}
