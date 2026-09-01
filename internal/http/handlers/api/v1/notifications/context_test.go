package notifications

import (
	"encoding/json"
	"strings"
	"testing"

	storenotif "okrs/internal/store/notifications"
)

// Контекст нужен, чтобы длинный список читался без открытия каждой записи:
// видно, к какой команде и цели относится событие.
func TestToDTOCarriesContext(t *testing.T) {
	d := toDTO(storenotif.Notification{
		ID: 1, Kind: "comment_added", EntityTitle: "Снизить отток", CoalesceCount: 1,
		ActorDisplayName: "Пётр",
		Payload:          map[string]any{"text": "Уточните метрику"},
		TeamPath:         "Компания / Платформа / Биллинг",
		GoalTitle:        "Снизить отток",
	})
	if d.Context == nil {
		t.Fatal("контекст обязан быть заполнен")
	}
	if d.Context.Team != "Компания / Платформа / Биллинг" {
		t.Errorf("команда = %q", d.Context.Team)
	}
	if d.Context.Goal != "Снизить отток" {
		t.Errorf("цель = %q", d.Context.Goal)
	}
}

// Половина контекста тоже контекст: у уведомления может быть команда без цели.
func TestToDTOCarriesHalfContext(t *testing.T) {
	d := toDTO(storenotif.Notification{
		ID: 2, Kind: "status_changed", EntityTitle: "Платформа", CoalesceCount: 1,
		ActorDisplayName: "Пётр",
		TeamPath:         "Компания / Платформа",
	})
	if d.Context == nil || d.Context.Team != "Компания / Платформа" {
		t.Fatalf("контекст = %+v", d.Context)
	}
	if d.Context.Goal != "" {
		t.Errorf("цели быть не должно: %q", d.Context.Goal)
	}
}

// Без команды и цели объект обязан отсутствовать в JSON целиком — иначе клиент
// нарисует пустую строку под телом карточки.
func TestToDTOOmitsEmptyContext(t *testing.T) {
	d := toDTO(storenotif.Notification{
		ID: 3, Kind: "comment_added", EntityTitle: "Цель", CoalesceCount: 1,
		ActorDisplayName: "Пётр",
		Payload:          map[string]any{"text": "текст"},
	})
	if d.Context != nil {
		t.Fatalf("контекст должен быть nil, got %+v", d.Context)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), `"context"`) {
		t.Errorf("пустой контекст не должен попадать в JSON: %s", raw)
	}
}
