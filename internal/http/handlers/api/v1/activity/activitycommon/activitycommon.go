// Package activitycommon holds what the /api/v1/activity endpoints share on the HTTP
// side: разбор query-параметров в сервисный фильтр и сборка ответа из доменных
// событий. Лист-пакет — по той же причине, что teamscommon: родитель монтирует
// подпакеты, поэтому подпакет не может импортировать его в обратную сторону.
//
// Сырых данных здесь нет: курсор — непрозрачная строка, его кодек и позиция строки
// в таблице живут в service/activity. Пакет не импортирует store.
package activitycommon

import (
	"net/http"
	"strconv"
	"time"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/dto"
	activitysvc "okrs/internal/service/activity"
)

// DefaultLimit — размер страницы ленты, если клиент не задал свой.
const DefaultLimit = 50

// ParseFilter собирает сервисный фильтр из query-параметров. Разбор намеренно
// снисходителен: неразбираемое число или неизвестный range означают «фильтр не
// задан», а не 400 — лента открывается по ссылке с чужими параметрами и не должна
// падать из-за мусора в URL.
//
// now передаётся явно, чтобы диапазоны (today/7d/30d) были проверяемы тестом.
func ParseFilter(r *http.Request, now time.Time) activitysvc.Filter {
	q := r.URL.Query()
	var teamIDs []int64
	for _, s := range q["team_ids"] {
		if p := ParseInt64(s); p != nil {
			teamIDs = append(teamIDs, *p)
		}
	}
	limit := DefaultLimit
	if p := ParseInt64(q.Get("limit")); p != nil && *p > 0 {
		limit = int(*p)
	}
	return activitysvc.Filter{
		PeriodID:  ParseInt64(q.Get("period_id")),
		TeamIDs:   teamIDs,
		Category:  q.Get("category"),
		ActorUDID: q.Get("actor_udid"),
		Since:     SinceFromRange(q.Get("range"), now),
		Query:     q.Get("q"),
		Limit:     limit,
		Cursor:    q.Get("cursor"),
	}
}

// BuildTarget resolves navigation. For v1 every target routes to the tracker board of the
// event's recorded team (owner/context team), which is always accessible to any viewer who can
// see the event. Events without a team have no navigable target.
func BuildTarget(ev domain.ActivityEvent) *dto.ActivityTarget {
	// Use the viewer-accessible target team resolved by the store (owner if accessible, else an
	// accessible shared team). A viewer can see a shared-goal event without owner-team access, so
	// linking to the owner board would open an inaccessible/empty page.
	teamID := ev.TargetTeamID
	if teamID == nil {
		teamID = ev.TeamID
	}
	if teamID == nil {
		return nil
	}
	return &dto.ActivityTarget{
		Section: "tracker", TeamID: *teamID, PeriodID: ev.PeriodID,
		GoalID: ev.GoalID, KRID: ev.KRID, CommentID: ev.CommentID,
	}
}

// FeedResponse собирает ответ ленты из готовой страницы сервиса.
func FeedResponse(page activitysvc.Page) dto.ActivityFeedResponse {
	items := make([]dto.ActivityEvent, 0, len(page.Events))
	for _, ev := range page.Events {
		items = append(items, dto.ActivityEvent{
			ID: ev.ID, Category: string(ev.Category), Action: string(ev.Action),
			Actor:       dto.ActivityActor{UDID: ev.ActorUDID, DisplayName: ev.ActorDisplayName, AvatarURL: ev.ActorAvatarURL, Removed: ev.ActorRemoved},
			TeamID:      ev.TeamID,
			PeriodID:    ev.PeriodID,
			GoalID:      ev.GoalID,
			KRID:        ev.KRID,
			CommentID:   ev.CommentID,
			EntityTitle: ev.EntityTitle,
			Target:      BuildTarget(ev),
			Payload:     ev.Payload,
			CreatedAt:   ev.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return dto.ActivityFeedResponse{Items: items, NextCursor: page.NextCursor}
}

// ScopeTeams returns the allowed team ids for the request. nil => admin/unrestricted (or scope
// not loaded, e.g. auth disabled). A non-nil slice (incl. empty) restricts visibility; an empty
// slice fails closed (no access).
func ScopeTeams(r *http.Request) []int64 {
	allowed, ok := auth.AllowedTeamIDsFromCtx(r.Context())
	if ok && allowed != nil {
		return allowed
	}
	return nil
}

// ParseInt64 возвращает nil на пустой или неразбираемой строке — «фильтр не задан».
func ParseInt64(s string) *int64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

// SinceFromRange переводит значение параметра range в нижнюю границу времени.
// nil — без ограничения ("all", пусто или неизвестное значение).
func SinceFromRange(rng string, now time.Time) *time.Time {
	switch rng {
	case "today":
		t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return &t
	case "7d":
		t := now.Add(-7 * 24 * time.Hour)
		return &t
	case "30d":
		t := now.Add(-30 * 24 * time.Hour)
		return &t
	default: // "all" or empty
		return nil
	}
}
