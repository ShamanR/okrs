package activity

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"okrs/internal/core/domain"
	storeactivity "okrs/internal/store/activity"
)

// Filter — параметры выборки журнала на уровне сервиса. От store-фильтра отличается
// курсором: наружу отдаётся непрозрачная строка, а не позиция строки в таблице.
// Handler собирает Filter из query-параметров и не знает, чем курсор является внутри.
type Filter struct {
	PeriodID  *int64
	TeamIDs   []int64
	Category  string
	ActorUDID string
	Since     *time.Time
	Query     string
	Limit     int

	// Cursor — токен из NextCursor предыдущей страницы. Пустая строка = первая страница;
	// неразбираемый токен трактуется как первая страница, а не как ошибка: курсор
	// непрозрачен для клиента, и «протухший» токен не должен ронять ленту.
	Cursor string
}

// Page — страница журнала вместе с токеном следующей. NextCursor пуст, когда
// страница последняя.
type Page struct {
	Events     []domain.ActivityEvent
	NextCursor string
}

// Feed отдаёт страницу журнала. allowedTeamIDs == nil — админ/без ограничений.
func (s *Service) Feed(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f Filter) (Page, error) {
	events, next, err := s.repo.List(ctx, scope, allowedTeamIDs, f.toStore())
	if err != nil {
		return Page{}, err
	}
	return Page{Events: events, NextCursor: encodeCursor(next)}, nil
}

// CountByCategory отдаёт счётчики по категориям для вкладок ленты. Категория из
// фильтра намеренно выброшена: счётчики вкладок не должны меняться при выборе вкладки.
func (s *Service) CountByCategory(ctx context.Context, scope domain.TenantScope, allowedTeamIDs []int64, f Filter) (map[string]int, error) {
	sf := f.toStore()
	sf.Category = ""
	sf.Limit = 0
	sf.Cursor = nil
	return s.repo.CategoryCounts(ctx, scope, allowedTeamIDs, sf)
}

func (f Filter) toStore() storeactivity.ListFilter {
	return storeactivity.ListFilter{
		PeriodID:  f.PeriodID,
		TeamIDs:   f.TeamIDs,
		Category:  f.Category,
		ActorUDID: f.ActorUDID,
		Since:     f.Since,
		Query:     f.Query,
		Limit:     f.Limit,
		Cursor:    decodeCursor(f.Cursor),
	}
}

// Курсор кодирует позицию строки (created_at + id) в непрозрачный base64-токен.
// Кодек живёт здесь, а не в handler: его формат — деталь того, как store листает
// таблицу, и handler не должен её знать.

func encodeCursor(c *storeactivity.Cursor) string {
	if c == nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(
		[]byte(c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(c.ID, 10)))
}

func decodeCursor(s string) *storeactivity.Cursor {
	if s == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return nil
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil
	}
	return &storeactivity.Cursor{CreatedAt: ts, ID: id}
}
