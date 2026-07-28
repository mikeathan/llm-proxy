package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrUnsupportedContentType is returned when the request Content-Type is not application/json.
var ErrUnsupportedContentType = errors.New("unsupported content type")

// WriteJSONError writes a JSON error response with the given status code and message.
func WriteJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

const maxBodySize = 4 * 1024 * 1024

// DecodeJSON decodes a JSON request body into v, enforcing Content-Type and size limits.
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	if r.Header.Get("Content-Type") != "application/json" {
		return ErrUnsupportedContentType
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	return json.NewDecoder(r.Body).Decode(v)
}

// RequirePathParams extracts path parameters by key, returning 400 if any are empty.
// Values are returned in the same order as keys.
func RequirePathParams(w http.ResponseWriter, r *http.Request, keys ...string) ([]string, bool) {
	vals := make([]string, len(keys))
	var missing []string
	for i, k := range keys {
		v := r.PathValue(k)
		if v == "" {
			missing = append(missing, k)
		}
		vals[i] = v
	}
	if len(missing) > 0 {
		msg := strings.Join(missing, " and ") + " "
		if len(missing) == 1 {
			msg += "is required"
		} else {
			msg += "are required"
		}
		WriteJSONError(w, http.StatusBadRequest, msg)
		return nil, false
	}
	return vals, true
}

// RequireQueryParam extracts a required query parameter, returning 400 if empty.
func RequireQueryParam(w http.ResponseWriter, r *http.Request, key string) (string, bool) {
	v := r.URL.Query().Get(key)
	if v == "" {
		WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("%s query parameter is required", key))
		return "", false
	}
	return v, true
}

// RequirePathParamsMsg extracts path parameters by key, returning 400 with the
// single combined `msg` if ANY key is empty (regardless of which). Values are
// returned in the same order as keys. Used where the original API contract
// specified one combined "X and Y are required" message for multiple params.
func RequirePathParamsMsg(w http.ResponseWriter, r *http.Request, msg string, keys ...string) ([]string, bool) {
	vals := make([]string, len(keys))
	for i, k := range keys {
		v := r.PathValue(k)
		if v == "" {
			WriteJSONError(w, http.StatusBadRequest, msg)
			return nil, false
		}
		vals[i] = v
	}
	return vals, true
}
