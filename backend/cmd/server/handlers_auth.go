package main

import "net/http"

func (s *server) handleRegister(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		methodNotAllowed(writer)
		return
	}
	var payload registerRequest
	if !decodeJSON(writer, request, &payload) {
		return
	}
	user, err := s.repository.RegisterUser(request.Context(), payload)
	if err != nil {
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
		methodNotAllowed(writer)
		return
	}
	var payload loginRequest
	if !decodeJSON(writer, request, &payload) {
		return
	}
	user, err := s.repository.AuthenticateUser(request.Context(), payload.Email, payload.Password)
	if err != nil {
		writeError(writer, http.StatusUnauthorized, "email or password is incorrect")
		return
	}
	if !s.startSession(writer, request, user) {
		return
	}
	writeJSON(writer, http.StatusOK, map[string]User{"user": user})
}

func (s *server) handleLogout(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		methodNotAllowed(writer)
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
	if request.Method != http.MethodGet {
		methodNotAllowed(writer)
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
