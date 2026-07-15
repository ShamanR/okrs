package activity

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/dto"
	storeactivity "okrs/internal/store/activity"
)

func encodeCursor(c *storeactivity.Cursor) string {
	if c == nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(c.ID, 10)))
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

// buildTarget resolves navigation. For v1 every target routes to the tracker board of the
// event's recorded team (owner/context team), which is always accessible to any viewer who can
// see the event. Events without a team have no navigable target.
func buildTarget(ev domain.ActivityEvent) *dto.ActivityTarget {
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

func newFeedResponse(events []domain.ActivityEvent, next *storeactivity.Cursor) dto.ActivityFeedResponse {
	items := make([]dto.ActivityEvent, 0, len(events))
	for _, ev := range events {
		items = append(items, dto.ActivityEvent{
			ID: ev.ID, Category: string(ev.Category), Action: string(ev.Action),
			Actor:       dto.ActivityActor{UDID: ev.ActorUDID, DisplayName: ev.ActorDisplayName, AvatarURL: ev.ActorAvatarURL, Removed: ev.ActorRemoved},
			TeamID:      ev.TeamID,
			PeriodID:    ev.PeriodID,
			GoalID:      ev.GoalID,
			KRID:        ev.KRID,
			CommentID:   ev.CommentID,
			EntityTitle: ev.EntityTitle,
			Target:      buildTarget(ev),
			Payload:     ev.Payload,
			CreatedAt:   ev.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return dto.ActivityFeedResponse{Items: items, NextCursor: encodeCursor(next)}
}
