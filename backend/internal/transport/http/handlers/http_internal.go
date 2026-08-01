package handlers

import (
	"encoding/json"
	"net/http"

	api "llm-proxy/internal/transport/http"
)

var writeJSONError = api.WriteJSONError
var decodeJSON = api.DecodeJSON
var respondError = api.WriteJSONError
var ErrUnsupportedContentType = api.ErrUnsupportedContentType
var requirePathParams = api.RequirePathParams
var requirePathParamsMsg = api.RequirePathParamsMsg
var requireQueryParam = api.RequireQueryParam

type handlerError struct {
	Status  int
	Message string
	Err     error
}

func (e *handlerError) Error() string {
	return e.Message
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
