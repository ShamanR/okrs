package logging

import (
	"log/slog"
	"reflect"
	"testing"
	"time"
)

type nested struct{ Secret string }

type sample struct {
	// Разрешены по типу.
	GoalID    int64
	Count     int
	Ratio     float64
	Enabled   bool
	TeamID    *int64
	MissingID *int64
	TeamIDs   []int64
	At        time.Time

	// Запрещены по типу.
	Title    string
	Titles   []string
	Nested   nested
	Payload  map[string]any
	Callback func()

	// Разрешены явной пометкой.
	Status  string            `log:"safe"`
	Changed map[string][2]any `log:"keys"`

	// Запрещено явной пометкой, хотя тип разрешён.
	Hidden int64 `log:"-"`

	// Неэкспортируемое — недоступно рефлексии.
	private int64
}

func attrMap(t *testing.T, v any) map[string]any {
	t.Helper()
	out := make(map[string]any)
	for _, a := range StructAttrs(v) {
		out[a.Key] = a.Value.Any()
	}
	return out
}

func TestStructAttrsAllowsSafeTypes(t *testing.T) {
	team := int64(7)
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	got := attrMap(t, sample{
		GoalID: 10, Count: 3, Ratio: 0.5, Enabled: true,
		TeamID: &team, TeamIDs: []int64{1, 2}, At: at,
		Status: "active", Changed: map[string][2]any{"b": {1, 2}, "a": {3, 4}},
	})

	if got["goal_id"] != int64(10) || got["count"] != int64(3) {
		t.Errorf("числа не извлечены: %v", got)
	}
	if got["ratio"] != 0.5 || got["enabled"] != true {
		t.Errorf("дробное или флаг не извлечены: %v", got)
	}
	if got["team_id"] != int64(7) {
		t.Errorf("указатель не разыменован: %v", got["team_id"])
	}
	if !reflect.DeepEqual(got["team_ids"], []int64{1, 2}) {
		t.Errorf("срез идентификаторов не извлечён: %v", got["team_ids"])
	}
	if got["at"] != at {
		t.Errorf("время не извлечено: %v", got["at"])
	}
	if got["status"] != "active" {
		t.Errorf("строка с пометкой safe не извлечена: %v", got["status"])
	}
	if !reflect.DeepEqual(got["changed"], []string{"a", "b"}) {
		t.Errorf("ключи map не извлечены в отсортированном виде: %v", got["changed"])
	}
}

func TestStructAttrsDropsUnsafeAndOptedOutFields(t *testing.T) {
	got := attrMap(t, sample{
		Title: "текст", Titles: []string{"текст"},
		Nested: nested{Secret: "текст"}, Payload: map[string]any{"k": "текст"},
		Hidden: 1, private: 2,
	})

	for _, key := range []string{"title", "titles", "nested", "payload", "callback", "hidden", "private"} {
		if v, ok := got[key]; ok {
			t.Errorf("поле %q не должно попадать в лог: %v", key, v)
		}
	}
}

func TestStructAttrsOmitsNilPointer(t *testing.T) {
	got := attrMap(t, sample{GoalID: 1})

	if _, ok := got["missing_id"]; ok {
		t.Errorf("нулевой указатель не должен давать поле: %v", got["missing_id"])
	}
	if _, ok := got["team_id"]; ok {
		t.Errorf("нулевой указатель не должен давать поле: %v", got["team_id"])
	}
}

func TestStructAttrsOmitsEmptyChangedMap(t *testing.T) {
	if _, ok := attrMap(t, sample{})["changed"]; ok {
		t.Error("пустой map не должен давать поле")
	}
}

func TestStructAttrsAcceptsPointerToStruct(t *testing.T) {
	if got := attrMap(t, &sample{GoalID: 5}); got["goal_id"] != int64(5) {
		t.Errorf("указатель на структуру не разобран: %v", got)
	}
	if StructAttrs((*sample)(nil)) != nil {
		t.Error("nil-указатель должен давать пустой набор")
	}
	if StructAttrs(42) != nil {
		t.Error("не-структура должна давать пустой набор")
	}
}

// Дескрипторы строятся один раз на тип: рефлексия не должна выполняться на каждое
// опубликованное событие.
func TestDescriptorsAreCachedPerType(t *testing.T) {
	rt := reflect.TypeOf(sample{})
	descCache.Delete(rt)

	first := descriptorsFor(rt)
	if _, ok := descCache.Load(rt); !ok {
		t.Fatal("разбор типа не попал в кеш")
	}
	second := descriptorsFor(rt)

	if &first[0] != &second[0] {
		t.Error("повторный вызов пересобрал дескрипторы вместо чтения из кеша")
	}
}

func TestSnakeCase(t *testing.T) {
	cases := map[string]string{
		"GoalID":            "goal_id",
		"KRID":              "kr_id",
		"KRTitle":           "kr_title",
		"SourceTeamID":      "source_team_id",
		"SharedWithTeamIDs": "shared_with_team_ids",
		"ParentGoalIDs":     "parent_goal_ids",
		"ParentCommentID":   "parent_comment_id",
		"WithProgress":      "with_progress",
		"Bulk":              "bulk",
		"HealthBefore":      "health_before",
		"OccurredAt":        "occurred_at",
		"DocumentationURL":  "documentation_url",
	}
	for in, want := range cases {
		if got := snakeCase(in); got != want {
			t.Errorf("snakeCase(%q) = %q, ожидалось %q", in, got, want)
		}
	}
}

// Значения, отобранные StructAttrs, всё равно проходят редакцию по имени ключа:
// два рубежа независимы.
func TestExtractedFieldsStillPassThroughRedaction(t *testing.T) {
	type withSecret struct {
		APIToken string `log:"safe"`
		GoalID   int64
	}
	attrs := StructAttrs(withSecret{APIToken: "s3cret", GoalID: 1})

	var tokenAttr slog.Attr
	for _, a := range attrs {
		if a.Key == "api_token" {
			tokenAttr = a
		}
	}
	if tokenAttr.Key == "" {
		t.Fatalf("поле не извлечено: %v", attrs)
	}
	if RedactAttr(nil, tokenAttr).Value.String() != Mask {
		t.Error("секрет не замаскирован редакцией, хотя ключ его выдаёт")
	}
}
