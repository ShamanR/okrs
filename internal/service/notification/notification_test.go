package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/store/notifications"
)

// capturingRepo records the filter the service passed to List, and hands back a
// preset page. Mirrors service/activity's capturingRepo (feed_test.go).
type capturingRepo struct {
	got  notifications.ListFilter
	next *notifications.Cursor
	err  error
}

func (r *capturingRepo) Insert(context.Context, domain.TenantScope, notifications.InsertInput) (bool, error) {
	return false, nil
}
func (r *capturingRepo) InsertBatch(context.Context, domain.TenantScope, []notifications.InsertInput) error {
	return nil
}
func (r *capturingRepo) List(_ context.Context, _ domain.TenantScope, _ int64, f notifications.ListFilter) ([]notifications.Notification, *notifications.Cursor, error) {
	r.got = f
	if r.err != nil {
		return nil, nil, r.err
	}
	return []notifications.Notification{{ID: 1}}, r.next, nil
}
func (r *capturingRepo) UnreadCount(context.Context, domain.TenantScope, int64) (int, error) {
	return 0, nil
}
func (r *capturingRepo) MarkRead(context.Context, domain.TenantScope, int64, []int64, bool) error {
	return nil
}
func (r *capturingRepo) PurgeOlderThan(context.Context, int, int) (int64, error) {
	return 0, nil
}

// Курсор непрозрачен для вызывающего: то, что ушло наружу в виде токена, обязано
// вернуться в ту же позицию строки (created_at + id), иначе следующая страница
// начнётся не там. Это же покрытие, что и service/activity's TestCursorRoundTrip,
// для того же класса бага после переноса кодека сюда (IMPORTANT 4 финального ревью).
func TestCursorRoundTrip(t *testing.T) {
	at := time.Date(2026, 3, 4, 5, 6, 7, 891000000, time.UTC)
	token := encodeCursor(&notifications.Cursor{CreatedAt: at, ID: 777})
	if token == "" {
		t.Fatal("пустой токен для непустого курсора")
	}
	back, err := decodeCursor(token)
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if back == nil || !back.CreatedAt.Equal(at) || back.ID != 777 {
		t.Fatalf("курсор не совпал: %+v, want CreatedAt=%v ID=777", back, at)
	}
}

func TestEncodeCursorNilIsEmpty(t *testing.T) {
	if got := encodeCursor(nil); got != "" {
		t.Fatalf("encodeCursor(nil) = %q, want пустую строку", got)
	}
}

// Unlike the activity feed, notifications has always answered a malformed cursor
// with a client-visible 400 (see the handler's pre-existing TestGetInvalidCursorIsBadRequest),
// so decodeCursor here reports an error rather than silently degrading to page one.
func TestDecodeCursorGarbageIsError(t *testing.T) {
	for _, s := range []string{"not-base64!!", "YWJj", "MjAyNi0wMy0wNHwx", "fDQy"} {
		if _, err := decodeCursor(s); err == nil {
			t.Fatalf("decodeCursor(%q) = nil error, want an error", s)
		}
	}
}

// List must reject an unparsable cursor with ErrInvalidCursor before ever reaching
// the repository, and must not silently fall back to the first page: the notification
// bell panel does not currently send a cursor at all, but a hand-crafted request
// carrying a garbage one is a client error, not a cue to start over unannounced.
func TestListInvalidCursorIsError(t *testing.T) {
	repo := &capturingRepo{}
	svc := New(repo)
	_, _, err := svc.List(context.Background(), domain.TenantScope{TenantID: 1}, 42,
		notifications.ListFilter{Limit: 10}, "not-base64!!")
	if !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("err = %v, want ErrInvalidCursor", err)
	}
}

// The cursor the service hands back as NextCursor must decode, through the same
// codec, to exactly the *notifications.Cursor the repo returned — the handler now
// forwards this string opaquely, so this package is the only place left that can
// catch a broken encode/decode pair.
func TestListEncodesNextCursor(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	repo := &capturingRepo{next: &notifications.Cursor{CreatedAt: at, ID: 7}}
	svc := New(repo)
	items, next, err := svc.List(context.Background(), domain.TenantScope{TenantID: 1}, 42,
		notifications.ListFilter{Limit: 10}, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	back, err := decodeCursor(next)
	if err != nil || back == nil || back.ID != 7 {
		t.Fatalf("NextCursor не декодируется в исходный курсор: %q (err=%v)", next, err)
	}
}

// Последняя страница отдаёт пустой cursor-токен, а не base64 от nil.
func TestListLastPageHasEmptyCursor(t *testing.T) {
	repo := &capturingRepo{next: nil}
	svc := New(repo)
	_, next, err := svc.List(context.Background(), domain.TenantScope{TenantID: 1}, 42,
		notifications.ListFilter{Limit: 10}, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if next != "" {
		t.Fatalf("next_cursor = %q, want пустую строку", next)
	}
}

// A round-tripped cursor (encode, then feed back as input) must reach the repository
// decoded to the same position it started from — this is what actually exercises the
// f.Cursor assignment inside List, not just the codec functions in isolation.
func TestListDecodesCursorIntoFilter(t *testing.T) {
	at := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	token := encodeCursor(&notifications.Cursor{CreatedAt: at, ID: 55})

	repo := &capturingRepo{}
	svc := New(repo)
	if _, _, err := svc.List(context.Background(), domain.TenantScope{TenantID: 1}, 42,
		notifications.ListFilter{Limit: 10}, token); err != nil {
		t.Fatalf("List: %v", err)
	}
	if repo.got.Cursor == nil || !repo.got.Cursor.CreatedAt.Equal(at) || repo.got.Cursor.ID != 55 {
		t.Fatalf("filter reaching the repo = %+v, want Cursor{%v, 55}", repo.got.Cursor, at)
	}
}
