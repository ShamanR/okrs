package eventbus_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"okrs/internal/core/event"
	"okrs/internal/platform/eventbus"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// collector — потокобезопасный сборщик, общий для тестов ниже.
type collector struct {
	mu   sync.Mutex
	seen []event.Kind
}

func (c *collector) add(ks ...event.Kind) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen = append(c.seen, ks...)
}

func (c *collector) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.seen)
}

// Главное свойство типизированной подписки: подписчик одного типа не должен
// видеть события другого. Без этого гранулярность подписки — фикция.
func TestSubscribeRoutesByType(t *testing.T) {
	b := eventbus.New(quietLogger())
	var comments, progress collector

	eventbus.Subscribe(b, "comments", func(_ context.Context, evs []event.CommentAdded) error {
		for range evs {
			comments.add(event.KindCommentAdded)
		}
		return nil
	}, eventbus.WithMode(eventbus.Sync))

	eventbus.Subscribe(b, "progress", func(_ context.Context, evs []event.KRCheckedIn) error {
		for range evs {
			progress.add(event.KindKRCheckedIn)
		}
		return nil
	}, eventbus.WithMode(eventbus.Sync))

	b.Start(context.Background())
	defer b.Close(time.Second)

	b.Publish(context.Background(), event.CommentAdded{GoalID: 1})
	b.Publish(context.Background(), event.CommentAdded{GoalID: 2})
	b.Publish(context.Background(), event.KRCheckedIn{KRID: 3})

	if got := comments.len(); got != 2 {
		t.Errorf("подписчик комментариев: got %d, want 2", got)
	}
	if got := progress.len(); got != 1 {
		t.Errorf("подписчик прогресса: got %d, want 1", got)
	}
}

// SubscribeAll существует ради журнала: ему нужны все типы, и перечислять 21
// подписку значит забыть про 22-ю.
func TestSubscribeAllReceivesEveryType(t *testing.T) {
	b := eventbus.New(quietLogger())
	var all collector

	eventbus.SubscribeAll(b, "journal", func(_ context.Context, evs []event.Event) error {
		for _, ev := range evs {
			all.add(ev.Kind())
		}
		return nil
	}, eventbus.WithMode(eventbus.Sync))

	b.Start(context.Background())
	defer b.Close(time.Second)

	b.Publish(context.Background(), event.CommentAdded{})
	b.Publish(context.Background(), event.KRCheckedIn{})
	b.Publish(context.Background(), event.StatusChanged{})

	if got := all.len(); got != 3 {
		t.Fatalf("wildcard-подписчик: got %d, want 3", got)
	}
}

// Паника в одном обработчике не должна убивать ни шину, ни соседей: подписчики
// изолированы, иначе один плохой слушатель роняет журнал.
func TestPanicInHandlerIsIsolated(t *testing.T) {
	b := eventbus.New(quietLogger())
	var good collector

	eventbus.Subscribe(b, "panicky", func(_ context.Context, _ []event.CommentAdded) error {
		panic("boom")
	}, eventbus.WithMode(eventbus.Sync))

	eventbus.Subscribe(b, "good", func(_ context.Context, evs []event.CommentAdded) error {
		good.add(event.KindCommentAdded)
		return nil
	}, eventbus.WithMode(eventbus.Sync))

	b.Start(context.Background())
	defer b.Close(time.Second)

	b.Publish(context.Background(), event.CommentAdded{}) // не должно паниковать наружу

	if got := good.len(); got != 1 {
		t.Fatalf("соседний подписчик не отработал: got %d, want 1", got)
	}
}

// Ошибка обработчика логируется, но не всплывает: публикация никогда не должна
// ронять пользовательскую мутацию.
func TestHandlerErrorDoesNotPropagate(t *testing.T) {
	b := eventbus.New(quietLogger())
	eventbus.Subscribe(b, "failing", func(_ context.Context, _ []event.CommentAdded) error {
		return errors.New("db down")
	}, eventbus.WithMode(eventbus.Sync))

	b.Start(context.Background())
	defer b.Close(time.Second)

	b.Publish(context.Background(), event.CommentAdded{}) // не должно паниковать и не должно блокировать
}

