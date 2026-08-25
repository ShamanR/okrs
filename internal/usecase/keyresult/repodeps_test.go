package keyresult

import (
	activitysvc "okrs/internal/service/activity"
	goalsvc "okrs/internal/service/goal"
	keyresultsvc "okrs/internal/service/keyresult"
	"okrs/internal/service/servicetest"
)

// newTestUC wires the shared in-memory fake into the entity services this usecase
// orchestrates. The migrated tests keep poking the store directly; only wiring moved.
func newTestUC(store *servicetest.Store) *UseCase {
	return New(Deps{
		KRs:      keyresultsvc.New(store),
		Goals:    goalsvc.New(store),
		Activity: activitysvc.New(&servicetest.ActivityRepo{}, nil),
	})
}

// newTestUCWithActivity is for tests that assert what landed in the journal.
func newTestUCWithActivity(store *servicetest.Store, act *servicetest.ActivityRepo) *UseCase {
	return New(Deps{
		KRs:      keyresultsvc.New(store),
		Goals:    goalsvc.New(store),
		Activity: activitysvc.New(act, nil),
	})
}

// newGoalTestService builds the usecase against the call-recording goal fake, which
// also implements the KR repository — KR scenarios read the parent goal.
func newGoalTestService(gf *servicetest.GoalStore) *UseCase {
	return New(Deps{
		KRs:      keyresultsvc.New(gf),
		Goals:    goalsvc.New(gf),
		Activity: activitysvc.New(&servicetest.ActivityRepo{}, nil),
	})
}
