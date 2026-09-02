package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDAndErrorResponse(t *testing.T) {
	handler := withRequestID(withLogging(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeError(writer, http.StatusBadRequest, "invalid request")
	})))
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	request.Header.Set(requestIDHeader, "request-123")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Header().Get(requestIDHeader) != "request-123" {
		t.Fatalf("request id = %q, want %q", recorder.Header().Get(requestIDHeader), "request-123")
	}
	var response struct {
		Error     string `json:"error"`
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Error != "invalid request" || response.Code != "INVALID_REQUEST" || response.Message != response.Error || response.RequestID != "request-123" {
		t.Fatalf("unexpected error response: %+v", response)
	}
}

func TestRequestIDRejectsHeaderLineBreaks(t *testing.T) {
	handler := withRequestID(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]string{"request_id": request.Context().Value(requestIDContextKey).(string)})
	}))
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	request.Header.Set(requestIDHeader, "invalid\r\nrequest")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	requestID := recorder.Header().Get(requestIDHeader)
	if requestID == "" || requestID == "invalid\r\nrequest" {
		t.Fatalf("unsafe request id was accepted: %q", requestID)
	}
}
