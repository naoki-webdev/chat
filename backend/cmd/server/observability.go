package main

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const requestIDHeader = "X-Request-ID"

type requestContextKey string

const (
	requestIDContextKey requestContextKey = "request_id"
	userIDContextKey    requestContextKey = "user_id"
)

var observabilityLogger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := strings.TrimSpace(request.Header.Get(requestIDHeader))
		if !validRequestID(requestID) {
			requestID = randomID()
		}
		writer.Header().Set(requestIDHeader, requestID)
		contextWithID := context.WithValue(request.Context(), requestIDContextKey, requestID)
		*request = *request.WithContext(contextWithID)
		next.ServeHTTP(writer, request)
	})
}

func validRequestID(requestID string) bool {
	if requestID == "" || len(requestID) > 128 {
		return false
	}
	for _, character := range requestID {
		if character < 0x21 || character > 0x7e {
			return false
		}
	}
	return true
}

func setAuthenticatedUser(request *http.Request, user User) {
	*request = *request.WithContext(context.WithValue(request.Context(), userIDContextKey, user.ID))
}

func requestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey).(string)
	return requestID
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (writer *loggingResponseWriter) WriteHeader(status int) {
	if writer.wroteHeader {
		return
	}
	writer.status = status
	writer.wroteHeader = true
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *loggingResponseWriter) Write(payload []byte) (int, error) {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(payload)
}

func (writer *loggingResponseWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func (writer *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := writer.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("http response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (writer *loggingResponseWriter) Flush() {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		responseWriter := &loggingResponseWriter{ResponseWriter: writer}
		next.ServeHTTP(responseWriter, request)

		requestID, _ := request.Context().Value(requestIDContextKey).(string)
		userID, _ := request.Context().Value(userIDContextKey).(string)
		status := responseWriter.status
		if status == 0 {
			status = http.StatusOK
		}
		observabilityLogger.Info("http_request",
			"request_id", requestID,
			"user_id", userID,
			"method", request.Method,
			"route", request.URL.Path,
			"status", status,
			"duration_ms", float64(time.Since(started).Microseconds())/1000,
		)
	})
}
