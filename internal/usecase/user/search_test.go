package user

// Тесты переехали из internal/service при выделении слоя usecase.

import (
	"context"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/service/servicetest"
	teamsvc "okrs/internal/service/team"
	usersvc "okrs/internal/service/user"
	"okrs/internal/store/grants"
)

// fakeGrantsProvider implements GrantsProvider for tests.
type fakeGrantsProvider struct {
	data map[int64][]grants.HierarchyGrant
}

func (f *fakeGrantsProvider) AllGrants(_ context.Context) (map[int64][]grants.HierarchyGrant, error) {
	return f.data, nil
}

// searchCapturingStore embeds servicetest.Store and captures SearchUsersInSet/SearchUsersUnrestricted calls.
type searchCapturingStore struct {
	*servicetest.Store
	unrestrictedCalls int
	lastUserIDs       []int64
	lastLeadNames     []string
	returnUsers       []*domain.User
}

func (s *searchCapturingStore) SearchUsersUnrestricted(_ context.Context, _ string, _ int) ([]*domain.User, error) {
	s.unrestrictedCalls++
	return s.returnUsers, nil
}

func (s *searchCapturingStore) SearchUsersInSet(_ context.Context, userIDs []int64, leadNames []string, _ string, _ int) ([]*domain.User, error) {
	s.lastUserIDs = userIDs
	s.lastLeadNames = leadNames
	return s.returnUsers, nil
}

func newSearchStore() *searchCapturingStore {
	return &searchCapturingStore{Store: servicetest.NewStore()}
}

func newSearchTestService(st *searchCapturingStore, grants GrantsProvider) *UseCase {
	return New(Deps{Users: usersvc.New(st), Teams: teamsvc.New(st), Grants: grants})
}

