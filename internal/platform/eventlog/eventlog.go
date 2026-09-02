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
	eventbus.SubscribeAll(bus, SubscriberName, func(ctx context.Context, evs []event.Event) error {
		for i, ev := range evs {
			logger.InfoContext(recordContext(ctx, ev, i == 0), "domain event", attrsFor(ev)...)
		}
		return nil
	})
}

// recordContext собирает контекст для записи об одном событии.
//
// Контекст батча для этого не годится: при коалесценции шина сохраняет контекст
// только первого публикатора (см. Bus.run), а батч может смешивать события разных
// запросов и разных организаций. С общим контекстом второе событие получало бы
// tenant_id и actor_id первого — то есть аудит связывал бы действие с чужой
// организацией.
//
// request_id переносится только на первое событие батча: именно ему принадлежит
// сохранённый контекст (Bus.run строит батч, начиная с него, и порядок внутри
// подписчика FIFO). Остальным событиям чужой request_id не приписывается:
// отсутствие поля честнее неверного значения.
func recordContext(ctx context.Context, ev event.Event, ownsContext bool) context.Context {
	m := ev.Context()

	out := context.Background()
	if ownsContext {
		if id, ok := logging.RequestIDFromContext(ctx); ok {
			out = logging.WithRequestID(out, id)
		}
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
