package eventbus_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"okrs/internal/core/event"
	"okrs/internal/platform/eventbus"
	"okrs/internal/platform/logging"
)

type ctxKey string

// Коалесценция не должна терять контекст публикации: подписчик, связывающий
// событие с породившим его запросом, обязан получить контекст СВОЕГО события,
// а не того, которое просто пришло в батч первым.
//
// Батч собирается детерминированно: первый обработчик блокируется, пока
// остальные события копятся в буфере, и следующий проход шины забирает их одним
// батчем.
func TestCoalescingPreservesPerEventContext(t *testing.T) {
	const key ctxKey = "publisher"

	b := eventbus.New(quietLogger())

	release := make(chan struct{})
	type seen struct {
		publisher any
		goalID    int64
	}
	got := make(chan seen, 8)
	blocked := make(chan struct{})

	eventbus.SubscribeAllWithContext(b, "collector", func(_ context.Context, evs []eventbus.Delivered) error {
		if len(evs) == 1 && evs[0].Event.(event.GoalCreated).GoalID == 0 {
			// Первое событие держит горутину, пока копится остальное.
			close(blocked)
			<-release
			return nil
		}
		for _, d := range evs {
			got <- seen{publisher: d.Ctx.Value(key), goalID: d.Event.(event.GoalCreated).GoalID}
		}
		return nil
	})
	b.Start(context.Background())

	// Занимаем горутину подписчика.
	b.Publish(context.Background(), event.GoalCreated{GoalID: 0})
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("подписчик не начал обработку")
	}

	// Пока она занята, три разных публикатора кладут по событию — они уйдут
	// в один батч.
	for i := 1; i <= 3; i++ {
		ctx := context.WithValue(context.Background(), key, i)
		b.Publish(ctx, event.GoalCreated{GoalID: int64(i)})
	}
	close(release)

	if err := b.Close(2 * time.Second); err != nil {
		t.Fatalf("шина не сдренилась: %v", err)
	}
	close(got)

	byGoal := make(map[int64]any)
	for s := range got {
		byGoal[s.goalID] = s.publisher
	}
	if len(byGoal) != 3 {
		t.Fatalf("получено %d событий, ожидались 3: %v", len(byGoal), byGoal)
	}
	for i := int64(1); i <= 3; i++ {
		if byGoal[i] != int(i) {
			t.Errorf("событие %d получило контекст публикатора %v, ожидался %d", i, byGoal[i], i)
		}
	}
}

// Паника подписчика — это несостоявшееся следствие чьей-то мутации, поэтому
// запись о ней обязана нести контекст публикатора и тип события.
func TestHandlerPanicRecordCarriesPublisherContextAndKind(t *testing.T) {
	buf := &bytes.Buffer{}
	b := eventbus.New(logging.New(logging.Config{Output: buf}))

	eventbus.Subscribe(b, "panicky", func(context.Context, []event.GoalCreated) error {
		panic("boom")
	}, eventbus.WithMode(eventbus.Sync))
	b.Start(context.Background())
	defer b.Close(time.Second)

	ctx := logging.WithScope(
		logging.WithRequestID(context.Background(), "req-panic"),
		logging.Scope{TenantID: 5, ActorID: 55},
	)
	b.Publish(ctx, event.GoalCreated{GoalID: 1})

	rec := findRecord(t, buf, "eventbus: handler panicked")
	if rec == nil {
		t.Fatalf("паника обработчика не оставила записи: %s", buf.String())
	}
	if rec[logging.KeyRequestID] != "req-panic" {
		t.Errorf("request_id = %v, ожидался req-panic", rec[logging.KeyRequestID])
	}
	if rec[logging.KeyTenantID] != float64(5) || rec[logging.KeyActorID] != float64(55) {
		t.Errorf("tenant/actor = %v/%v", rec[logging.KeyTenantID], rec[logging.KeyActorID])
	}
	kinds, _ := rec["kinds"].([]any)
	if len(kinds) != 1 || kinds[0] != string(event.KindGoalCreated) {
		t.Errorf("kinds = %v, ожидался [%s]", rec["kinds"], event.KindGoalCreated)
	}
}

// findRecord ищет первую запись с заданным сообщением.
func findRecord(t *testing.T, buf *bytes.Buffer, msg string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("запись не является валидным JSON: %v\n%s", err, line)
		}
		if rec["msg"] == msg {
			return rec
		}
	}
	return nil
}

// Синхронная доставка коалесценции не знает: контекст один на всю публикацию
// и верен для каждого её события.
func TestSyncDeliveryCarriesThePublishContext(t *testing.T) {
	const key ctxKey = "publisher"

	b := eventbus.New(quietLogger())
	var seenCtx []any

	eventbus.SubscribeAllWithContext(b, "sync", func(_ context.Context, evs []eventbus.Delivered) error {
		for _, d := range evs {
			seenCtx = append(seenCtx, d.Ctx.Value(key))
		}
		return nil
	}, eventbus.WithMode(eventbus.Sync))
	b.Start(context.Background())
	defer b.Close(time.Second)

	ctx := context.WithValue(context.Background(), key, "one-request")
	b.PublishBatch(ctx, []event.Event{
		event.GoalCreated{GoalID: 1},
		event.GoalCreated{GoalID: 2},
	})

	if len(seenCtx) != 2 {
		t.Fatalf("получено %d событий, ожидались 2", len(seenCtx))
	}
	for i, v := range seenCtx {
		if v != "one-request" {
			t.Errorf("событие %d получило контекст %v", i, v)
		}
	}
}
