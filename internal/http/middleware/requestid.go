package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"okrs/internal/platform/logging"
)

// RequestIDHeader переносит идентификатор запроса и внутрь, и наружу: входящее
// значение подхватывается, исходящее возвращается клиенту, чтобы пользователь мог
// назвать его в обращении, а поддержка — найти по нему всю цепочку записей.
const RequestIDHeader = "X-Request-Id"

// maxRequestIDLen ограничивает принимаемое извне значение. Без ограничения клиент
// мог бы задать идентификатор неограниченной длины и кардинальности, раздув индекс
// в системе логов.
const maxRequestIDLen = 128

// RequestID кладёт идентификатор запроса в контекст и возвращает его в ответе.
//
// Должен монтироваться первым: всё, что логируется дальше по цепочке, ссылается на
// этот идентификатор.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if !validRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(logging.WithRequestID(r.Context(), id)))
	})
}

// validRequestID проверяет пришедшее извне значение.
//
// Проверка обязательна, а не гигиенична: значение попадает в лог-запись, и без неё
// клиент мог бы прислать перевод строки, разорвав JSON-строку и подделав соседние
// записи в Kibana.
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// На поддерживаемых платформах crypto/rand не отказывает, но обслуживание
		// запроса не должно зависеть от этого: время даёт значение, уникальное
		// в пределах наносекунды.
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}
