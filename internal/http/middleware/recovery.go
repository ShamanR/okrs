package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"okrs/internal/http/httperr"
	"okrs/internal/platform/logging"
)

// Recovery перехватывает необработанную панику, логирует её со стеком и отвечает 500.
//
// Своя реализация, а не chi/middleware.Recoverer: последний печатает многострочный
// неструктурированный дамп прямо в stdout, что разрывает построчный JSON и делает
// запись неразбираемой системой сбора логов.
//
// Монтируется ВНУТРИ AccessLog: паника превращается здесь в ответ 500, и внешний
// access-log видит статус 500 и пишет обычную запись о завершении запроса. При
// обратном порядке паника не оставила бы записи о самом запросе.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rv := recover()
				if rv == nil {
					return
				}
				// http.ErrAbortHandler — договорённый способ оборвать соединение
				// молча; перехватывать его означало бы превращать намеренный
				// разрыв в ложную ошибку сервера.
				if rv == http.ErrAbortHandler {
					panic(rv)
				}

				logger.ErrorContext(r.Context(), "unhandled panic",
					slog.String(logging.KeyEvent, logging.EventHTTPPanic),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("panic", fmt.Sprint(rv)),
					slog.String("stack", string(debug.Stack())),
				)

				_ = httperr.Record(w, httperr.CodeForStatus(http.StatusInternalServerError),
					fmt.Errorf("panic: %v", rv))
				// Если заголовок уже ушёл, дописать 500 нельзя — но запись о панике
				// уже сделана, и это главное.
				if rec, ok := w.(*Recorder); ok && rec.wroteHeader {
					return
				}
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}()

			next.ServeHTTP(w, r)
		})
	}
}
