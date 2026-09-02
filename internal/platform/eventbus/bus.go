// Package eventbus is the in-process domain event bus. Each subscription owns a
// buffered channel and a goroutine, so a slow subscriber never blocks a fast one and
// event order is preserved per subscriber (one goroutine = FIFO).
//
// Publish never blocks and never fails: a full buffer drops the event for that one
// subscriber, logs, and bumps a counter. That is the same guarantee the activity
// journal already gave — a bookkeeping write must not break a user's mutation.
package eventbus

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"okrs/internal/core/event"
	"okrs/internal/platform/logging"
)

// Mode selects how a subscriber is invoked.
type Mode int

const (
	// Async runs the handler on the subscriber's goroutine. Default.
	Async Mode = iota
	// Sync runs the handler inline inside Publish. Used by the activity journal so a
	// mutation's event is durable before the HTTP response, exactly as before the bus.
	Sync
)

const (
	defaultBuffer  = 256
	defaultTimeout = 30 * time.Second
)

// Handler receives a batch. A single Publish delivers a slice of one; PublishBatch
// and the async drain deliver bigger slices. Always-a-slice means a batching
// subscriber cannot silently degrade into a per-event loop.
type Handler[T event.Event] func(ctx context.Context, evs []T) error

type options struct {
	buffer  int
	mode    Mode
	timeout time.Duration
}

type Option func(*options)

func WithBuffer(n int) Option            { return func(o *options) { o.buffer = n } }
func WithMode(m Mode) Option             { return func(o *options) { o.mode = m } }
func WithTimeout(d time.Duration) Option { return func(o *options) { o.timeout = d } }

// deliver is the type-erased handler stored per subscription.
//
// It receives the queued events rather than bare ones so that each event's own
// publication context survives coalescing: a batch can mix publishers, and a
// subscriber that correlates records with the request that caused them needs the
// context of the event it is handling, not of whichever event happened to arrive
// first.
type deliver func(ctx context.Context, evs []queued) error

// queued carries the detached context alongside the event, so an async handler runs
// with the publisher's values but not its cancellation.
type queued struct {
	ctx context.Context
	ev  event.Event
}

// Delivered is one event together with the context it was published in.
//
// Handlers registered with SubscribeAllWithContext receive these instead of bare
// events. Everything else keeps the plain Handler shape: carrying the context per
// event only matters to subscribers that attribute an event to its originating
// request, and the rest should not have to unwrap a struct for nothing.
type Delivered struct {
	// Ctx is the context of THIS event's publication, detached from the
	// publisher's cancellation but carrying its values.
	Ctx   context.Context
	Event event.Event
}

