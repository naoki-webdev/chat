package main

import "net/http"

func (s *server) handleThreadRoots(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	user, ok := s.requireUser(writer, request)
	if !ok {
		return
	}
	limit, err := queryLimit(request)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	page, err := s.repository.ListThreadRoots(request.Context(), user.ID, limit)
	if err != nil {
		writeRepositoryError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}