func TestSearchUsersInScopeUnrestricted(t *testing.T) {
	st := newSearchStore()
	st.returnUsers = []*domain.User{{ID: 1, DisplayName: "Alice"}}
	svc := newSearchTestService(st, &fakeGrantsProvider{data: nil})

	// nil scopeTeamIDs = admin / unrestricted
	users, err := svc.SearchInScope(context.Background(), domain.TenantScope{TenantID: 1}, nil, "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.unrestrictedCalls != 1 {
		t.Fatalf("expected SearchUsersUnrestricted to be called once, got %d", st.unrestrictedCalls)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
}

func TestSearchUsersInScopeEmptyGrantsReturnsNil(t *testing.T) {
	st := newSearchStore()
	svc := newSearchTestService(st, &fakeGrantsProvider{data: make(map[int64][]grants.HierarchyGrant)})

	// empty scope slice = user with no grants
	users, err := svc.SearchInScope(context.Background(), domain.TenantScope{TenantID: 1}, []int64{}, "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if users != nil {
		t.Fatalf("expected nil for empty scope, got %+v", users)
	}
}

func TestSearchUsersInScopeFiltersGrantsByAncestor(t *testing.T) {
	// Tree: root(1) → child(2) → grandchild(3)
	st := newSearchStore()
	st.Teams = []domain.Team{
		{ID: 1, Name: "Root", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Child", Type: domain.TeamTypeTeam, ParentID: ptr(1)},
		{ID: 3, Name: "Grandchild", Type: domain.TeamTypeTeam, ParentID: ptr(2)},
	}

	grants := map[int64][]grants.HierarchyGrant{
		// user 10 has a grant to root (ID=1) — covers all scope nodes
		10: {{ID: 1, UserID: 10, TeamID: 1}},
		// user 20 has a grant to an unrelated team (ID=99) — should be excluded
		20: {{ID: 2, UserID: 20, TeamID: 99}},
	}
	st.returnUsers = []*domain.User{{ID: 10, DisplayName: "Root granter"}}
	svc := newSearchTestService(st, &fakeGrantsProvider{data: grants})

	// scope = [3] (grandchild); ancestors are {3, 2, 1}
	_, err := svc.SearchInScope(context.Background(), domain.TenantScope{TenantID: 1}, []int64{3}, "", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(st.lastUserIDs) != 1 || st.lastUserIDs[0] != 10 {
		t.Fatalf("expected user 10 (root granter) to be eligible, got %v", st.lastUserIDs)
	}
}

func TestSearchUsersInScopeIncludesTeamLeads(t *testing.T) {
	aliceUDID := "udid-alice"
	st := newSearchStore()
	st.Teams = []domain.Team{
		{ID: 5, Name: "TeamA", Type: domain.TeamTypeTeam, Lead: "Alice", LeadUDID: &aliceUDID},
	}
	// No grants at all — but Alice is a lead of scope team 5.
	svc := newSearchTestService(st, &fakeGrantsProvider{data: make(map[int64][]grants.HierarchyGrant)})

	_, err := svc.SearchInScope(context.Background(), domain.TenantScope{TenantID: 1}, []int64{5}, "", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(st.lastLeadNames) != 1 || st.lastLeadNames[0] != aliceUDID {
		t.Fatalf("expected lead UDID %q in lead UDIDs, got %v", aliceUDID, st.lastLeadNames)
	}
}

func TestSearchUsersInScopeFiltersGrantsByDescendant(t *testing.T) {
	// Tree: root(1) → child(2) → grandchild(3)
	st := newSearchStore()
	st.Teams = []domain.Team{
		{ID: 1, Name: "Root", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Child", Type: domain.TeamTypeTeam, ParentID: ptr(1)},
		{ID: 3, Name: "Grandchild", Type: domain.TeamTypeTeam, ParentID: ptr(2)},
	}

	grants := map[int64][]grants.HierarchyGrant{
		// user 30 has a grant only to grandchild(3) — a descendant of scope node 1
		30: {{ID: 3, UserID: 30, TeamID: 3}},
		// user 20 has a grant to an unrelated team — excluded
		20: {{ID: 2, UserID: 20, TeamID: 99}},
	}
	st.returnUsers = []*domain.User{{ID: 30, DisplayName: "Grandchild granter"}}
	svc := newSearchTestService(st, &fakeGrantsProvider{data: grants})

	// scope = [1] (root); descendants are {2, 3}
	_, err := svc.SearchInScope(context.Background(), domain.TenantScope{TenantID: 1}, []int64{1}, "", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(st.lastUserIDs) != 1 || st.lastUserIDs[0] != 30 {
		t.Fatalf("expected user 30 (grandchild granter) to be eligible via descendant, got %v", st.lastUserIDs)
	}
}

func TestSearchUsersInScopeIncludesDescendantTeamLeads(t *testing.T) {
	charlieUDID := "udid-charlie"
	st := newSearchStore()
	st.Teams = []domain.Team{
		{ID: 1, Name: "Parent", Type: domain.TeamTypeUnit},
		{ID: 2, Name: "Child", Type: domain.TeamTypeTeam, ParentID: ptr(1), Lead: "Charlie", LeadUDID: &charlieUDID},
	}
	svc := newSearchTestService(st, &fakeGrantsProvider{data: make(map[int64][]grants.HierarchyGrant)})

	// scope = [1] (parent); child is a descendant with lead Charlie
	_, err := svc.SearchInScope(context.Background(), domain.TenantScope{TenantID: 1}, []int64{1}, "", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, udid := range st.lastLeadNames {
		if udid == charlieUDID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected descendant lead UDID %q in lead UDIDs, got %v", charlieUDID, st.lastLeadNames)
	}
}

func TestSearchUsersInScopeExcludesDeletedTeamLeads(t *testing.T) {
	deletedAt := time.Now()
	st := newSearchStore()
	st.Teams = []domain.Team{
		{ID: 5, Name: "Deleted", Type: domain.TeamTypeTeam, Lead: "Bob", DeletedAt: &deletedAt},
	}
	svc := newSearchTestService(st, &fakeGrantsProvider{data: make(map[int64][]grants.HierarchyGrant)})

	_, err := svc.SearchInScope(context.Background(), domain.TenantScope{TenantID: 1}, []int64{5}, "", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(st.lastLeadNames) != 0 {
		t.Fatalf("expected no lead names for deleted team, got %v", st.lastLeadNames)
	}
}

func ptr(id int64) *int64 { return &id }
