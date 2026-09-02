package eventbus

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"okrs/internal/core/event"
	"okrs/internal/platform/logging"
)

func dropRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка не является валидным JSON: %v\n%s", err, line)
		}
		if rec[logging.KeyEvent] == logging.EventEventDropped {
			out = append(out, rec)
		}
	}
	return out
}

// Потеря события обязана оставлять след с типом события и причиной: иначе
// переполненный буфер выглядит как «мутация просто не произошла».
func TestDroppedEventIsLoggedWithKindAndReason(t *testing.T) {
	buf := &bytes.Buffer{}
	bus := New(logging.New(logging.Config{Output: buf}))

	// Буфер на одно событие и обработчик, который его занимает: следующие
	// публикации в этом же такте теряются.
	block := make(chan struct{})
	Subscribe(bus, "slow", func(context.Context, []event.GoalCreated) error {
		<-block
		return nil
	}, WithBuffer(1))
	bus.Start(context.Background())

	for i := 0; i < 20; i++ {
		bus.Publish(context.Background(), event.GoalCreated{GoalID: int64(i)})
	}

	if bus.Dropped() == 0 {
		close(block)
		t.Skip("буфер не переполнился — планировщик успел разобрать очередь")
	}

	recs := dropRecords(t, buf)
	if len(recs) == 0 {
		close(block)
		t.Fatalf("потери не оставили записи: %s", buf.String())
	}
	rec := recs[0]
	if rec["level"] != "WARN" {
		t.Errorf("уровень = %v, ожидался WARN", rec["level"])
	}
	if rec["kind"] != string(event.KindGoalCreated) {
		t.Errorf("kind = %v, ожидался %q", rec["kind"], event.KindGoalCreated)
	}
	if rec["reason"] != "subscriber buffer full" {
		t.Errorf("причина потери = %v", rec["reason"])
	}
	if rec["subscriber"] != "slow" {
		t.Errorf("подписчик = %v", rec["subscriber"])
	}

	close(block)
	_ = bus.Close(2 * time.Second)
}

func TestPublishAfterCloseIsLoggedAsDropped(t *testing.T) {
	buf := &bytes.Buffer{}
	bus := New(logging.New(logging.Config{Output: buf}))
	Subscribe(bus, "any", func(context.Context, []event.GoalCreated) error { return nil })
	bus.Start(context.Background())
	if err := bus.Close(2 * time.Second); err != nil {
		t.Fatalf("шина не закрылась: %v", err)
	}
	buf.Reset()

	bus.Publish(context.Background(), event.GoalCreated{GoalID: 1})

	recs := dropRecords(t, buf)
	if len(recs) != 1 {
		t.Fatalf("публикация после закрытия не оставила записи: %s", buf.String())
	}
	if recs[0]["reason"] != "bus closed" {
		t.Errorf("причина потери = %v, ожидалась \"bus closed\"", recs[0]["reason"])
	}
}
