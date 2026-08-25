package domain

import "errors"

// Доменные инварианты, пересекающие границу сущности. Живут здесь, а не в service,
// потому что на них смотрят и сервисы, и usecase, и handlers при маппинге в HTTP-коды;
// после распила service держать их там означало бы размазать по 13 пакетам.
var (
	ErrTeamHasGoals                = errors.New("team has goals")
	ErrTeamNotVisibleInPeriod      = errors.New("team not visible in period")
	ErrPeriodClosed                = errors.New("period is closed")
	ErrPeriodNotClosed             = errors.New("period must be closed to archive")
	ErrCannotShareWithClosedPeriod = errors.New("cannot share goal with team whose period is in_progress or closed")
	ErrShareTargetNotInTenant      = errors.New("share target team is not in the active tenant")
	ErrTransferTargetSameAsSource  = errors.New("transfer target equals source team and period")
	ErrTransferTargetNotFound      = errors.New("transfer target team or period not found in tenant")
	// ErrForbidden signals an authorization failure the handler maps to HTTP 403.
	ErrForbidden = errors.New("forbidden")
	// ErrGoalNotOnTeamBoard signals a goal-scope export where the goal is not on the context team's board.
	ErrGoalNotOnTeamBoard = errors.New("goal not on team board")
	// Goal-link errors mapped by handlers to 400 (self/not accessible) and 409 (cycle).
	ErrGoalLinkSelf          = errors.New("goal cannot link to itself")
	ErrGoalLinkNotAccessible = errors.New("parent goal not accessible")
	ErrGoalLinkCycle         = errors.New("goal link would create a cycle")
)