// Переполнение буфера роняет событие и считает дроп, но не блокирует Publish.
// Обработчик держим заблокированным, чтобы канал гарантированно переполнился.
func TestFullBufferDropsInsteadOfBlocking(t *testing.T) {
	b := eventbus.New(quietLogger())
	release := make(chan struct{})

	eventbus.Subscribe(b, "slow", func(_ context.Context, _ []event.CommentAdded) error {
		<-release
		return nil
	}, eventbus.WithMode(eventbus.Async), eventbus.WithBuffer(1))

	b.Start(context.Background())

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			b.Publish(context.Background(), event.CommentAdded{GoalID: int64(i)})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish заблокировался на полном буфере — должен дропать")
	}

	close(release)
	if err := b.Close(time.Second); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if b.Dropped() == 0 {
		t.Fatal("ожидались дропнутые события при переполнении буфера")
	}
}

// Async-обработчик не должен зависеть от ctx запроса: тот отменяется, как только
// handler вернул ответ, а работа подписчика продолжается уже после этого.
func TestAsyncHandlerSurvivesRequestContextCancel(t *testing.T) {
	b := eventbus.New(quietLogger())
	got := make(chan error, 1)

	eventbus.Subscribe(b, "async", func(ctx context.Context, _ []event.CommentAdded) error {
		got <- ctx.Err()
		return nil
	}, eventbus.WithMode(eventbus.Async))

	b.Start(context.Background())
	defer b.Close(time.Second)

	reqCtx, cancel := context.WithCancel(context.Background())
	b.Publish(reqCtx, event.CommentAdded{})
	cancel() // запрос завершился

	select {
	case err := <-got:
		if err != nil {
			t.Fatalf("ctx обработчика отменён вместе с запросом: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("async-обработчик не был вызван")
	}
}

// Close дренирует буферы: события, лежавшие в канале на момент остановки,
// должны быть обработаны, а не потеряны при штатном SIGTERM.
func TestCloseDrainsBuffer(t *testing.T) {
	b := eventbus.New(quietLogger())
	var seen collector
	gate := make(chan struct{})

	eventbus.Subscribe(b, "drain", func(_ context.Context, evs []event.CommentAdded) error {
		<-gate
		for range evs {
			seen.add(event.KindCommentAdded)
		}
		return nil
	}, eventbus.WithMode(eventbus.Async), eventbus.WithBuffer(16))

	b.Start(context.Background())
	for i := 0; i < 5; i++ {
		b.Publish(context.Background(), event.CommentAdded{GoalID: int64(i)})
	}
	close(gate)

	if err := b.Close(2 * time.Second); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := seen.len(); got != 5 {
		t.Fatalf("дренаж потерял события: got %d, want 5", got)
	}
}

// PublishBatch должен доходить до обработчика одним срезом, а не пятью вызовами:
// на этом держится RecordBatch в журнале (иначе N+1).
func TestPublishBatchArrivesAsOneSlice(t *testing.T) {
	b := eventbus.New(quietLogger())
	sizes := make(chan int, 4)

	eventbus.SubscribeAll(b, "batch", func(_ context.Context, evs []event.Event) error {
		sizes <- len(evs)
		return nil
	}, eventbus.WithMode(eventbus.Sync))

	b.Start(context.Background())
	defer b.Close(time.Second)

	b.PublishBatch(context.Background(), []event.Event{
		event.CommentAdded{}, event.CommentAdded{}, event.KRCheckedIn{},
	})

	select {
	case n := <-sizes:
		if n != 3 {
			t.Fatalf("батч пришёл срезом длины %d, want 3", n)
		}
	default:
		t.Fatal("обработчик не вызван")
	}
}

// TestPublishDuringCloseDoesNotPanic guards the correction to the brief: PublishBatch
// must hold the read lock across both the target lookup and the channel sends, and
// Close must hold the write lock while closing those same channels — otherwise a
// publish in flight during shutdown can send on a channel Close just closed, which
// panics with "send on closed channel". This test hammers Publish concurrently with
// Close and must be run with -race to also catch a data race on the subscriber map.
func TestPublishDuringCloseDoesNotPanic(t *testing.T) {
	for i := 0; i < 20; i++ {
		b := eventbus.New(quietLogger())
		eventbus.Subscribe(b, "concurrent", func(_ context.Context, _ []event.CommentAdded) error {
			return nil
		}, eventbus.WithMode(eventbus.Async), eventbus.WithBuffer(4))

		b.Start(context.Background())

		var wg sync.WaitGroup
		stop := make(chan struct{})
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					b.Publish(context.Background(), event.CommentAdded{})
				}
			}
		}()

		// Give the publisher goroutine a moment to actually start racing with Close.
		time.Sleep(time.Millisecond)
		if err := b.Close(time.Second); err != nil {
			t.Fatalf("Close: %v", err)
		}
		close(stop)
		wg.Wait()
	}
}

