package period

import (
	"log/slog"

	activitysvc "okrs/internal/service/activity"
	goalsvc "okrs/internal/service/goal"
	periodsvc "okrs/internal/service/period"
	progresssnapsvc "okrs/internal/service/progresssnap"
	"okrs/internal/service/servicetest"
	teamsvc "okrs/internal/service/team"
	teamstatussvc "okrs/internal/service/teamstatus"
)

// newTestUC wraps the shared in-memory fake into the entity services this usecase
// orchestrates. Tests keep poking the store directly; only the wiring changed.
func newTestUC(store *servicetest.Store, act *servicetest.ActivityRepo) *UseCase {
	var logger *slog.Logger
	return New(Deps{
		Teams:    teamsvc.New(store),
		Goals:    goalsvc.New(store),
		Statuses: teamstatussvc.New(store),
		Periods:  periodsvc.New(store),
		Snaps:    progresssnapsvc.New(store),
		Activity: activitysvc.New(act, logger),
		Logger:   logger,
	})
}
