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
		for _, ev := range evs {
			logger.InfoContext(ctx, "domain event", attrsFor(ev)...)
		}
		return nil
	})
}

// attrsFor собирает поля записи о событии: тип, контекст из Meta и извлечённые
// идентификаторы.
func attrsFor(ev event.Event) []any {
	m := ev.Context()

	attrs := []any{
		slog.String(logging.KeyEvent, logging.EventDomainEvent),
		slog.String("kind", string(ev.Kind())),
	}
	if m.Scope.TenantID != 0 {
		attrs = append(attrs, slog.Int64(logging.KeyTenantID, m.Scope.TenantID))
	}
	if m.ActorID != 0 {
		attrs = append(attrs, slog.Int64(logging.KeyActorID, m.ActorID))
	}
	if m.TeamID != nil {
		attrs = append(attrs, slog.Int64(logging.KeyTeamID, *m.TeamID))
	}
	if m.PeriodID != nil {
		attrs = append(attrs, slog.Int64(logging.KeyPeriodID, *m.PeriodID))
	}
	if !m.OccurredAt.IsZero() {
		attrs = append(attrs, slog.Time("occurred_at", m.OccurredAt))
	}

	// Поля самого события отбираются по типу: идентификаторы, числа и флаги
	// попадают в лог, пользовательский текст — нет. Встроенная Meta не
	// раскрывается: её поля уже разобраны выше.
	for _, a := range logging.StructAttrs(ev) {
		attrs = append(attrs, a)
	}
	return attrs
}
