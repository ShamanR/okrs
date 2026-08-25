package activity_test

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/store/activity"
	"okrs/internal/store/teams"
	"okrs/internal/store/testutil"
)

func TestPurgeIsTenantScopedAndDepth(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug, name) OVERRIDING SYSTEM VALUE VALUES (2,'t2','T2')`); err != nil {
		t.Fatalf("tenant 2: %v", err)
	}
	s1 := domain.TenantScope{TenantID: 1}
	s2 := domain.TenantScope{TenantID: 2}
	tr := teams.NewTeamRepository(pool)
	ar := activity.NewActivityRepository(pool)
	team1, _ := tr.CreateTeam(ctx, s1, teams.TeamInput{Name: "T1", Type: domain.TeamTypeTeam})

	old := time.Now().Add(-400 * 24 * time.Hour)
	recent := time.Now()
	idOld, _ := ar.Record(ctx, s1, domain.ActivityEvent{ActorUserID: 1, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged, TeamID: &team1})
	_, _ = ar.Record(ctx, s1, domain.ActivityEvent{ActorUserID: 1, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged, TeamID: &team1})
	if _, err := pool.Exec(ctx, `UPDATE activity_events SET created_at=$1 WHERE id=$2`, old, idOld); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	_, _ = ar.Record(ctx, s2, domain.ActivityEvent{ActorUserID: 1, Category: domain.ActivityStatus, Action: domain.ActionStatusChanged})

	cutoff := recent.Add(-90 * 24 * time.Hour)
	n, err := ar.Purge(ctx, s1, &cutoff)
	if err != nil || n != 1 {
		t.Fatalf("purge quarter: n=%d err=%v", n, err)
	}
	if evs, _, _ := ar.List(ctx, s2, nil, activity.ListFilter{}); len(evs) != 1 {
		t.Fatalf("tenant2 leaked: %d", len(evs))
	}
	n, err = ar.Purge(ctx, s1, nil)
	if err != nil || n != 1 {
		t.Fatalf("purge all: n=%d err=%v", n, err)
	}
	if evs, _, _ := ar.List(ctx, s1, nil, activity.ListFilter{}); len(evs) != 0 {
		t.Fatalf("tenant1 not empty: %d", len(evs))
	}
}
