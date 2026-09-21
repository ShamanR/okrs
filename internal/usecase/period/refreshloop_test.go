package healthcheckin

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/platform/logging"
)

// Цикл обновления кеша обязан пережить панику тика и продолжить работу.
//
// Без перехвата паника здесь уносит весь процесс. С перехватом вокруг всей
// горутины он бы выжил, но обновление кеша остановилось бы навсегда, и кеш
// молча отдавал бы устаревшие данные до перезапуска пода — отказ тем хуже,
// что незаметен. Поэтому защищена именно единица работы одного тика.
func TestRefreshLoopSurvivesAPanickingTick(t *testing.T) {
	buf := &bytes.Buffer{}
	var mu sync.Mutex
	ticks := 0

	c := NewCache(
		func(context.Context, domain.TenantScope, int64) (*PeriodData, error) {
			return &PeriodData{}, nil
		},
		time.Minute,
		logging.New(logging.Config{Output: buf}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.StartRefreshLoop(ctx, time.Millisecond, func(context.Context) []Active {
		mu.Lock()
		defer mu.Unlock()
		ticks++
		if ticks == 1 {
			panic("обход периодов сорвался")
		}
		return nil
	})

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		seen := ticks
		mu.Unlock()
		if seen >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("цикл остановился после паники: тиков %d", seen)
		case <-time.After(2 * time.Millisecond):
		}
	}

	rec := panicRecord(t, buf)
	if rec == nil {
		t.Fatalf("паника тика не оставила записи: %s", buf.String())
	}
	if rec["level"] != "ERROR" {
		t.Errorf("уровень = %v, ожидался ERROR", rec["level"])
	}
	if rec["task"] != "healthcheckin_cache_refresh" {
		t.Errorf("task = %v", rec["task"])
	}
	if cause, _ := rec["panic"].(string); !strings.Contains(cause, "обход периодов сорвался") {
		t.Errorf("причина = %v", rec["panic"])
	}
}

func panicRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка не является валидным JSON: %v\n%s", err, line)
		}
		if rec[logging.KeyEvent] == logging.EventBackgroundPanic {
			return rec
		}
	}
	return nil
}
