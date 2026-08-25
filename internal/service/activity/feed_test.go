package activity

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
	storeactivity "okrs/internal/store/activity"
)

// capturingRepo запоминает фильтр, с которым сервис пошёл в store, и отдаёт
// заранее заданный курсор следующей страницы.
type capturingRepo struct {
	got  storeactivity.ListFilter
	next *storeactivity.Cursor
}

func (r *capturingRepo) Record(context.Context, domain.TenantScope, domain.ActivityEvent) (int64, error) {
	return 0, nil
}
func (r *capturingRepo) RecordBatch(context.Context, domain.TenantScope, []domain.ActivityEvent) error {
	return nil
}
func (r *capturingRepo) List(_ context.Context, _ domain.TenantScope, _ []int64, f storeactivity.ListFilter) ([]domain.ActivityEvent, *storeactivity.Cursor, error) {
	r.got = f
	return []domain.ActivityEvent{{ID: 1}}, r.next, nil
}
func (r *capturingRepo) TreeCounts(context.Context, domain.TenantScope, []int64, *int64, *time.Time) (map[int64]int, error) {
	return nil, nil
}
func (r *capturingRepo) CategoryCounts(_ context.Context, _ domain.TenantScope, _ []int64, f storeactivity.ListFilter) (map[string]int, error) {
	r.got = f
	return map[string]int{"goal": 2}, nil
}
func (r *capturingRepo) Purge(context.Context, domain.TenantScope, *time.Time) (int64, error) {
	return 0, nil
}

// Курсор непрозрачен для клиента: то, что ушло в NextCursor, должно вернуться
// в ту же позицию строки, иначе следующая страница начнётся не там.
func TestCursorRoundTrip(t *testing.T) {
	at := time.Date(2026, 3, 4, 5, 6, 7, 890123456, time.UTC)
	token := encodeCursor(&storeactivity.Cursor{CreatedAt: at, ID: 42})
	if token == "" {
		t.Fatal("пустой токен для непустого курсора")
	}
	back := decodeCursor(token)
	if back == nil {
		t.Fatal("токен не разобрался обратно")
	}
	if !back.CreatedAt.Equal(at) || back.ID != 42 {
		t.Fatalf("курсор не совпал: %v/%d, want %v/42", back.CreatedAt, back.ID, at)
	}
}

func TestEncodeCursorNilIsEmpty(t *testing.T) {
	if got := encodeCursor(nil); got != "" {
		t.Fatalf("encodeCursor(nil) = %q, want пустую строку", got)
	}
}

// «Протухший» или подделанный токен не должен ронять ленту: он трактуется как
// первая страница. Клиент курсор не конструирует и починить его не может.
func TestDecodeCursorGarbageIsFirstPage(t *testing.T) {
	for _, s := range []string{"", "!!!не base64!!!", "YWJj", "MjAyNi0wMy0wNHwx", "fDQy"} {
		if got := decodeCursor(s); got != nil {
			t.Fatalf("decodeCursor(%q) = %+v, want nil", s, got)
		}
	}
}

func TestFeedEncodesNextCursor(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	repo := &capturingRepo{next: &storeactivity.Cursor{CreatedAt: at, ID: 7}}
	page, err := New(repo, nil).Feed(context.Background(), domain.TenantScope{TenantID: 1}, nil, Filter{Limit: 10})
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if len(page.Events) != 1 {
		t.Fatalf("событий = %d, want 1", len(page.Events))
	}
	back := decodeCursor(page.NextCursor)
	if back == nil || back.ID != 7 {
		t.Fatalf("NextCursor не декодируется в исходный курсор: %q", page.NextCursor)
	}
}

// Последняя страница отдаёт пустой NextCursor — по нему клиент понимает, что
// листать больше нечего.
func TestFeedLastPageHasEmptyCursor(t *testing.T) {
	repo := &capturingRepo{next: nil}
	page, err := New(repo, nil).Feed(context.Background(), domain.TenantScope{TenantID: 1}, nil, Filter{})
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if page.NextCursor != "" {
		t.Fatalf("NextCursor = %q, want пустую строку", page.NextCursor)
	}
}

func TestFeedPassesFilterToStore(t *testing.T) {
	repo := &capturingRepo{}
	since := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	pid := int64(9)
	_, err := New(repo, nil).Feed(context.Background(), domain.TenantScope{TenantID: 1}, nil, Filter{
		PeriodID: &pid, TeamIDs: []int64{3, 4}, Category: "goal",
		ActorUDID: "u-1", Since: &since, Query: "план", Limit: 25,
	})
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	got := repo.got
	if got.PeriodID == nil || *got.PeriodID != 9 || len(got.TeamIDs) != 2 ||
		got.Category != "goal" || got.ActorUDID != "u-1" || got.Query != "план" || got.Limit != 25 {
		t.Fatalf("фильтр доехал искажённым: %+v", got)
	}
	if got.Since == nil || !got.Since.Equal(since) {
		t.Fatalf("Since не доехал: %v", got.Since)
	}
}

// Счётчики вкладок должны быть стабильны при переключении вкладки, поэтому
// категория из фильтра выбрасывается. Лимит и курсор для агрегата бессмысленны.
func TestCountByCategoryDropsCategoryLimitAndCursor(t *testing.T) {
	repo := &capturingRepo{}
	_, err := New(repo, nil).CountByCategory(context.Background(), domain.TenantScope{TenantID: 1}, nil, Filter{
		Category: "goal", Limit: 50,
		Cursor: encodeCursor(&storeactivity.Cursor{CreatedAt: time.Now(), ID: 3}),
	})
	if err != nil {
		t.Fatalf("CountByCategory: %v", err)
	}
	if repo.got.Category != "" {
		t.Fatalf("категория = %q, want пустую: счётчики вкладок обязаны быть стабильны", repo.got.Category)
	}
	if repo.got.Limit != 0 || repo.got.Cursor != nil {
		t.Fatalf("лимит/курсор не сброшены: limit=%d cursor=%+v", repo.got.Limit, repo.got.Cursor)
	}
}
