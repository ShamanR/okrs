package teams_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/http/handlers/api/v1/testutil"
	"okrs/internal/service"
	"okrs/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestDeletedTeamsVisibilityDependsOnPeriodIntegration(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.RunContainer(ctx,
		tcpostgres.WithDatabase("okrs"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second),
		),
	)
	testutil.RequireDockerOrSkip(t, err)
	defer func() { _ = container.Terminate(ctx) }()

	dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}
	if err := testutil.RunMigrations(dbURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	repo := store.New(pool)
	var deletedTeamID, activeTeamID, currentPeriodID, historyPeriodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type) VALUES ('Deleted team', 'team') RETURNING id`).Scan(&deletedTeamID); err != nil {
		t.Fatalf("insert deleted team: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type) VALUES ('Active team', 'team') RETURNING id`).Scan(&activeTeamID); err != nil {
		t.Fatalf("insert active team: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date, sort_order)
		VALUES ('History', '2025-01-01', '2025-03-31', 1)
		RETURNING id`).Scan(&historyPeriodID); err != nil {
		t.Fatalf("insert history period: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date, sort_order)
		VALUES ('Current', CURRENT_DATE - INTERVAL '5 day', CURRENT_DATE + INTERVAL '5 day', 2)
		RETURNING id`).Scan(&currentPeriodID); err != nil {
		t.Fatalf("insert current period: %v", err)
	}

	if _, err := repo.CreateGoal(ctx, store.GoalInput{
		TeamID:      deletedTeamID,
		PeriodID:    historyPeriodID,
		Title:       "History goal",
		Description: "desc",
		Priority:    domain.PriorityP1,
		Weight:      100,
		WorkType:    domain.WorkTypeDelivery,
		FocusType:   domain.FocusStability,
		OwnerText:   "Owner",
	}); err != nil {
		t.Fatalf("create historical goal: %v", err)
	}
	if err := repo.SoftDeleteTeam(ctx, deletedTeamID); err != nil {
		t.Fatalf("soft delete team: %v", err)
	}

	svc := service.New(repo)
	server := httptest.NewServer(testutil.NewAPIV1Router(svc))
	defer server.Close()

	currentHierarchyResp, err := http.Get(fmt.Sprintf("%s/api/v1/hierarchy?period_id=%d", server.URL, currentPeriodID))
	if err != nil {
		t.Fatalf("get current hierarchy: %v", err)
	}
	defer currentHierarchyResp.Body.Close()
	if currentHierarchyResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for current hierarchy, got %d", currentHierarchyResp.StatusCode)
	}
	var currentHierarchy hierarchyResponse
	if err := json.NewDecoder(currentHierarchyResp.Body).Decode(&currentHierarchy); err != nil {
		t.Fatalf("decode current hierarchy: %v", err)
	}
	currentHierarchyIDs := flattenHierarchyIDs(currentHierarchy.Items)
	if _, ok := currentHierarchyIDs[deletedTeamID]; ok {
		t.Fatalf("expected deleted team to be hidden from current hierarchy without goals, got %+v", currentHierarchyIDs)
	}
	if _, ok := currentHierarchyIDs[activeTeamID]; !ok {
		t.Fatalf("expected active team in current hierarchy, got %+v", currentHierarchyIDs)
	}
	activeCurrentNode := findHierarchyNodeByID(currentHierarchy.Items, activeTeamID)
	if activeCurrentNode == nil {
		t.Fatalf("expected to find active team node in current hierarchy")
	}
	if activeCurrentNode.HasGoals {
		t.Fatalf("expected active team without goals to have has_goals=false, got true")
	}
	if activeCurrentNode.Progress != nil {
		t.Fatalf("expected active team without goals to not have progress, got %v", *activeCurrentNode.Progress)
	}

	historyHierarchyResp, err := http.Get(fmt.Sprintf("%s/api/v1/hierarchy?period_id=%d", server.URL, historyPeriodID))
	if err != nil {
		t.Fatalf("get history hierarchy: %v", err)
	}
	defer historyHierarchyResp.Body.Close()
	if historyHierarchyResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for history hierarchy, got %d", historyHierarchyResp.StatusCode)
	}
	var historyHierarchy hierarchyResponse
	if err := json.NewDecoder(historyHierarchyResp.Body).Decode(&historyHierarchy); err != nil {
		t.Fatalf("decode history hierarchy: %v", err)
	}
	historyHierarchyIDs := flattenHierarchyIDs(historyHierarchy.Items)
	if _, ok := historyHierarchyIDs[deletedTeamID]; !ok {
		t.Fatalf("expected deleted team with historical goals in hierarchy, got %+v", historyHierarchyIDs)
	}
	if _, ok := historyHierarchyIDs[activeTeamID]; !ok {
		t.Fatalf("expected active team in history hierarchy, got %+v", historyHierarchyIDs)
	}

	okrResp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", server.URL, deletedTeamID, historyPeriodID))
	if err != nil {
		t.Fatalf("get historical okr: %v", err)
	}
	defer okrResp.Body.Close()
	if okrResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for historical deleted team, got %d", okrResp.StatusCode)
	}

	activeHistoryResp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", server.URL, activeTeamID, historyPeriodID))
	if err != nil {
		t.Fatalf("get active team history okr: %v", err)
	}
	defer activeHistoryResp.Body.Close()
	if activeHistoryResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for active team without historical goals, got %d", activeHistoryResp.StatusCode)
	}

	currentDeletedResp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", server.URL, deletedTeamID, currentPeriodID))
	if err != nil {
		t.Fatalf("get current deleted team okr: %v", err)
	}
	defer currentDeletedResp.Body.Close()
	if currentDeletedResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for deleted team in current period, got %d", currentDeletedResp.StatusCode)
	}

	if _, err := repo.CreateGoal(ctx, store.GoalInput{
		TeamID:      deletedTeamID,
		PeriodID:    currentPeriodID,
		Title:       "Current goal",
		Description: "desc",
		Priority:    domain.PriorityP1,
		Weight:      100,
		WorkType:    domain.WorkTypeDelivery,
		FocusType:   domain.FocusStability,
		OwnerText:   "Owner",
	}); err != nil {
		t.Fatalf("create current goal: %v", err)
	}

	currentHierarchyAfterGoalResp, err := http.Get(fmt.Sprintf("%s/api/v1/hierarchy?period_id=%d", server.URL, currentPeriodID))
	if err != nil {
		t.Fatalf("get current hierarchy after goal: %v", err)
	}
	defer currentHierarchyAfterGoalResp.Body.Close()
	if currentHierarchyAfterGoalResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for current hierarchy after goal, got %d", currentHierarchyAfterGoalResp.StatusCode)
	}
	var currentHierarchyAfterGoal hierarchyResponse
	if err := json.NewDecoder(currentHierarchyAfterGoalResp.Body).Decode(&currentHierarchyAfterGoal); err != nil {
		t.Fatalf("decode current hierarchy after goal: %v", err)
	}
	currentHierarchyAfterGoalIDs := flattenHierarchyIDs(currentHierarchyAfterGoal.Items)
	if _, ok := currentHierarchyAfterGoalIDs[deletedTeamID]; !ok {
		t.Fatalf("expected deleted team with current goals in hierarchy, got %+v", currentHierarchyAfterGoalIDs)
	}
	deletedWithGoalNode := findHierarchyNodeByID(currentHierarchyAfterGoal.Items, deletedTeamID)
	if deletedWithGoalNode == nil {
		t.Fatalf("expected deleted team node in current hierarchy after goal")
	}
	if !deletedWithGoalNode.HasGoals {
		t.Fatalf("expected deleted team with current goals to have has_goals=true")
	}
	if deletedWithGoalNode.Progress == nil {
		t.Fatalf("expected deleted team with current goals to have progress")
	}

	currentDeletedVisibleResp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", server.URL, deletedTeamID, currentPeriodID))
	if err != nil {
		t.Fatalf("get current deleted team okr after goal: %v", err)
	}
	defer currentDeletedVisibleResp.Body.Close()
	if currentDeletedVisibleResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for deleted team with current goal, got %d", currentDeletedVisibleResp.StatusCode)
	}
}

