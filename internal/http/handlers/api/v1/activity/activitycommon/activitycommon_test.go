package activitycommon

import (
	"net/http/httptest"
	"testing"
	"time"

	"okrs/internal/core/domain"
	activitysvc "okrs/internal/service/activity"
)

var now = time.Date(2026, 8, 25, 15, 30, 0, 0, time.UTC)

func parse(t *testing.T, query string) activitysvc.Filter {
	t.Helper()
	return ParseFilter(httptest.NewRequest("GET", "/api/v1/activity?"+query, nil), now)
}

func TestParseFilterReadsAllParams(t *testing.T) {
	f := parse(t, "period_id=7&team_ids=3&team_ids=4&category=goal&actor_udid=u-1&q=%D0%BF%D0%BB%D0%B0%D0%BD&limit=25&cursor=abc")
	if f.PeriodID == nil || *f.PeriodID != 7 {
		t.Fatalf("period_id = %v", f.PeriodID)
	}
	if len(f.TeamIDs) != 2 || f.TeamIDs[0] != 3 || f.TeamIDs[1] != 4 {
		t.Fatalf("team_ids = %v", f.TeamIDs)
	}
	if f.Category != "goal" || f.ActorUDID != "u-1" || f.Query != "план" {
		t.Fatalf("строковые фильтры разобраны неверно: %+v", f)
	}
	if f.Limit != 25 {
		t.Fatalf("limit = %d, want 25", f.Limit)
	}
	// Курсор проходит насквозь как непрозрачная строка: handler его не трогает.
	if f.Cursor != "abc" {
		t.Fatalf("cursor = %q, want \"abc\"", f.Cursor)
	}
}

func TestParseFilterDefaultLimit(t *testing.T) {
	if got := parse(t, "").Limit; got != DefaultLimit {
		t.Fatalf("limit без параметра = %d, want %d", got, DefaultLimit)
	}
}

// Лента открывается по ссылке с чужими параметрами: мусор в URL означает
// «фильтр не задан», а не 400 и не панику.
func TestParseFilterIgnoresGarbage(t *testing.T) {
	f := parse(t, "period_id=abc&team_ids=x&team_ids=5&limit=-3&range=позавчера")
	if f.PeriodID != nil {
		t.Fatalf("нечисловой period_id должен дать nil, получено %v", *f.PeriodID)
	}
	if len(f.TeamIDs) != 1 || f.TeamIDs[0] != 5 {
		t.Fatalf("нечисловой team_id должен быть пропущен, а числовой сохранён: %v", f.TeamIDs)
	}
	if f.Limit != DefaultLimit {
		t.Fatalf("отрицательный limit должен дать дефолт, получено %d", f.Limit)
	}
	if f.Since != nil {
		t.Fatalf("неизвестный range должен дать nil, получено %v", *f.Since)
	}
}

func TestSinceFromRange(t *testing.T) {
	cases := []struct {
		rng  string
		want *time.Time
	}{
		{"all", nil},
		{"", nil},
		{"мусор", nil},
		{"today", ptr(time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC))},
		{"7d", ptr(now.Add(-7 * 24 * time.Hour))},
		{"30d", ptr(now.Add(-30 * 24 * time.Hour))},
	}
	for _, c := range cases {
		got := SinceFromRange(c.rng, now)
		switch {
		case c.want == nil && got != nil:
			t.Fatalf("range=%q дал %v, want nil", c.rng, *got)
		case c.want != nil && got == nil:
			t.Fatalf("range=%q дал nil, want %v", c.rng, *c.want)
		case c.want != nil && !got.Equal(*c.want):
			t.Fatalf("range=%q дал %v, want %v", c.rng, *got, *c.want)
		}
	}
}

func ptr(t time.Time) *time.Time { return &t }

// Событие ведёт на доску команды, доступной смотрящему. Если store разрешил
// target-команду (расшаренная цель), ссылка идёт на неё, а не на команду-владельца:
// иначе пользователь попадёт на недоступную страницу.
func TestBuildTargetPrefersTargetTeam(t *testing.T) {
	owner, shared := int64(1), int64(2)
	tgt := BuildTarget(domain.ActivityEvent{TeamID: &owner, TargetTeamID: &shared})
	if tgt == nil || tgt.TeamID != shared {
		t.Fatalf("target = %+v, want команду %d", tgt, shared)
	}
	if tgt.Section != "tracker" {
		t.Fatalf("section = %q, want tracker", tgt.Section)
	}
}

func TestBuildTargetFallsBackToEventTeam(t *testing.T) {
	owner := int64(1)
	tgt := BuildTarget(domain.ActivityEvent{TeamID: &owner})
	if tgt == nil || tgt.TeamID != owner {
		t.Fatalf("target = %+v, want команду %d", tgt, owner)
	}
}

// Событие без команды никуда не ведёт — ссылку рисовать не из чего.
func TestBuildTargetNilWithoutTeam(t *testing.T) {
	if tgt := BuildTarget(domain.ActivityEvent{}); tgt != nil {
		t.Fatalf("target = %+v, want nil", tgt)
	}
}

func TestFeedResponseCarriesCursorAndItems(t *testing.T) {
	at := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	resp := FeedResponse(activitysvc.Page{
		Events:     []domain.ActivityEvent{{ID: 11, Category: "goal", Action: "created", CreatedAt: at}},
		NextCursor: "next-token",
	})
	if len(resp.Items) != 1 || resp.Items[0].ID != 11 {
		t.Fatalf("items = %+v", resp.Items)
	}
	if resp.Items[0].CreatedAt != "2026-05-06T07:08:09Z" {
		t.Fatalf("время сериализовано как %q", resp.Items[0].CreatedAt)
	}
	if resp.NextCursor != "next-token" {
		t.Fatalf("NextCursor = %q", resp.NextCursor)
	}
}

// Пустая лента должна отдавать [] а не null: клиент итерирует по массиву.
func TestFeedResponseEmptyIsArray(t *testing.T) {
	if got := FeedResponse(activitysvc.Page{}).Items; got == nil {
		t.Fatal("Items = nil, want пустой слайс")
	}
}