// TestSyncHandlerPublishDuringCloseDoesNotDeadlock guards Critical 1 from the first
// review round: a Sync handler that calls back into Publish (as the activity journal
// will from Task 4, when one usecase call publishes several events) must not
// deadlock against a concurrent Close.
//
// Before the fix, PublishBatch invoked Sync handlers while still holding
// b.mu.RLock(). The handler's own Publish call then tried to re-acquire RLock as a
// *nested* read lock on the same goroutine. Go's RWMutex parks a new reader behind a
// writer that is already waiting for Lock, so if a Close happened to be parked on
// Lock in between the outer RLock and the reentrant one, both the reentrant Publish
// and Close would block forever — Close never reaches its own timeout select,
// because it hangs before that, still waiting on mu.Lock().
//
// The test has its own hard timeout so a regression fails this test instead of
// hanging the whole suite.
// TestClosePreventsSyncDelivery guards the fix to the second review round: PublishBatch
// used to check s.mode == Sync before checking b.closed, so a publish that landed after
// Close had returned still invoked a Sync handler inline. The only Sync subscriber in
// the tree is the activity journal, writing to Postgres through a pool that main closes
// around the same time — so a publish racing (or simply arriving after) Close could
// write through an already-closed pool. After the fix, closed is checked first: a
// publish after Close must not invoke the Sync handler, and must count as a drop.
func TestClosePreventsSyncDelivery(t *testing.T) {
	b := eventbus.New(quietLogger())
	var calls int

	eventbus.Subscribe(b, "sync-after-close", func(_ context.Context, _ []event.CommentAdded) error {
		calls++
		return nil
	}, eventbus.WithMode(eventbus.Sync))

	b.Start(context.Background())
	if err := b.Close(time.Second); err != nil {
		t.Fatalf("Close: %v", err)
	}

	before := b.Dropped()
	b.Publish(context.Background(), event.CommentAdded{GoalID: 1})

	if calls != 0 {
		t.Fatalf("Sync handler invoked after Close: calls = %d, want 0", calls)
	}
	if got := b.Dropped(); got != before+1 {
		t.Fatalf("Dropped() after publishing to a closed bus: got %d, want %d", got, before+1)
	}
}

func TestSyncHandlerPublishDuringCloseDoesNotDeadlock(t *testing.T) {
	b := eventbus.New(quietLogger())

	eventbus.Subscribe(b, "reentrant", func(ctx context.Context, _ []event.CommentAdded) error {
		// Reenters the bus from inside a Sync handler.
		b.Publish(ctx, event.KRCheckedIn{KRID: 1})
		return nil
	}, eventbus.WithMode(eventbus.Sync))

	b.Start(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				b.Publish(context.Background(), event.CommentAdded{GoalID: int64(i)})
			}
		}()
		go func() {
			defer wg.Done()
			if err := b.Close(2 * time.Second); err != nil {
				t.Errorf("Close: %v", err)
			}
		}()
		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Sync handler re-entering Publish deadlocked against a concurrent Close")
	}
}
