package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

var ErrUnsupportedContentType = errors.New("unsupported content type")

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

const maxBodySize = 4 * 1024 * 1024

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	if r.Header.Get("Content-Type") != "application/json" {
		return ErrUnsupportedContentType
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	return json.NewDecoder(r.Body).Decode(v)
}
