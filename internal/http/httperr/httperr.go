// Package httperr собирает общее поведение ошибочных ответов API: передачу кода и
// технической причины в лог и запрет на её утечку в тело ответа.
//
// Пакет намеренно крошечный и не зависит ни от логгера, ни от middleware: связь
// с логированием держится на интерфейсе Recorder, который обёртка ответа из
// internal/http/middleware удовлетворяет структурно. Так обработчики не начинают
// зависеть от middleware, а middleware — от обработчиков.
package httperr

import (
	"encoding/json"
	"errors"
	"net/http"
)

// InternalErrorMessage — единственное, что пользователь узнаёт о внутренней ошибке.
// Техническая причина уходит в лог: там она нужна для расследования, а в ответе
// раскрывала бы устройство системы.
const InternalErrorMessage = "internal server error"

// Recorder принимает код и техническую причину ошибочного ответа.
//
// Реализуется обёрткой ответа, которую цепочка middleware передаёт обработчику.
// Именно этот интерфейс позволяет не менять сигнатуры error-writer'ов, у которых
// сотни мест вызова без доступа к логгеру и контексту.
type Recorder interface {
	RecordError(code string, cause error)
}

// Record сообщает обёртке ответа код и причину ошибки и сообщает, удалось ли это.
//
// Получается, когда запрос идёт через цепочку middleware. Вне цепочки — например,
// в юнит-тесте обработчика с httptest.ResponseRecorder — вызов возвращает false,
// и вызывающий код может записать ошибку сам, если ему есть чем.
func Record(w http.ResponseWriter, code string, cause error) bool {
	if rec, ok := w.(Recorder); ok {
		rec.RecordError(code, cause)
		return true
	}
	return false
}

// CodeForStatus даёт машиночитаемый код ошибки writer'ам, у которых своего кода нет.
func CodeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusMethodNotAllowed:
		return "method_not_allowed"
	case http.StatusConflict:
		return "conflict"
	case http.StatusUnprocessableEntity:
		return "unprocessable_entity"
	case http.StatusTooManyRequests:
		return "too_many_requests"
	case http.StatusInternalServerError:
		return "internal_error"
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	default:
		if status >= http.StatusInternalServerError {
			return "server_error"
		}
		return "client_error"
	}
}

// WriteJSON — общее тело простых error-writer'ов слоя API: {"error": "..."}.
//
// Тело подменяется на InternalErrorMessage ровно для 500. Именно с этим статусом
// по обработчикам рассыпаны вызовы вида WriteError(w, 500, err.Error()), каждый из
// которых отдавал наружу текст ошибки из слоёв usecase и store; подмена здесь
// закрывает их все разом, не трогая ни одного места вызова.
//
// Остальные 5xx не трогаем намеренно: 502 и 503 в этом приложении несут выверенный
// текст для администратора (почему не удалось подключиться к каналу уведомлений,
// почему снят ключ шифрования), уже очищенный от деталей адреса на стороне
// обработчика. Подмена и там превратила бы диагностику в «internal server error»
// и оставила бы администратора без единственного источника причины.
func WriteJSON(w http.ResponseWriter, status int, msg string) {
	Record(w, CodeForStatus(status), errors.New(msg))

	body := msg
	if status == http.StatusInternalServerError {
		body = InternalErrorMessage
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": body})
}