// kindsOfEvents собирает отсортированный набор уникальных типов событий.
//
// Нужен записям о потере и о сбое обработчика: без типа потерянное событие
// неотличимо от любого другого, а именно тип и определяет, что именно не
// произошло.
func kindsOfEvents(evs []event.Event) []string {
	seen := make(map[string]struct{}, len(evs))
	out := make([]string, 0, len(evs))
	for _, ev := range evs {
		k := string(ev.Kind())
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func kindsOf(evs []queued) []string {
	return kindsOfEvents(events(evs))
}

// events unwraps the queued batch for handlers that do not need per-event context.
func events(evs []queued) []event.Event {
	out := make([]event.Event, len(evs))
	for i, q := range evs {
		out[i] = q.ev
	}
	return out
}

type subscriber struct {
	name    string
	mode    Mode
	timeout time.Duration
	ch      chan queued
	fn      deliver
}

type Bus struct {
	logger *slog.Logger

	// mu guards byKind, all, started and closed. PublishBatch holds RLock across the
	// target lookup and the non-blocking async channel sends only — never across a
	// handler call, sync or async — so no user code ever runs while mu is held.
	// That is what keeps this lock deadlock-free even though a Sync handler may
	// itself call back into Publish (the activity journal does, from Task 4): the
	// reentrant call is a brand new, independent RLock/RUnlock pair, not a nested
	// one, so it cannot be stuck behind a Close that is parked on Lock waiting for
	// an outer RLock to release.
	//
	// Close holds Lock only long enough to flip closed and close the async
	// channels — both O(subscribers), no handler code — so it cannot be blocked
	// for long by a concurrent PublishBatch either. That pairing is what makes
	// "send on closed channel" impossible: Close cannot close a channel while a
	// publish in flight still holds the read lock, and a publish that acquires the
	// read lock after Close finishes sees closed==true and skips the send instead
	// of racing it.
	mu      sync.RWMutex
	byKind  map[event.Kind][]*subscriber
	all     []*subscriber
	started bool
	closed  bool // true once channel-closing has been initiated by the first Close

	wg sync.WaitGroup
	// drainDone is closed once the drain started by the first Close call actually
	// finishes. Every Close call — the first, a concurrent second, or a retry after
	// a timeout — waits on this same channel with its own timeout, so each one
	// reports the real, current outcome instead of a cached or presumed one.
	drainDone chan struct{}
	dropped   atomic.Int64
}

func New(logger *slog.Logger) *Bus {
	return &Bus{
		logger:    logger,
		byKind:    make(map[event.Kind][]*subscriber),
		drainDone: make(chan struct{}),
	}
}

// Dropped reports how many events were discarded because a subscriber's buffer was
// full (or the bus was already closed). Non-zero in production means the buffer or
// the handler needs attention.
func (b *Bus) Dropped() int64 { return b.dropped.Load() }

func newSubscriber(name string, mode Mode, buffer int, timeout time.Duration, fn deliver) *subscriber {
	return &subscriber{name: name, mode: mode, timeout: timeout, ch: make(chan queued, buffer), fn: fn}
}

func resolve(opts []Option) options {
	o := options{buffer: defaultBuffer, mode: Async, timeout: defaultTimeout}
	for _, fn := range opts {
		fn(&o)
	}
	if o.buffer <= 0 {
		o.buffer = defaultBuffer
	}
	if o.timeout <= 0 {
		o.timeout = defaultTimeout
	}
	return o
}

// Subscribe registers a handler for one concrete event type. It is a package
// function, not a method: Go methods cannot take type parameters.
//
// The routing key comes from the zero value of T, so no reflection is involved. T is
// always one of the value-receiver event structs in package event, whose Kind()
// tolerates a zero value; T instantiated as a pointer or interface type is not
// reachable from any call site in this plan and is not guarded against here.
// Must be called before Start.
func Subscribe[T event.Event](b *Bus, name string, h Handler[T], opts ...Option) {
	var zero T
	kind := zero.Kind()
	o := resolve(opts)

	fn := func(ctx context.Context, evs []queued) error {
		typed := make([]T, 0, len(evs))
		for _, q := range evs {
			if t, ok := any(q.ev).(T); ok {
				typed = append(typed, t)
			}
		}
		if len(typed) == 0 {
			return nil
		}
		return h(ctx, typed)
	}

	s := newSubscriber(name, o.mode, o.buffer, o.timeout, fn)

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started {
		panic("eventbus: Subscribe after Start")
	}
	b.byKind[kind] = append(b.byKind[kind], s)
}

// SubscribeAll registers a handler for every event type. It exists for the activity
// journal, which needs all 22 kinds — listing them one by one would silently miss
// the 23rd.
func SubscribeAll(b *Bus, name string, h Handler[event.Event], opts ...Option) {
	o := resolve(opts)
	s := newSubscriber(name, o.mode, o.buffer, o.timeout, func(ctx context.Context, evs []queued) error {
		return h(ctx, events(evs))
	})

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started {
		panic("eventbus: SubscribeAll after Start")
	}
	b.all = append(b.all, s)
}

// SubscribeAllWithContext registers a handler for every event type that also needs
// each event's own publication context.
//
// It exists because coalescing merges events from different publishers into one
// batch: a handler that reads context off the batch would attribute every event to
// whichever publisher arrived first. The logging subscriber needs the real one to
// tie a record to the request that caused it — and one HTTP request can publish an
// event per affected entity, so "the first event's context" is not good enough
// either.
//
// ctx is the batch's context and carries the handler timeout; Delivered.Ctx is the
// per-event one. Must be called before Start.
func SubscribeAllWithContext(b *Bus, name string, h func(ctx context.Context, evs []Delivered) error, opts ...Option) {
	o := resolve(opts)
	s := newSubscriber(name, o.mode, o.buffer, o.timeout, func(ctx context.Context, evs []queued) error {
		out := make([]Delivered, len(evs))
		for i, q := range evs {
			out[i] = Delivered{Ctx: q.ctx, Event: q.ev}
		}
		return h(ctx, out)
	})

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started {
		panic("eventbus: SubscribeAllWithContext after Start")
	}
	b.all = append(b.all, s)
}

// Start launches one goroutine per async subscriber. Sync subscribers need none.
func (b *Bus) Start(context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started {
		return
	}
	b.started = true
	for _, s := range b.subscribersLocked() {
		if s.mode != Async {
			continue
		}
		b.wg.Add(1)
		go b.run(s)
	}
}

// subscribersLocked returns every subscriber, kind-specific and wildcard. Callers
// must already hold mu (read or write).
func (b *Bus) subscribersLocked() []*subscriber {
	n := len(b.all)
	for _, list := range b.byKind {
		n += len(list)
	}
	out := make([]*subscriber, 0, n)
	out = append(out, b.all...)
	for _, list := range b.byKind {
		out = append(out, list...)
	}
	return out
}

// run drains the subscriber's channel, coalescing whatever is already queued into a
// single batch — a burst of publishes becomes one handler call, not N.
func (b *Bus) run(s *subscriber) {
	defer b.wg.Done()
	for first := range s.ch {
		// Каждое событие переносится в батч ВМЕСТЕ со своим контекстом.
		// Раньше сохранялся только контекст первого публикатора, и подписчик,
		// связывающий событие с породившим его запросом, либо приписывал всем
		// событиям чужой запрос, либо терял связь для всех, кроме первого.
		// Оба исхода плохи: один HTTP-запрос может опубликовать событие на каждую
		// затронутую сущность (bulk-переход статусов).
		batch := []queued{first}
	drain:
		for {
			select {
			case next, ok := <-s.ch:
				if !ok {
					break drain
				}
				batch = append(batch, next)
			default:
				break drain
			}
		}
		// Для таймаута и отмены берётся контекст первого события: это одно решение
		// на весь вызов обработчика, и выбрать тут нечего кроме первого.
		b.invoke(s, batch[0].ctx, batch)
	}
}

// invoke calls the handler with panic and error containment plus a timeout.
func (b *Bus) invoke(s *subscriber, ctx context.Context, evs []queued) {
	defer func() {
		if r := recover(); r != nil {
			// ErrorContext: без контекста запись о сорвавшемся обработчике невозможно
			// связать с мутацией, которая её вызвала. Для батча из одного события
			// контекст точен; для коалесцированного он принадлежит первому событию,
			// поэтому рядом идёт размер батча — читатель видит, что сбой мог
			// затронуть и другие запросы.
			b.logger.ErrorContext(ctx, "eventbus: handler panicked",
				slog.String(logging.KeyEvent, logging.EventDomainEvent),
				slog.String("subscriber", s.name),
				slog.Int("batch_size", len(evs)),
				slog.Any("kinds", kindsOf(evs)),
				slog.String("panic", fmt.Sprint(r)),
				slog.String("stack", string(debug.Stack())))
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if err := s.fn(ctx, evs); err != nil {
		b.logger.WarnContext(ctx, "eventbus: handler failed",
			slog.String(logging.KeyEvent, logging.EventDomainEvent),
			slog.String("subscriber", s.name),
			slog.Int("batch_size", len(evs)),
			slog.Any("kinds", kindsOf(evs)),
			slog.String("err", err.Error()))
	}
}

// Publish delivers one event. Never blocks, never returns an error.
func (b *Bus) Publish(ctx context.Context, ev event.Event) {
	b.PublishBatch(ctx, []event.Event{ev})
}

// PublishBatch delivers many events at once. Each subscriber receives the subset it
// is registered for, in one handler call — that is what keeps the journal's
// RecordBatch batched.
//
// The read lock is held across the target lookup and the non-blocking async channel
// sends only — both bounded, O(subscribers), no user code. It is released before any
// handler runs, sync or async. That is deliberate on two counts:
//
//  1. Sync handlers may call back into Publish/PublishBatch themselves (the activity
//     journal does, from Task 4). Invoking them under RLock would make that a nested
//     RLock, and Go's RWMutex parks a new reader behind a writer that is already
//     waiting for Lock — so a Close racing in between would deadlock both itself and
//     the reentrant Publish. Running Sync handlers after RUnlock makes the reentrant
//     call an independent, non-nested RLock instead.
//  2. It keeps Close's own Lock() wait bounded: Close only ever contends with the
//     cheap target-lookup-and-send section, never with an arbitrary (possibly slow
//     or blocked) handler, so Close cannot be made to overrun its timeout by a slow
//     subscriber.
//
// Close closing a subscriber's channel still cannot race a send into it: Close
// cannot close a channel until every in-flight PublishBatch has released the read
// lock, and a PublishBatch that acquires the read lock after Close finished sees
// b.closed and drops instead of writing to a closed channel. The closed check runs
// before the mode branch below, so it drops Sync targets the same way: closed is
// checked and decided before a subscriber is ever routed into syncCalls, not after.
func (b *Bus) PublishBatch(ctx context.Context, evs []event.Event) {
	if len(evs) == 0 {
		return
	}
	b.mu.RLock()

	targets := make(map[*subscriber][]event.Event)
	for _, ev := range evs {
		for _, s := range b.byKind[ev.Kind()] {
			targets[s] = append(targets[s], ev)
		}
		for _, s := range b.all {
			targets[s] = append(targets[s], ev)
		}
	}

	// The request context is detached: values (trace, logger) survive, cancellation
	// does not — an async handler outlives the request that triggered it.
	async := context.WithoutCancel(ctx)
	closed := b.closed // loop-invariant; read once instead of on every iteration.

	var syncCalls []struct {
		s     *subscriber
		batch []event.Event
	}
	for s, batch := range targets {
		if closed {
			// The bus is shutting down (or already has). For an async subscriber
			// s.ch is closed or about to be, and sending would panic; for a sync
			// subscriber the handler must not run at all past Close — a mutation
			// that completes after Close returned must not still be writing through a
			// pool the caller is free to tear down right after. Either way: drop, same
			// as a full buffer would, and counted the same in Dropped().
			b.dropped.Add(int64(len(batch)))
			// WarnContext, а не Warn: контекст публикации несёт запрос, организацию
			// и действующего пользователя, а запись о потере без них не позволяет
			// узнать, чьё действие потеряло событие.
			b.logger.WarnContext(ctx, "eventbus: bus closed, events dropped",
				slog.String(logging.KeyEvent, logging.EventEventDropped),
				slog.String("subscriber", s.name),
				slog.String("reason", "bus closed"),
				// Типы обязательны: без них потерянный goal_created неотличим
				// от любого другого события.
				slog.Any("kinds", kindsOfEvents(batch)),
				slog.Int("count", len(batch)))
			continue
		}
		if s.mode == Sync {
			// Deferred until after RUnlock — see doc comment above.
			syncCalls = append(syncCalls, struct {
				s     *subscriber
				batch []event.Event
			}{s, batch})
			continue
		}
		for _, ev := range batch {
			select {
			case s.ch <- queued{ctx: async, ev: ev}:
			default:
				b.dropped.Add(1)
				// См. комментарий выше о WarnContext.
				b.logger.WarnContext(ctx, "eventbus: buffer full, event dropped",
					slog.String(logging.KeyEvent, logging.EventEventDropped),
					slog.String("subscriber", s.name),
					slog.String("reason", "subscriber buffer full"),
					slog.String("kind", string(ev.Kind())))
			}
		}
	}
	b.mu.RUnlock()

	// Sync handlers run here, with no lock held at all — see doc comment above.
	// У синхронного пути контекст один на всю публикацию и верен для каждого
	// её события: коалесценции здесь нет.
	for _, call := range syncCalls {
		queue := make([]queued, len(call.batch))
		for i, ev := range call.batch {
			queue[i] = queued{ctx: ctx, ev: ev}
		}
		b.invoke(call.s, ctx, queue)
	}
}

// Close stops accepting events and waits for the buffers to drain, so a graceful
// SIGTERM does not lose what is already queued. This applies to both modes: once
// Close has flipped b.closed (see PublishBatch), a publish that reaches a Sync
// subscriber after that point is dropped and counted in Dropped() exactly like an
// async subscriber's full buffer would be — a Sync handler never runs again after
// Close has been observed as closed, so a caller that closes a resource (e.g. a DB
// pool) right after Close returns cannot race a Sync handler still using it.
//
// The async subscribers' channels are closed while holding the write lock. Only the
// first call does the closing; the write-lock section is O(subscribers) with no
// handler code in it (PublishBatch never invokes a handler while holding mu — see
// its doc comment), so Close is never blocked waiting on a slow or stuck handler to
// finish, and cannot itself overrun its timeout while acquiring the lock.
//
// Every call — the first, a concurrent second, or a retry after a previous timeout —
// waits on the same drainDone channel, each with its own timeout, and reports
// whatever is actually true at that moment: nil only if the drain has genuinely
// finished by then, otherwise a timeout error. No caller ever gets a bare nil for a
// drain that is still in flight or that a previous call already gave up on. A
// goroutine started by the first Close keeps running wg.Wait() past any caller's
// timeout so it can still close drainDone later — harmless, since it holds no lock
// and Close's own state (started/closed) never regresses.
func (b *Bus) Close(timeout time.Duration) error {
	b.mu.Lock()
	if !b.started {
		b.mu.Unlock()
		return nil
	}
	if !b.closed {
		b.closed = true
		subs := b.subscribersLocked()
		for _, s := range subs {
			if s.mode == Async {
				close(s.ch)
			}
		}
		go func() {
			b.wg.Wait()
			close(b.drainDone)
		}()
	}
	b.mu.Unlock()

	select {
	case <-b.drainDone:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("eventbus: drain timed out after %s", timeout)
	}
}
