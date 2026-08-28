package goal

// TODO(refactoring): rawDeps/newFromRepos — переходный адаптер. Тесты писались, когда
// сценарии ходили в репозитории напрямую, и их Deps были репозиториями; чтобы не
// переписывать ~60 тел, адаптер оборачивает фейки в сервисы. Это честный долг: в
// проде такой обвязки нет, тесты проверяют поведение через слой, которого не существует.
// Убирается переписыванием тестов на Deps с сервисами — отдельная задача, не рефакторинг слоёв.

import (
	goalsvc "okrs/internal/service/goal"
	goallinksvc "okrs/internal/service/goallink"
	goalsharesvc "okrs/internal/service/goalshare"
	periodsvc "okrs/internal/service/period"
	"okrs/internal/service/servicetest"
	teamsvc "okrs/internal/service/team"
	teamstatussvc "okrs/internal/service/teamstatus"
)

// rawDeps mirrors Deps but takes repository fakes instead of entity services.
//
// These tests predate the usecase layer: they were written when scenarios talked to
// repositories directly, and their bodies still poke the fakes to assert what was
// written. Rather than rewrite sixty test bodies, this adapter wraps each fake in the
// service the usecase now expects. What is asserted stays the same; only the wiring moved.
type rawDeps struct {
	Goals    goalsvc.Repo
	Shares   goalsharesvc.Repo
	Links    goallinksvc.Repo
	Statuses teamstatussvc.Repo
	Periods  periodsvc.Repo
	Teams    teamsvc.Repo
	Events   Publisher // defaults to a fresh *servicetest.FakeBus when nil
}

func newFromRepos(d rawDeps) *UseCase {
	deps := Deps{}
	if d.Goals != nil {
		deps.Goals = goalsvc.New(d.Goals)
	}
	if d.Shares != nil {
		deps.Shares = goalsharesvc.New(d.Shares)
	}
	if d.Links != nil {
		deps.Links = goallinksvc.New(d.Links, deps.Goals)
	}
	if d.Statuses != nil {
		deps.Statuses = teamstatussvc.New(d.Statuses)
	}
	if d.Periods != nil {
		deps.Periods = periodsvc.New(d.Periods)
	}
	if d.Teams != nil {
		deps.Teams = teamsvc.New(d.Teams)
	}
	// Events is always present: scenarios publish unconditionally, and a nil
	// Publisher would panic (method call on a nil interface) rather than no-op.
	deps.Events = d.Events
	if deps.Events == nil {
		deps.Events = &servicetest.FakeBus{}
	}
	return New(deps)
}

// newGoalTestService builds the usecase against the call-recording goal fake.
func newGoalTestService(gf *servicetest.GoalStore) *UseCase {
	return newFromRepos(rawDeps{Teams: gf, Goals: gf, Shares: gf, Periods: gf, Statuses: gf})
}
