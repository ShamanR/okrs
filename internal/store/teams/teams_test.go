package teams_test

import (
	"context"
	"testing"

	"okrs/internal/domain"
	"okrs/internal/store/teams"
	"okrs/internal/store/testutil"
)

func TestTeamsCRUD(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := teams.NewTeamRepository(pool)

	id, err := r.CreateTeam(ctx, teams.TeamInput{
		Name: "Alpha", Type: domain.TeamTypeTeam,
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}

	team, err := r.GetTeam(ctx, id)
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if team.Name != "Alpha" || team.Type != domain.TeamTypeTeam {
		t.Fatalf("unexpected team %+v", team)
	}

	if err := r.UpdateTeam(ctx, teams.TeamInput{Name: "Alpha 2", Type: domain.TeamTypeUnit}, id); err != nil {
		t.Fatalf("UpdateTeam: %v", err)
	}
	team, _ = r.GetTeam(ctx, id)
	if team.Name != "Alpha 2" || team.Type != domain.TeamTypeUnit {
		t.Fatalf("expected updated team, got %+v", team)
	}

	list, err := r.ListTeams(ctx)
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	found := false
	for _, tm := range list {
		if tm.ID == id {
			found = true
		}
	}
	if !found {
		t.Fatalf("created team not found in ListTeams")
	}
}

func TestSoftDeleteReparentsChildren(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := teams.NewTeamRepository(pool)

	parentID, _ := r.CreateTeam(ctx, teams.TeamInput{Name: "Parent", Type: domain.TeamTypeUnit})
	midID, _ := r.CreateTeam(ctx, teams.TeamInput{Name: "Mid", Type: domain.TeamTypeTeam, ParentID: &parentID})
	childID, _ := r.CreateTeam(ctx, teams.TeamInput{Name: "Child", Type: domain.TeamTypeTeam, ParentID: &midID})

	if err := r.SoftDeleteTeam(ctx, midID); err != nil {
		t.Fatalf("SoftDeleteTeam: %v", err)
	}

	// Mid should be soft-deleted (DeletedAt set).
	mid, _ := r.GetTeam(ctx, midID)
	if mid.DeletedAt == nil {
		t.Fatal("expected DeletedAt to be set on soft-deleted team")
	}

	// Child should be reparented to Parent.
	child, _ := r.GetTeam(ctx, childID)
	if child.ParentID == nil || *child.ParentID != parentID {
		t.Fatalf("expected child to be reparented to %d, got %v", parentID, child.ParentID)
	}

	// Soft-deleted team appears in ListDeletedTeams but not ListTeams.
	deleted, _ := r.ListDeletedTeams(ctx)
	foundDeleted := false
	for _, tm := range deleted {
		if tm.ID == midID {
			foundDeleted = true
		}
	}
	if !foundDeleted {
		t.Fatal("expected mid team in ListDeletedTeams")
	}
	active, _ := r.ListTeams(ctx)
	for _, tm := range active {
		if tm.ID == midID {
			t.Fatal("soft-deleted team must not appear in ListTeams")
		}
	}

	// Restore puts it back.
	if err := r.RestoreTeam(ctx, midID); err != nil {
		t.Fatalf("RestoreTeam: %v", err)
	}
	mid, _ = r.GetTeam(ctx, midID)
	if mid.DeletedAt != nil {
		t.Fatal("expected DeletedAt to be nil after restore")
	}
}

func TestHardDeleteReparentsChildren(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := teams.NewTeamRepository(pool)

	parentID, _ := r.CreateTeam(ctx, teams.TeamInput{Name: "HParent", Type: domain.TeamTypeUnit})
	midID, _ := r.CreateTeam(ctx, teams.TeamInput{Name: "HMid", Type: domain.TeamTypeTeam, ParentID: &parentID})
	childID, _ := r.CreateTeam(ctx, teams.TeamInput{Name: "HChild", Type: domain.TeamTypeTeam, ParentID: &midID})

	if err := r.HardDeleteTeam(ctx, midID); err != nil {
		t.Fatalf("HardDeleteTeam: %v", err)
	}

	// Mid should be gone.
	if _, err := r.GetTeam(ctx, midID); err == nil {
		t.Fatal("expected error fetching hard-deleted team")
	}

	// Child should be reparented to Parent.
	child, err := r.GetTeam(ctx, childID)
	if err != nil {
		t.Fatalf("GetTeam child: %v", err)
	}
	if child.ParentID == nil || *child.ParentID != parentID {
		t.Fatalf("expected reparented to %d, got %v", parentID, child.ParentID)
	}
}

func TestListAllTeamsIncludesDeleted(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	r := teams.NewTeamRepository(pool)

	id, _ := r.CreateTeam(ctx, teams.TeamInput{Name: "AllTeam", Type: domain.TeamTypeTeam})
	r.SoftDeleteTeam(ctx, id)

	all, err := r.ListAllTeams(ctx)
	if err != nil {
		t.Fatalf("ListAllTeams: %v", err)
	}
	found := false
	for _, tm := range all {
		if tm.ID == id {
			found = true
		}
	}
	if !found {
		t.Fatal("expected soft-deleted team in ListAllTeams")
	}
}
