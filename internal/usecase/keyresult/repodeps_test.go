package keyresult

import (
	goalsvc "okrs/internal/service/goal"
	keyresultsvc "okrs/internal/service/keyresult"
	"okrs/internal/service/servicetest"
)

// ptr — общий по пакету хелпер для тестов, которые зовут CheckIn напрямую и
// строят CheckInInput с указателем на литерал (после удаления однополевых обёрток
// UpdateProgressNumerical/Boolean/Project и UpsertNote это единственный способ
// получить *float64/*bool/*string из значения в одну строку).
func ptr[T any](v T) *T { return &v }

// newTestUC wires the shared in-memory fake into the entity services this usecase
// orchestrates. The migrated tests keep poking the store directly; only wiring moved.
func newTestUC(store *servicetest.Store) *UseCase {
	return New(Deps{
		KRs:    keyresultsvc.New(store),
		Goals:  goalsvc.New(store),
		Events: &servicetest.FakeBus{},
	})
}

// newTestUCWithBus is for tests that assert on the events published.
func newTestUCWithBus(store *servicetest.Store, bus *servicetest.FakeBus) *UseCase {
	return New(Deps{
		KRs:    keyresultsvc.New(store),
		Goals:  goalsvc.New(store),
		Events: bus,
	})
}

// newGoalTestService builds the usecase against the call-recording goal fake, which
// also implements the KR repository — KR scenarios read the parent goal.
func newGoalTestService(gf *servicetest.GoalStore) *UseCase {
	return New(Deps{
		KRs:    keyresultsvc.New(gf),
		Goals:  goalsvc.New(gf),
		Events: &servicetest.FakeBus{},
	})
}
