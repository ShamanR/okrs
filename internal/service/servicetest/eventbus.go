package servicetest

import (
	"context"
	"sync"

	"okrs/internal/core/event"
)

// FakeBus records what a usecase published. Usecase tests assert on domain events
// now, not on journal rows — the journal shape is service/activity's business.
type FakeBus struct {
	mu     sync.Mutex
	Events []event.Event
}

func (f *FakeBus) Publish(_ context.Context, ev event.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Events = append(f.Events, ev)
}

func (f *FakeBus) PublishBatch(_ context.Context, evs []event.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Events = append(f.Events, evs...)
}

// KindsPublished is a convenience for assertions: the ordered list of what was sent.
func (f *FakeBus) KindsPublished() []event.Kind {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]event.Kind, 0, len(f.Events))
	for _, ev := range f.Events {
		out = append(out, ev.Kind())
	}
	return out
}
