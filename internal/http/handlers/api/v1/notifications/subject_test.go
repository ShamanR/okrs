package notifications

import (
	"encoding/json"
	"strings"
	"testing"

	storenotif "okrs/internal/store/notifications"
)

// Находка ревью: в теле чек-ина названия KR нет (пять правил текста его туда не
// кладут), поэтому карточка обязана получить его отдельным полем. Решение принимает
// сервер — рендерер, — а клиент лишь рисует строку, когда поле непусто.
func TestToDTOCarriesSubjectForCheckIn(t *testing.T) {
	d := toDTO(storenotif.Notification{
		ID: 1, Kind: "kr_checked_in", EntityTitle: "MAU", CoalesceCount: 1,
		ActorDisplayName: "Пётр",
		Payload: map[string]any{
			"before": map[string]any{"progress": 10, "health": "on_track", "note": ""},
			"after":  map[string]any{"progress": 60, "health": "on_track", "note": ""},
		},
	})
	if d.Subject != "MAU" {
		t.Fatalf("subject = %q, want %q", d.Subject, "MAU")
	}
	if strings.Contains(d.Body, "MAU") {
		t.Errorf("название не должно дублироваться в теле: %q", d.Body)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"subject":"MAU"`) {
		t.Errorf("непустой предмет обязан быть в JSON: %s", raw)
	}
}

// У остальных видов название уже внутри тела: поле обязано отсутствовать в JSON
// целиком, иначе клиент нарисует пустую строку в карточке.
func TestToDTOOmitsEmptySubject(t *testing.T) {
	d := toDTO(storenotif.Notification{
		ID: 2, Kind: "comment_added", EntityTitle: "Снизить отток", CoalesceCount: 1,
		ActorDisplayName: "Пётр",
		Payload:          map[string]any{"text": "Уточните метрику"},
	})
	if d.Subject != "" {
		t.Fatalf("subject должен быть пуст, got %q", d.Subject)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "subject") {
		t.Errorf("пустой предмет не должен попадать в JSON: %s", raw)
	}
}
