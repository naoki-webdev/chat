package main

import (
	"errors"
	"net/http"
	"strings"
)

func (s *server) handleChannels(writer http.ResponseWriter, request *http.Request) {
	user, ok := s.requireUser(writer, request)
	if !ok {
		return
	}
	switch request.Method {
	case http.MethodGet:
		channels, cursor, err := s.repository.ListChannels(request.Context(), user.ID)
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"channels": channels, "cursor": cursor})
	case http.MethodPost:
		var payload channelRequest
		if !decodeJSON(writer, request, &payload) {
			return
		}
		channel, err := s.repository.CreateChannel(request.Context(), user.ID, payload)
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		writeJSON(writer, http.StatusCreated, channel)
	default:
		methodNotAllowed(writer)
	}
}

func (s *server) handleChannelRoutes(writer http.ResponseWriter, request *http.Request) {
	user, ok := s.requireUser(writer, request)
	if !ok {
		return
	}
	segments := strings.Split(strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/channels/"), "/"), "/")
	if len(segments) != 2 || (segments[1] != "messages" && segments[1] != "read" && segments[1] != "summary") {
		http.NotFound(writer, request)
		return
	}
	channelID := segments[0]
	exists, err := s.repository.HasChannel(request.Context(), channelID)
	if err != nil {
		writeRepositoryError(writer, err)
		return
	}
	if !exists {
		http.NotFound(writer, request)
		return
	}
	if !s.requireChannelMember(writer, request, user, channelID) {
		return
	}
	if segments[1] == "read" {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		cursor, err := s.repository.MarkChannelRead(request.Context(), user.ID, channelID)
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"cursor": cursor, "unread": 0})
		return
	}
	if segments[1] == "summary" {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		summary, err := s.generateChannelSummary(request.Context(), user.ID, channelID)
		if err != nil {
			var validation validationError
			if errors.As(err, &validation) {
				writeRepositoryError(writer, err)
				return
			}
			writeError(writer, http.StatusBadGateway, "AI要約の生成に失敗しました。しばらくしてから再試行してください。")
			return
		}
		writeJSON(writer, http.StatusOK, summary)
		return
	}
	switch request.Method {
	case http.MethodGet:
		limit, err := queryLimit(request)
		if err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		page, err := s.repository.ListMessagePage(request.Context(), channelID, request.URL.Query().Get("before"), limit)
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, page)
	case http.MethodPost:
		var payload messageRequest
		if !decodeJSON(writer, request, &payload) {
			return
		}
		message, record, err := s.repository.CreateMessage(request.Context(), channelID, user.ID, payload)
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		s.broadcast(record.Event)
		if shouldInvokeAI(channelID, payload.Body) {
			go s.startAIReply(channelID, user.ID, message)
		}
		writeJSON(writer, http.StatusCreated, message)
	default:
		methodNotAllowed(writer)
	}
}
