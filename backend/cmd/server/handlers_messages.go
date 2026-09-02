package main

import (
	"net/http"
	"strings"
)

func (s *server) handleEvents(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	user, ok := s.requireUser(writer, request)
	if !ok {
		return
	}
	after, err := cursorValue(request.URL.Query().Get("after"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid cursor")
		return
	}
	limit, err := queryLimit(request)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	page, err := s.repository.ListEvents(request.Context(), user.ID, after, limit)
	if err != nil {
		writeRepositoryError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func (s *server) handleMessageRoutes(writer http.ResponseWriter, request *http.Request) {
	user, ok := s.requireUser(writer, request)
	if !ok {
		return
	}
	segments := strings.Split(strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/messages/"), "/"), "/")
	if len(segments) == 2 && segments[1] == "replies" {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer, http.MethodGet)
			return
		}
		if _, ok := s.requireMessageMember(writer, request, user, segments[0]); !ok {
			return
		}
		limit, err := queryLimit(request)
		if err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		page, err := s.repository.ListThreadPage(request.Context(), segments[0], request.URL.Query().Get("before"), limit)
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, page)
		return
	}
	if len(segments) == 2 && segments[1] == "reactions" {
		messageID := segments[0]
		if _, ok := s.requireMessageMember(writer, request, user, messageID); !ok {
			return
		}
		var message Message
		var record EventRecord
		var err error
		switch request.Method {
		case http.MethodPost:
			var payload reactionRequest
			if !decodeJSON(writer, request, &payload) {
				return
			}
			message, record, err = s.repository.AddReaction(request.Context(), messageID, user.ID, payload.Emoji)
		case http.MethodDelete:
			message, record, err = s.repository.RemoveReaction(request.Context(), messageID, user.ID, request.URL.Query().Get("emoji"))
		default:
			methodNotAllowed(writer, http.MethodPost, http.MethodDelete)
			return
		}
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		if record.Event.Type != "" {
			s.broadcast(record.Event)
		}
		writeJSON(writer, http.StatusOK, message)
		return
	}
	if len(segments) != 1 || segments[0] == "" {
		http.NotFound(writer, request)
		return
	}
	messageID := segments[0]
	if _, ok := s.requireMessageMember(writer, request, user, messageID); !ok {
		return
	}
	switch request.Method {
	case http.MethodPatch:
		var payload updateMessageRequest
		if !decodeJSON(writer, request, &payload) {
			return
		}
		message, record, err := s.repository.UpdateMessage(request.Context(), messageID, user.ID, payload.Body)
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		s.broadcast(record.Event)
		writeJSON(writer, http.StatusOK, message)
	case http.MethodDelete:
		channelID, record, err := s.repository.DeleteMessage(request.Context(), messageID, user.ID)
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		record.Event.ChannelID = channelID
		s.broadcast(record.Event)
		writeJSON(writer, http.StatusOK, map[string]string{"message_id": messageID})
	default:
		methodNotAllowed(writer, http.MethodPatch, http.MethodDelete)
	}
}
