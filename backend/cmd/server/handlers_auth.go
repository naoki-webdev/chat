package main

import (
	"errors"
	"net/http"
)

func (s *server) handleRegister(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		methodNotAllowed(writer, http.MethodPost)
		return
	}
	var payload registerRequest
	if !decodeJSON(writer, request, &payload) {
		return
	}
	keys := authRateLimitKeys(request, payload.Email)
	allowed, limiterErr := s.allowAuth(request.Context(), keys...)
	if limiterErr != nil {
		writeRepositoryError(writer, limiterErr)
		return
	}
	if !allowed {
		writeAuthRateLimitError(writer)
		return
	}
	user, err := s.repository.RegisterUser(request.Context(), payload)
	if err != nil {
		writeRepositoryError(writer, err)
		return
	}
	if err := s.resetAuth(request.Context(), keys...); err != nil {
		writeRepositoryError(writer, err)
		return
	}
	if !s.startSession(writer, request, user) {
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]User{"user": user})
}

func (s *server) handleLogin(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		methodNotAllowed(writer, http.MethodPost)
		return
	}
	var payload loginRequest
	if !decodeJSON(writer, request, &payload) {
		return
	}
	keys := authRateLimitKeys(request, payload.Email)
	allowed, limiterErr := s.allowAuth(request.Context(), keys...)
	if limiterErr != nil {
		writeRepositoryError(writer, limiterErr)
		return
	}
	if !allowed {
		writeAuthRateLimitError(writer)
		return
	}
	user, err := s.repository.AuthenticateUser(request.Context(), payload.Email, payload.Password)
	if err != nil {
		if !errors.Is(err, ErrUnauthorized) {
			writeRepositoryError(writer, err)
			return
		}
		writeErrorCode(writer, http.StatusUnauthorized, "INVALID_CREDENTIALS", "email or password is incorrect")
		return
	}
	if err := s.resetAuth(request.Context(), keys...); err != nil {
		writeRepositoryError(writer, err)
		return
	}
	if !s.startSession(writer, request, user) {
		return
	}
	writeJSON(writer, http.StatusOK, map[string]User{"user": user})
}

func (s *server) handleLogout(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		methodNotAllowed(writer, http.MethodPost)
		return
	}
	if cookie, err := request.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if err := s.repository.DeleteSession(request.Context(), cookie.Value); err != nil {
			writeRepositoryError(writer, err)
			return
		}
	}
	http.SetCookie(writer, expiredSessionCookie())
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleCurrentUser(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodPatch {
		user, ok := s.requireUser(writer, request)
		if !ok {
			return
		}
		var payload updateProfileRequest
		if !decodeJSON(writer, request, &payload) {
			return
		}
		updated, err := s.repository.UpdateUserProfile(request.Context(), user.ID, payload)
		if err != nil {
			writeRepositoryError(writer, err)
			return
		}
		s.broadcast(realtimeEvent{
			Type:                "user.updated",
			ChannelID:           "*",
			ActorID:             updated.ID,
			ActorName:           updated.Name,
			ActorHandle:         updated.Handle,
			ActorInitials:       updated.Initials,
			ActorColor:          updated.Color,
			PreviousActorHandle: user.Handle,
		})
		writeJSON(writer, http.StatusOK, map[string]User{"user": updated})
		return
	}
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet, http.MethodPatch)
		return
	}
	user, ok := s.requireUser(writer, request)
	if !ok {
		return
	}
	writeJSON(writer, http.StatusOK, map[string]User{"user": user})
}

func (s *server) startSession(writer http.ResponseWriter, request *http.Request, user User) bool {
	token, err := s.repository.CreateSession(request.Context(), user.ID)
	if err != nil {
		writeRepositoryError(writer, err)
		return false
	}
	http.SetCookie(writer, sessionCookie(token))
	return true
}
