package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

var ErrUnsupportedContentType = errors.New("unsupported content type")

type handlerError struct {
	Status  int
	Message string
	Err     error
}

func (e *handlerError) Error() string {
	return e.Message
}

func decodeJSON(r *http.Request, v any) error {
	if r.Header.Get("Content-Type") != "application/json" {
		return ErrUnsupportedContentType
	}
	return json.NewDecoder(r.Body).Decode(v)
}

func respondJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func getTraceID(conversationID, remoteAddr string) string {
	if conversationID != "" {
		return conversationID
	}
	return remoteAddr
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
