package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"mgu-showcase/backend/internal/registry"
)

const maxBodyBytes = 1 << 20

type apiError struct {
	Code      string                `json:"code"`
	Message   string                `json:"message"`
	Fields    []registry.FieldError `json:"fields,omitempty"`
	RequestID string                `json:"request_id,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, msg string, fields ...registry.FieldError) {
	writeJSON(w, status, map[string]apiError{"error": {
		Code: code, Message: msg, Fields: fields, RequestID: requestIDFrom(r.Context()),
	}})
}

// writeDomainError переводит ошибки Registry в HTTP-ответы.
func (s *Server) writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *registry.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_failed", "Некорректные данные", ve.Fields...)
	case errors.Is(err, registry.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "not_found", "Элемент не найден")
	case errors.Is(err, registry.ErrConflict):
		writeError(w, r, http.StatusConflict, "conflict", "Элемент с таким slug уже существует")
	default:
		s.log.ErrorContext(r.Context(), "internal error", "err", err, "request_id", requestIDFrom(r.Context()))
		writeError(w, r, http.StatusInternalServerError, "internal", "Внутренняя ошибка сервера")
	}
}

// decodeJSON читает тело строго: неизвестные поля и мусор после JSON — ошибка.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, r, http.StatusBadRequest, "bad_request", "Некорректный JSON: "+err.Error())
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, r, http.StatusBadRequest, "bad_request", "В теле запроса должен быть один JSON-объект")
		return false
	}
	return true
}