func TestTeamOverviewIncludesChildrenSummaryIntegration(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.RunContainer(ctx,
		tcpostgres.WithDatabase("okrs"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second),
		),
	)
	testutil.RequireDockerOrSkip(t, err)
	defer func() { _ = container.Terminate(ctx) }()

	dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}
	if err := testutil.RunMigrations(dbURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	repo := store.New(pool)
	var parentID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Parent') RETURNING id`).Scan(&parentID); err != nil {
		t.Fatalf("insert parent team: %v", err)
	}
	var childID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, parent_id) VALUES ('Child', $1) RETURNING id`, parentID).Scan(&childID); err != nil {
		t.Fatalf("insert child team: %v", err)
	}
	var periodID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date, sort_order)
		VALUES ('2024 Q3', '2024-07-01', '2024-09-30', 1)
		RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}
	goalID, err := repo.CreateGoal(ctx, store.GoalInput{
		TeamID:      childID,
		PeriodID:    periodID,
		Title:       "Child goal",
		Description: "desc",
		Priority:    domain.PriorityP1,
		Weight:      100,
		WorkType:    domain.WorkTypeDelivery,
		FocusType:   domain.FocusStability,
		OwnerText:   "Owner",
	})
	if err != nil {
		t.Fatalf("create child goal: %v", err)
	}
	krID, err := repo.CreateKeyResult(ctx, store.KeyResultInput{
		GoalID:      goalID,
		Title:       "KR bool",
		Description: "",
		Weight:      100,
		Kind:        domain.KRKindBoolean,
	})
	if err != nil {
		t.Fatalf("create child key result: %v", err)
	}
	if err := repo.UpdateBoolean(ctx, krID, true); err != nil {
		t.Fatalf("update child key result progress: %v", err)
	}
	if err := repo.SetTeamPeriodStatus(ctx, childID, periodID, domain.TeamPeriodStatusInProgress); err != nil {
		t.Fatalf("set status: %v", err)
	}

	svc := service.New(repo)
	server := httptest.NewServer(testutil.NewAPIV1Router(svc))
	defer server.Close()

	resp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/overview?period_id=%d", server.URL, parentID, periodID))
	if err != nil {
		t.Fatalf("get team overview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var payload struct {
		ChildrenSummary struct {
			Items []struct {
				Team struct {
					ID int64 `json:"id"`
				} `json:"team"`
				HasGoals     bool        `json:"has_goals"`
				ProgressMeta interface{} `json:"progress_meta"`
				Status       string      `json:"status"`
				LastUpdated  *time.Time  `json:"last_updated"`
			} `json:"items"`
		} `json:"children_summary"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if len(payload.ChildrenSummary.Items) != 1 {
		t.Fatalf("expected one child summary row, got %d", len(payload.ChildrenSummary.Items))
	}
	item := payload.ChildrenSummary.Items[0]
	if item.Team.ID != childID {
		t.Fatalf("expected child team id %d, got %d", childID, item.Team.ID)
	}
	if !item.HasGoals {
		t.Fatalf("expected has_goals=true for child team with goal")
	}
	if item.ProgressMeta == nil {
		t.Fatalf("expected progress_meta for child team with goals")
	}
	if item.Status != string(domain.TeamPeriodStatusInProgress) {
		t.Fatalf("expected status in_progress, got %s", item.Status)
	}
	if item.LastUpdated == nil {
		t.Fatalf("expected last_updated to be present")
	}
}

type hierarchyResponse struct {
	Items []teamNode `json:"items"`
}

type teamNode struct {
	ID       int64      `json:"id"`
	HasGoals bool       `json:"has_goals"`
	Progress *int       `json:"progress,omitempty"`
	Children []teamNode `json:"children"`
}

func flattenHierarchyIDs(nodes []teamNode) map[int64]struct{} {
	ids := make(map[int64]struct{})
	var walk func(items []teamNode)
	walk = func(items []teamNode) {
		for _, node := range items {
			ids[node.ID] = struct{}{}
			if len(node.Children) > 0 {
				walk(node.Children)
			}
		}
	}
	walk(nodes)
	return ids
}

func findHierarchyNodeByID(nodes []teamNode, targetID int64) *teamNode {
	for _, node := range nodes {
		if node.ID == targetID {
			copyNode := node
			return &copyNode
		}
		if len(node.Children) > 0 {
			if child := findHierarchyNodeByID(node.Children, targetID); child != nil {
				return child
			}
		}
	}
	return nil
}
