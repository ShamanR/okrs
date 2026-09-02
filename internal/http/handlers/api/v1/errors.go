package v1

import (
	"encoding/json"
	"errors"
	"net/http"

	"okrs/internal/http/httperr"
)

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string, fields map[string]string) {
	// Код и причина уходят в итоговую запись о запросе через обёртку ответа:
	// так ни одно из сотен мест вызова не нуждается ни в логгере, ни в контексте.
	// Тело ответа не подменяется, в отличие от простых writer'ов: здесь message —
	// отдельный параметр рядом с code, то есть заведомо предназначенный пользователю
	// текст, а не подставленный err.Error().
	_ = httperr.Record(w, code, errors.New(message))
	writeJSON(w, status, ErrorResponse{Error: ErrorDetail{Code: code, Message: message, Fields: fields}})
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	writeJSON(w, status, payload)
}

func WriteError(w http.ResponseWriter, status int, code, message string, fields map[string]string) {
	writeError(w, status, code, message, fields)
}
