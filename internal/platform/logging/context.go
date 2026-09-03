package logging

import (
	"context"
	"log/slog"
)

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyScope
	ctxKeyLogger
)

// Scope — контекст действия, попадающий в каждую запись, порождённую его
// обработкой. Намеренно объявлен как плоская структура, а не как domain.TenantScope:
// платформенный логгер не должен зависеть от доменных типов, а вызывающий код
// и так знает, как заполнить четыре числа.
//
// Нулевое значение поля означает «неизвестно» и в запись не попадает — подставлять
// вместо отсутствующего контекста ноль значило бы выдавать выдумку за факт.
type Scope struct {
	TenantID int64
	ActorID  int64
	TeamID   *int64
	PeriodID *int64
}

// WithRequestID кладёт идентификатор запроса в контекст.
func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyRequestID, id)
}

// RequestIDFromContext возвращает идентификатор запроса, если он установлен.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	id, ok := ctx.Value(ctxKeyRequestID).(string)
	return id, ok && id != ""
}

// WithScope кладёт контекст действия в контекст запроса.
func WithScope(ctx context.Context, s Scope) context.Context {
	return context.WithValue(ctx, ctxKeyScope, s)
}

// ScopeFromContext возвращает контекст действия, если он установлен.
func ScopeFromContext(ctx context.Context) (Scope, bool) {
	if ctx == nil {
		return Scope{}, false
	}
	s, ok := ctx.Value(ctxKeyScope).(Scope)
	return s, ok
}

// WithLogger кладёт логгер в контекст — для мест, у которых есть ctx, но нет
// внедрённого логгера.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	if l == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyLogger, l)
}

// FromContext возвращает логгер из контекста, либо slog.Default().
func FromContext(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if l, ok := ctx.Value(ctxKeyLogger).(*slog.Logger); ok && l != nil {
			return l
		}
	}
	return slog.Default()
}

// ContextAttrs собирает атрибуты, которые контекст добавляет к записи.
func ContextAttrs(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}
	var attrs []slog.Attr
	if id, ok := RequestIDFromContext(ctx); ok {
		attrs = append(attrs, slog.String(KeyRequestID, id))
	}
	if s, ok := ScopeFromContext(ctx); ok {
		if s.TenantID != 0 {
			attrs = append(attrs, slog.Int64(KeyTenantID, s.TenantID))
		}
		if s.ActorID != 0 {
			attrs = append(attrs, slog.Int64(KeyActorID, s.ActorID))
		}
		if s.TeamID != nil {
			attrs = append(attrs, slog.Int64(KeyTeamID, *s.TeamID))
		}
		if s.PeriodID != nil {
			attrs = append(attrs, slog.Int64(KeyPeriodID, *s.PeriodID))
		}
	}
	return attrs
}

// contextHandler добавляет в запись контекстные поля и гарантирует наличие поля
// event.
//
// Обогащение сделано на уровне обработчика, а не вызывающего кода, потому что
// slog.Handler.Handle получает context: иначе request_id пришлось бы протаскивать
// через сигнатуры сервисов и хранилищ — ровно то протекание сквозной заботы между
// слоями, которого архитектура избегает.
//
// Цена решения: поля появляются только у вызовов с контекстом (InfoContext,
// WarnContext, ErrorContext). Для записей вне обработки запроса — например,
// жизненного цикла приложения — контекста запроса и не существует.
type contextHandler struct {
	base slog.Handler
	// hasEvent запоминает, что поле event уже задано через logger.With: такие
	// атрибуты живут в состоянии обработчика и в самой записи не видны.
	hasEvent bool
}

func newContextHandler(base slog.Handler) slog.Handler {
	return &contextHandler{base: base}
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	hasEvent := h.hasEvent
	for _, a := range attrs {
		if a.Key == KeyEvent {
			hasEvent = true
			break
		}
	}
	return &contextHandler{base: h.base.WithAttrs(attrs), hasEvent: hasEvent}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{base: h.base.WithGroup(name), hasEvent: h.hasEvent}
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := ContextAttrs(ctx)

	needEvent := !h.hasEvent
	if needEvent {
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == KeyEvent {
				needEvent = false
				return false
			}
			return true
		})
	}
	if needEvent {
		attrs = append(attrs, slog.String(KeyEvent, EventUnspecified))
	}

	if len(attrs) == 0 {
		return h.base.Handle(ctx, r)
	}
	// Clone обязателен: запас атрибутов записи может разделяться с копиями,
	// и дописывание в него без клонирования испортило бы чужую запись.
	r = r.Clone()
	r.AddAttrs(attrs...)
	return h.base.Handle(ctx, r)
}
