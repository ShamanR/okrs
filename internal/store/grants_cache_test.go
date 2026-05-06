package store

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// fakeGrantsBackend is a controllable backend for testing GrantsCache.
type fakeGrantsBackend struct {
	data      map[int64][]HierarchyGrant
	loadCount atomic.Int64
	addCalls  []addGrantCall
	rmCalls   []removeGrantCall
}

type addGrantCall struct{ userID, teamID, grantedByUserID int64 }
type removeGrantCall struct{ userID, teamID int64 }

func (f *fakeGrantsBackend) loadAllGrants(_ context.Context) (map[int64][]HierarchyGrant, error) {
	f.loadCount.Add(1)
	cp := make(map[int64][]HierarchyGrant, len(f.data))
	for k, v := range f.data {
		cp[k] = append([]HierarchyGrant(nil), v...)
	}
	return cp, nil
}
func (f *fakeGrantsBackend) addUserGrant(_ context.Context, userID, teamID, grantedByUserID int64) error {
	f.addCalls = append(f.addCalls, addGrantCall{userID, teamID, grantedByUserID})
	f.data[userID] = append(f.data[userID], HierarchyGrant{UserID: userID, TeamID: teamID, CreatedByUserID: grantedByUserID})
	return nil
}
func (f *fakeGrantsBackend) removeUserGrant(_ context.Context, userID, teamID int64) error {
	f.rmCalls = append(f.rmCalls, removeGrantCall{userID, teamID})
	grants := f.data[userID]
	filtered := grants[:0]
	for _, g := range grants {
		if g.TeamID != teamID {
			filtered = append(filtered, g)
		}
	}
	f.data[userID] = filtered
	return nil
}
func (f *fakeGrantsBackend) ListDescendantTeamIDs(_ context.Context, rootIDs []int64) ([]int64, error) {
	return rootIDs, nil
}

func newFakeBackend(data map[int64][]HierarchyGrant) *fakeGrantsBackend {
	if data == nil {
		data = make(map[int64][]HierarchyGrant)
	}
	return &fakeGrantsBackend{data: data}
}

func TestGrantsCacheListUserGrantsCachesData(t *testing.T) {
	backend := newFakeBackend(map[int64][]HierarchyGrant{
		1: {{ID: 10, UserID: 1, TeamID: 5}},
	})
	cache := newGrantsCacheWithBackend(backend, time.Minute)
	ctx := context.Background()

	grants1, err := cache.ListUserGrants(ctx, 1)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if len(grants1) != 1 {
		t.Fatalf("expected 1 grant, got %d", len(grants1))
	}

	// Second call must not hit backend again.
	_, err = cache.ListUserGrants(ctx, 1)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if n := backend.loadCount.Load(); n != 1 {
		t.Fatalf("expected 1 backend load, got %d", n)
	}
}

func TestGrantsCacheRefreshesAfterTTL(t *testing.T) {
	backend := newFakeBackend(map[int64][]HierarchyGrant{
		1: {{ID: 10, UserID: 1, TeamID: 5}},
	})
	cache := newGrantsCacheWithBackend(backend, 10*time.Millisecond)
	ctx := context.Background()

	if _, err := cache.ListUserGrants(ctx, 1); err != nil {
		t.Fatalf("first call: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	if _, err := cache.ListUserGrants(ctx, 1); err != nil {
		t.Fatalf("after TTL: %v", err)
	}
	if n := backend.loadCount.Load(); n < 2 {
		t.Fatalf("expected ≥2 backend loads after TTL expiry, got %d", n)
	}
}

func TestGrantsCacheInvalidatesOnAddGrant(t *testing.T) {
	backend := newFakeBackend(nil)
	cache := newGrantsCacheWithBackend(backend, time.Minute)
	ctx := context.Background()

	// Warm cache.
	if _, err := cache.AllGrants(ctx); err != nil {
		t.Fatalf("warm: %v", err)
	}
	if n := backend.loadCount.Load(); n != 1 {
		t.Fatalf("expected 1 load after warm, got %d", n)
	}

	if err := cache.AddUserGrant(ctx, 2, 7, 1); err != nil {
		t.Fatalf("add grant: %v", err)
	}
	if len(backend.addCalls) != 1 {
		t.Fatalf("expected 1 add call to backend, got %d", len(backend.addCalls))
	}

	// Next read must reload from backend.
	grants, err := cache.ListUserGrants(ctx, 2)
	if err != nil {
		t.Fatalf("after add: %v", err)
	}
	if len(grants) != 1 || grants[0].TeamID != 7 {
		t.Fatalf("expected grant to team 7, got %+v", grants)
	}
	if n := backend.loadCount.Load(); n != 2 {
		t.Fatalf("expected 2 backend loads after invalidation, got %d", n)
	}
}

func TestGrantsCacheInvalidatesOnRemoveGrant(t *testing.T) {
	backend := newFakeBackend(map[int64][]HierarchyGrant{
		3: {{ID: 20, UserID: 3, TeamID: 9}},
	})
	cache := newGrantsCacheWithBackend(backend, time.Minute)
	ctx := context.Background()

	// Warm cache.
	if _, err := cache.AllGrants(ctx); err != nil {
		t.Fatalf("warm: %v", err)
	}

	if err := cache.RemoveUserGrant(ctx, 3, 9); err != nil {
		t.Fatalf("remove grant: %v", err)
	}
	if len(backend.rmCalls) != 1 {
		t.Fatalf("expected 1 remove call to backend, got %d", len(backend.rmCalls))
	}

	grants, err := cache.ListUserGrants(ctx, 3)
	if err != nil {
		t.Fatalf("after remove: %v", err)
	}
	if len(grants) != 0 {
		t.Fatalf("expected no grants after removal, got %+v", grants)
	}
}

func TestGrantsCacheAllGrantsReturnsFullSnapshot(t *testing.T) {
	backend := newFakeBackend(map[int64][]HierarchyGrant{
		1: {{ID: 1, UserID: 1, TeamID: 10}},
		2: {{ID: 2, UserID: 2, TeamID: 11}, {ID: 3, UserID: 2, TeamID: 12}},
	})
	cache := newGrantsCacheWithBackend(backend, time.Minute)
	ctx := context.Background()

	all, err := cache.AllGrants(ctx)
	if err != nil {
		t.Fatalf("all grants: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 users in snapshot, got %d", len(all))
	}
	if len(all[2]) != 2 {
		t.Fatalf("expected 2 grants for user 2, got %d", len(all[2]))
	}
}

func TestGrantsCacheListUserGrantsReturnsEmptyForUnknownUser(t *testing.T) {
	cache := newGrantsCacheWithBackend(newFakeBackend(nil), time.Minute)
	ctx := context.Background()

	grants, err := cache.ListUserGrants(ctx, 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if grants != nil {
		t.Fatalf("expected nil for unknown user, got %+v", grants)
	}
}
