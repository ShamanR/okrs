package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/httperr"
	"okrs/internal/httproute"
	"okrs/internal/platform/logging"
)

// Recorder оборачивает http.ResponseWriter и накапливает всё, что нужно итоговой
// записи о запросе: статус, код и причину ошибки, установленную личность.
//
// Личность накапливается здесь, а не читается из контекста в конце запроса, потому
// что middleware аутентификации кладёт пользователя в НОВЫЙ request (r.WithContext)
// и передаёт его вниз — внешний r, который видит access-log, остаётся прежним.
// Обёртка же передаётся вниз указателем, поэтому она видит то, что установили
// внутренние слои.
type Recorder struct {
	http.ResponseWriter

	status      int
	wroteHeader bool

	errCode  string
	errCause error

	user  *domain.User
	scope logging.Scope

	// aborted отмечает ответ, оборванный паникой уже после отправки заголовка.
	// Статус на проводе к этому моменту уже отправлен и обычно равен 200,
	// поэтому без этого флага запись о запросе выглядела бы чистым успехом.
	aborted bool
}

// markAborted фиксирует, что ответ не был доведён до конца.
func (rec *Recorder) markAborted() { rec.aborted = true }

func (rec *Recorder) WriteHeader(status int) {
	if !rec.wroteHeader {
		rec.status = status
		rec.wroteHeader = true
	}
	rec.ResponseWriter.WriteHeader(status)
}

// Write фиксирует неявный 200: обработчик, начавший писать тело без WriteHeader,
// иначе оставил бы статус недостоверным.
func (rec *Recorder) Write(b []byte) (int, error) {
	if !rec.wroteHeader {
		rec.status = http.StatusOK
		rec.wroteHeader = true
	}
	return rec.ResponseWriter.Write(b)
}

// Unwrap отдаёт исходный writer http.ResponseController, чтобы обёртка не отбирала
// у обработчика Flush и Hijack.
func (rec *Recorder) Unwrap() http.ResponseWriter { return rec.ResponseWriter }

// RecordError запоминает код и техническую причину ошибочного ответа. Побеждает
// первый вызов: именно он породил ответ, который увидел клиент.
//
// Реализует httperr.Recorder: общие error-writer'ы делают assertion переданного
// им writer'а на этот интерфейс, что и позволяет не менять их сигнатуры.
func (rec *Recorder) RecordError(code string, cause error) {
	if rec.errCode == "" {
		rec.errCode = code
	}
	if rec.errCause == nil {
		rec.errCause = cause
	}
}

// Status возвращает статус ответа. Для тестов и внутренних потребителей пакета.
func (rec *Recorder) Status() int { return rec.status }

// AccessLog пишет ровно одну запись на запрос — она же и есть запись об ошибке.
//
// Одна запись вместо двух (access + error) выбрана намеренно: спецификация требует
// и того, чтобы ни один ошибочный ответ не остался без следа, и того, чтобы одна
// ошибка не порождала дублей. Обе гарантии выполняются по построению, когда запись
// физически одна и её уровень выводится из статуса.
func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &Recorder{ResponseWriter: w, status: http.StatusOK}

			// Логгер кладётся в контекст, чтобы его могли взять те middleware,
			// чьи сигнатуры заданы типом func(http.Handler) http.Handler и не имеют
			// места для зависимости — в первую очередь гейты доступа в internal/auth.
			r = r.WithContext(logging.WithLogger(r.Context(), logger))

			// Запись отложена: паника, оборвавшая уже начатый ответ, проходит
			// сквозь этот кадр наружу, и без defer запрос не оставил бы следа вовсе.
			defer func() {
				attrs := []any{
					slog.String(logging.KeyEvent, logging.EventHTTPRequest),
					slog.String("method", r.Method),
					// Шаблон маршрута, а не конкретный путь: параметр пути может
					// быть учётными данными (/invite/{token} — действующий токен
					// приглашения), а редакция по имени ключа произвольную строку
					// не маскирует. Шаблон ко всему прочему и агрегируется.
					slog.String("path", httproute.Pattern(r)),
					slog.Int("status", rec.status),
					slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				}
				if rec.aborted {
					attrs = append(attrs, slog.Bool("aborted", true))
				}
				if rec.user != nil {
					attrs = append(attrs,
						slog.Bool("authenticated", true),
						slog.String("provider", rec.user.Provider),
						slog.Bool("is_system_admin", rec.user.IsSystemAdmin),
					)
				} else {
					attrs = append(attrs, slog.Bool("authenticated", false))
				}
				// Код ошибки обязателен для любого ошибочного ответа, а часть
				// обработчиков отвечает напрямую через http.Error, мимо общих
				// error-writer'ов, и код не записывает. Выводим его из статуса:
				// выборка по error_code не должна молча терять часть отказов.
				if code := rec.errCode; code != "" {
					attrs = append(attrs, slog.String("error_code", code))
				} else if rec.status >= http.StatusBadRequest {
					attrs = append(attrs, slog.String("error_code", httperr.CodeForStatus(rec.status)))
				}
				if rec.errCause != nil {
					attrs = append(attrs, slog.String("err", rec.errCause.Error()))
				}

				// Контекст пересобирается с накопленным scope: идентификаторы
				// организации и пользователя добавит обработчик логгера, теми же
				// ключами, что и у любой другой записи.
				ctx := logging.WithScope(r.Context(), rec.scope)
				level := levelForStatus(rec.status)
				if rec.aborted {
					// На проводе остался статус успеха, но ответ не состоялся.
					level = slog.LevelError
				}
				logger.Log(ctx, level, "request", attrs...)
			}()

			next.ServeHTTP(rec, r)
		})
	}
}

// levelForStatus: ответ об ошибке сервера — error, об ошибке клиента —
// предупреждение, всё остальное — информация.
func levelForStatus(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

// LogContext переносит установленную личность и организацию в контекст логирования
// и в обёртку ответа.
//
// Монтируется после каждого шага, который что-то из этого разрешает (сессия,
// резолв организации): вызовы идемпотентны и только дополняют накопленное, поэтому
// повторное монтирование безопасно.
func LogContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		scope, _ := logging.ScopeFromContext(ctx)

		user := auth.UserFromContext(ctx)
		if user != nil {
			scope.ActorID = user.ID
		}
		if ts, ok := auth.TenantScopeFromContext(ctx); ok {
			scope.TenantID = ts.TenantID
		}
		if rec, ok := w.(*Recorder); ok {
			if user != nil {
				rec.user = user
			}
			rec.scope = scope
		}

		next.ServeHTTP(w, r.WithContext(logging.WithScope(ctx, scope)))
	})
}
