package httpdeps

import (
	"log/slog"

	"okrs/internal/store"
	"okrs/internal/store/grants"

	activitysvc "okrs/internal/service/activity"
	goalsvc "okrs/internal/service/goal"
	goallinksvc "okrs/internal/service/goallink"
	goalsharesvc "okrs/internal/service/goalshare"
	hcsvc "okrs/internal/service/healthcheckin"
	keyresultsvc "okrs/internal/service/keyresult"
	periodsvc "okrs/internal/service/period"
	progresssnapsvc "okrs/internal/service/progresssnap"
	teamsvc "okrs/internal/service/team"
	teamstatussvc "okrs/internal/service/teamstatus"
	usersvc "okrs/internal/service/user"

	exportuc "okrs/internal/usecase/export"
	goaluc "okrs/internal/usecase/goal"
	goaltreeuc "okrs/internal/usecase/goaltree"
	kruc "okrs/internal/usecase/keyresult"
	okrboarduc "okrs/internal/usecase/okrboard"
	perioduc "okrs/internal/usecase/period"
	useruc "okrs/internal/usecase/user"
)

// Deps is the assembled service and usecase graph the handlers are wired from.
//
// Handlers take exactly the members they need — that narrowing is the point of the
// per-URI package split. Until the service facade is removed, both this graph and the
// facade build their own instances of the entity services; that costs nothing (they are
// stateless wrappers over repositories) and disappears with the facade.
type Deps struct {
	Teams    *teamsvc.Service
	Goals    *goalsvc.Service
	Shares   *goalsharesvc.Service
	Links    *goallinksvc.Service
	Statuses *teamstatussvc.Service
	Periods  *periodsvc.Service
	Krs      *keyresultsvc.Service
	Users    *usersvc.Service
	Activity *activitysvc.Service
	Snaps    *progresssnapsvc.Service
	HC       *hcsvc.Service

	Board    *okrboarduc.UseCase
	GoalUC   *goaluc.UseCase
	KrUC     *kruc.UseCase
	PeriodUC *perioduc.UseCase
	TreeUC   *goaltreeuc.UseCase
	ExportUC *exportuc.UseCase
	UserUC   *useruc.UseCase
}

func Build(st *store.Store, grantsCache *grants.GrantsCache, hcCache *hcsvc.Cache, logger *slog.Logger) Deps {
	hc := hcsvc.New(hcCache)
	teams := teamsvc.New(st.Teams)
	goals := goalsvc.New(st.Goals)
	shares := goalsharesvc.New(st.Shares)
	links := goallinksvc.New(st.GoalLinks, goals)
	statuses := teamstatussvc.New(st.Statuses)
	periods := periodsvc.New(st.Periods)
	krs := keyresultsvc.New(st.KRs)
	users := usersvc.New(st.Users)
	activity := activitysvc.New(st.Activity, logger)
	snaps := progresssnapsvc.New(st.ProgressSnap)

	board := okrboarduc.New(okrboarduc.Deps{Teams: teams, Goals: goals, Shares: shares, Statuses: statuses, Links: links})

	return Deps{
		Teams: teams, Goals: goals, Shares: shares, Links: links, Statuses: statuses,
		Periods: periods, Krs: krs, Users: users, Activity: activity, Snaps: snaps, HC: hc,

		Board: board,
		GoalUC: goaluc.New(goaluc.Deps{Goals: goals, Shares: shares, Links: links, Statuses: statuses,
			Periods: periods, Teams: teams, Activity: activity}),
		KrUC: kruc.New(kruc.Deps{KRs: krs, Goals: goals, Activity: activity}),
		PeriodUC: perioduc.New(perioduc.Deps{Periods: periods, Teams: teams, Goals: goals, Statuses: statuses,
			Snaps: snaps, Activity: activity, HCCache: hcCache, Logger: logger}),
		TreeUC:   goaltreeuc.New(goaltreeuc.Deps{Teams: teams, Goals: goals, Links: links, Periods: periods}),
		ExportUC: exportuc.New(exportuc.Deps{Board: board, Teams: teams, Goals: goals, KRs: krs, Periods: periods}),
		UserUC:   useruc.New(useruc.Deps{Users: users, Teams: teams, Grants: grantsCache}),
	}
}
