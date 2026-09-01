// Package event holds the OKR domain events. One struct per event type, pure data,
// no I/O — the activity journal and the notification fan-out are both subscribers,
// neither owns these types.
package event

import (
	"time"

	"okrs/internal/core/domain"
)

// Kind is the routing key of an event type. eventbus.Subscribe reads it off the
// zero value of T, so it must be a constant per type and never depend on state.
type Kind string

// Event is the marker every domain event implements.
type Event interface {
	Kind() Kind
	// Context exposes the embedded Meta, so a subscriber can read scope and actor
	// without a type switch over all 21 types. Promoted through embedding.
	Context() Meta
}

// Meta is the context every event carries. Embedded, so Scope/ActorID are readable
// without a type switch.
type Meta struct {
	Scope    domain.TenantScope
	ActorID  int64
	TeamID   *int64
	PeriodID *int64
	// OccurredAt is stamped at every publication site. The activity journal's
	// base() ignores it — a journal row's created_at comes from the database
	// instead — but notification coalescing does not: coalesceKey in
	// internal/usecase/notification derives the collapse bucket from it
	// (at.Unix() / CoalesceWindow), falling back to time.Now() only when it is
	// zero. Leaving it unset therefore changes which events collapse into one
	// notification, so it is load-bearing on that path.
	OccurredAt time.Time
}

// Context returns the event's common context. Because Meta is embedded in every
// event, declaring Kind is all a new event type needs to satisfy Event.
func (m Meta) Context() Meta { return m }
