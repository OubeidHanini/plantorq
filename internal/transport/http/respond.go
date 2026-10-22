package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/OubeidHanini/plantorq/internal/app/platform"
)

type errorBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError maps application errors to HTTP status codes in one place.
// Unknown errors become a generic 500 so internals never leak to clients.
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, msg := http.StatusInternalServerError, "internal error"
	switch {
	case errors.Is(err, platform.ErrInvalid):
		status, msg = http.StatusBadRequest, err.Error()
	case errors.Is(err, platform.ErrUnauthorized):
		status, msg = http.StatusUnauthorized, "invalid credentials"
	case errors.Is(err, platform.ErrForbidden):
		status, msg = http.StatusForbidden, "insufficient role"
	case errors.Is(err, platform.ErrNotFound):
		status, msg = http.StatusNotFound, "not found"
	default:
		s.Log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	}
	writeJSON(w, status, errorBody{msg})
}

// decode reads a JSON body, rejecting unknown fields and oversized payloads.
func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errors.Join(platform.ErrInvalid, errors.New("malformed JSON body"))
	}
	return nil
}
