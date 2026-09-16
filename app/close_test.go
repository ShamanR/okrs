package app

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"okrs/internal/platform/eventbus"
)

func newTestBus(t *testing.T) *eventbus.Bus {
	t.Helper()
	b := eventbus.New(slog.New(slog.DiscardHandler))
	b.Start(context.Background())
	return b
}

// Порядок остановки нагружен смыслом: каналы уведомлений копят сообщения в
// памяти, и последние события обязаны успеть стать сообщениями до того, как
// накопленное выгружается. Значит выгрузка идёт ПОСЛЕ слива шины, а не рядом
// с остановкой фоновых петель.
func TestCloseFlushesChannelsAfterTheBusDrains(t *testing.T) {
	var order []string
	a := &App{
		bus:            newTestBus(t),
		stopBackground: func() { order = append(order, "background") },
		flushChannels: func(context.Context) error {
			order = append(order, "flush")
			return nil
		},
	}

	if err := a.Close(time.Second); err != nil {
		t.Fatalf("close: %v", err)
	}
	want := []string{"background", "flush"}
	if len(order) != len(want) {
		t.Fatalf("порядок остановки: got %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("шаг %d: got %q, want %q", i, order[i], want[i])
		}
	}
}

// Штатная остановка имеет бюджет: недоступный внешний сервис не должен
// превращать выкатку в застрявший под.
func TestCloseBoundsTheChannelFlush(t *testing.T) {
	var deadline time.Time
	var hasDeadline bool
	a := &App{
		bus:            newTestBus(t),
		stopBackground: func() {},
		flushChannels: func(ctx context.Context) error {
			deadline, hasDeadline = ctx.Deadline()
			return nil
		},
	}
	if err := a.Close(time.Second); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !hasDeadline {
		t.Fatal("выгрузка накопленного обязана идти с ограничением по времени")
	}
	if left := time.Until(deadline); left <= 0 || left > channelFlushTimeout+time.Second {
		t.Fatalf("бюджет выгрузки вне ожидаемого: осталось %v при лимите %v", left, channelFlushTimeout)
	}
}

// Неудачная выгрузка сообщается, а не проглатывается: оператор должен узнать,
// что часть уведомлений не ушла.
func TestCloseReportsFlushFailure(t *testing.T) {
	boom := errors.New("mattermost: posts: status 500")
	a := &App{
		bus:            newTestBus(t),
		stopBackground: func() {},
		flushChannels:  func(context.Context) error { return boom },
	}
	err := a.Close(time.Second)
	if !errors.Is(err, boom) {
		t.Fatalf("отказ выгрузки потерян: %v", err)
	}
}

// Сборка без каналов уведомлений останавливается как прежде.
func TestCloseWithoutChannelsIsUnchanged(t *testing.T) {
	a := &App{bus: newTestBus(t), stopBackground: func() {}}
	if err := a.Close(time.Second); err != nil {
		t.Fatalf("close: %v", err)
	}
}
