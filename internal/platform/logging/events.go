package logging

import (
	"context"
	"log/slog"
	"time"
)

// Значения поля event — стабильные машиночитаемые идентификаторы типов записей.
//
// Они вынесены в константы, а не написаны литералами по месту, потому что это
// внешний контракт: на них строятся фильтры, дашборды и алерты в Kibana.
// Переименование значения ломает потребителя так же, как переименование поля API,
// поэтому добавлять новое значение можно свободно, а менять существующее — нет.
const (
	// EventUnspecified проставляется автоматически записи, в которой автор не
	// указал тип. Это заметный маркер пропуска, а не значение по умолчанию:
	// его наличие в логе означает, что запись нужно дополнить.
	EventUnspecified = "unspecified"

	EventConfigInvalid = "config_invalid"

	EventHTTPRequest = "http_request"
	EventHTTPPanic   = "http_panic"

	EventDomainEvent  = "domain_event"
	EventEventDropped = "event_dropped"

	EventAuthLogin     = "auth_login"
	EventAuthLogout    = "auth_logout"
	EventAuthFailed    = "auth_failed"
	EventAuthzDenied   = "authz_denied"
	EventAccessChanged = "access_changed"

	EventAppStart    = "app_start"
	EventAppReady    = "app_ready"
	EventAppShutdown = "app_shutdown"
	EventMigration   = "migration"

	EventBackgroundTask = "background_task"
	EventExternalCall   = "external_call"
)

// AccessChanged фиксирует изменение прав доступа: смену роли участника, выдачу или
// отзыв прав, изменение административных настроек.
//
// Хелпер, а не голый вызов логгера по месту, потому что таких мест десяток в разных
// пакетах: единый набор полей — условие того, чтобы «кто и когда менял доступ»
// собиралось в Kibana одним запросом, а не сшивалось вручную.
//
// Логгер берётся из контекста: обработчики административных ручек логгера не
// получают, а прокидывать его в каждый ради одной записи означало бы менять их
// конструкторы.
//
// Идентификатор действующего пользователя и организации проставит обработчик
// логгера из контекста запроса; здесь передаётся только то, что изменилось.
func AccessChanged(ctx context.Context, action string, attrs ...any) {
	FromContext(ctx).InfoContext(ctx, "access changed",
		append([]any{
			slog.String(KeyEvent, EventAccessChanged),
			slog.String("action", action),
		}, attrs...)...)
}

// ExternalCall фиксирует исход обращения во внешнюю систему: цель, длительность
// и, при неуспехе, причину.
//
// Уровень выводится из исхода: неуспешное обращение — error, успешное — info.
//
// Вызывается на границе приложения, а не внутри реализации канала: канал — это
// подключаемый шов, который другая сборка может заменить своим, и он не должен
// зависеть от схемы логирования этого приложения. Повторы, которые реализация
// канала делает внутри себя, здесь поэтому не видны; номер попытки передаётся
// через extra тем вызывающим кодом, который повторяет обращение сам.
//
// Ни адрес подключения с учётными данными, ни токен, ни адресат сюда передавать
// нельзя: target — это ИМЯ внешней системы, а не её адрес.
func ExternalCall(ctx context.Context, target string, took time.Duration, err error, extra ...any) {
	attrs := append([]any{
		slog.String(KeyEvent, EventExternalCall),
		slog.String("target", target),
		slog.Int64("duration_ms", took.Milliseconds()),
	}, extra...)

	logger := FromContext(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "external call failed",
			append(attrs, slog.String("outcome", "failed"), slog.String("err", err.Error()))...)
		return
	}
	logger.InfoContext(ctx, "external call succeeded",
		append(attrs, slog.String("outcome", "ok"))...)
}

// AllEvents перечисляет все объявленные идентификаторы типов записей. Существует
// ради теста на уникальность: дубль значения молча слил бы в Kibana два разных
// типа записей в один.
func AllEvents() []string {
	return []string{
		EventUnspecified,
		EventConfigInvalid,
		EventHTTPRequest,
		EventHTTPPanic,
		EventDomainEvent,
		EventEventDropped,
		EventAuthLogin,
		EventAuthLogout,
		EventAuthFailed,
		EventAuthzDenied,
		EventAccessChanged,
		EventAppStart,
		EventAppReady,
		EventAppShutdown,
		EventMigration,
		EventBackgroundTask,
		EventExternalCall,
	}
}
