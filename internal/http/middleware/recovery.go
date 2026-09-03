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

				// Контекст пересобирается из обёртки ответа: recovery стоит снаружи
				// и видит тот request, который был до разрешения сессии и организации
				// (внутренние middleware создают копию запроса). Без этого запись
				// о панике несла бы только request_id, хотя организация и пользователь
				// уже известны и накоплены в обёртке.
				ctx := r.Context()
				if rec, ok := w.(*Recorder); ok {
					ctx = logging.WithScope(ctx, rec.scope)
				}

				logger.ErrorContext(ctx, "unhandled panic",
					slog.String(logging.KeyEvent, logging.EventHTTPPanic),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("panic", fmt.Sprint(rv)),
					slog.String("stack", string(debug.Stack())),
				)

				_ = httperr.Record(w, httperr.CodeForStatus(http.StatusInternalServerError),
					fmt.Errorf("panic: %v", rv))
				// Заголовок уже ушёл: дописать 500 нельзя, но и завершать ответ
				// штатно нельзя. Клиент получил бы усечённое тело со статусом
				// успеха и никак не отличил бы его от полного. Поэтому помечаем
				// запись как прерванную и обрываем поток: ErrAbortHandler —
				// договорённый способ закрыть соединение без собственного вывода
				// net/http. Запись о запросе при этом не теряется — access-log
				// пишет её из defer.
				if rec, ok := w.(*Recorder); ok && rec.wroteHeader {
					rec.markAborted()
					panic(http.ErrAbortHandler)
				}
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}()

			next.ServeHTTP(w, r)
		})
	}
}
