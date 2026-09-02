// Package eventlog логирует факт каждого опубликованного доменного события.
//
// Пакет отдельный, а не часть logging, чтобы платформенный логгер не начал зависеть
// от доменных типов: здесь встречаются шина, события и схема лог-записи, и только
// здесь.
package eventlog

import (
	"context"
	"log/slog"

	"okrs/internal/core/event"
	"okrs/internal/platform/eventbus"
	"okrs/internal/platform/logging"
)

// SubscriberName — имя подписки на шине. Оно же попадает в записи шины о потерях.
const SubscriberName = "logging"

// Subscribe подписывает логирование на все доменные события.
//
// SubscribeAll, а не подписка на каждый тип: спецификация требует, чтобы новый тип
// события покрывался автоматически, а перечисление 21 типа руками рано или поздно
// пропустило бы 22-й.
//
// Режим асинхронный: логирование не должно ни задерживать мутацию, ни ронять её.
// Переполнение буфера шина фиксирует сама, записью event_dropped.
//
// Подписчик не обращается к базе данных и не имеет для этого зависимостей: он
// логирует только то, что уже лежит в событии. Обогащение названиями сущностей
// стоило бы запроса на каждое событие, то есть N+1 на пути каждой мутации, и вдобавок
// внесло бы в лог пользовательский текст, которого там быть не должно. Сопоставить
// идентификатор с названием — задача того, кто расследует инцидент.
func Subscribe(bus *eventbus.Bus, logger *slog.Logger) {
	// SubscribeAllWithContext, а не SubscribeAll: запись обязана ссылаться на запрос,
	// породивший именно это событие, а батч после коалесценции может смешивать
	// публикации разных запросов.
	eventbus.SubscribeAllWithContext(bus, SubscriberName, func(_ context.Context, evs []eventbus.Delivered) error {
		for _, d := range evs {
			logger.InfoContext(recordContext(d.Ctx, d.Event), "domain event", attrsFor(d.Event)...)
		}
		return nil
	})
}

// recordContext собирает контекст для записи об одном событии.
//
// ctx — контекст публикации именно этого события: шина доносит его через
// коалесценцию (см. eventbus.Delivered). Из него берётся только request_id — связь
// с породившим запросом.
//
// Организация и действующий пользователь берутся из Meta события, а не из контекста:
// событие — авторитетный источник того, чьё действие оно описывает.
func recordContext(ctx context.Context, ev event.Event) context.Context {
	m := ev.Context()

	out := context.Background()
	if id, ok := logging.RequestIDFromContext(ctx); ok {
		out = logging.WithRequestID(out, id)
	}
	return logging.WithScope(out, logging.Scope{
		TenantID: m.Scope.TenantID,
		ActorID:  m.ActorID,
		TeamID:   m.TeamID,
		PeriodID: m.PeriodID,
	})
}

// attrsFor собирает поля записи о событии: тип, время наступления и извлечённые
// идентификаторы сущностей.
//
// Организацию, действующего пользователя, команду и период здесь НЕ добавляем:
// их проставляет обработчик логгера из контекста, собранного recordContext.
// Добавлять их в обоих местах значило бы дублирующие ключи в JSON, где побеждает
// последний, — именно так аудит и получал чужую организацию.
func attrsFor(ev event.Event) []any {
	attrs := []any{
		slog.String(logging.KeyEvent, logging.EventDomainEvent),
		slog.String("kind", string(ev.Kind())),
	}
	if at := ev.Context().OccurredAt; !at.IsZero() {
		attrs = append(attrs, slog.Time("occurred_at", at))
	}

	// Поля самого события отбираются по типу: идентификаторы, числа и флаги
	// попадают в лог, пользовательский текст — нет. Встроенная Meta не
	// раскрывается: она структура, а её поля уже попали в контекст.
	for _, a := range logging.StructAttrs(ev) {
		attrs = append(attrs, a)
	}
	return attrs
}
