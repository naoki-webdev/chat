package main

import "net/http"

func (s *server) handleUsers(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	if _, ok := s.requireUser(writer, request); !ok {
		return
	}
	users, err := s.repository.ListUsers(request.Context())
	if err != nil {
		writeRepositoryError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"users": users})
}
