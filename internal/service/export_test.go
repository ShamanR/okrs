package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/export"
	apitestutil "okrs/internal/http/handlers/api/v1/testutil"
	"okrs/internal/service"
	"okrs/internal/store"
	"okrs/internal/store/goals"
	"okrs/internal/store/grants"
	"okrs/internal/store/shares"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestExportOKRTree(t *testing.T) {
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
	apitestutil.RequireDockerOrSkip(t, err)
	defer func() { _ = container.Terminate(ctx) }()

	dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}
	if err := apitestutil.RunMigrations(dbURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	repo := store.New(pool)
	scope := domain.TenantScope{TenantID: 1}

	var rootID, childID, periodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type) VALUES ('Платформа', 'unit') RETURNING id`).Scan(&rootID); err != nil {
		t.Fatalf("insert root team: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type, parent_id) VALUES ('Web', 'team', $1) RETURNING id`, rootID).Scan(&childID); err != nil {
		t.Fatalf("insert child team: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO periods (name, start_date, end_date)
		VALUES ('Current', CURRENT_DATE - INTERVAL '5 day', CURRENT_DATE + INTERVAL '5 day')
		RETURNING id`).Scan(&periodID); err != nil {
		t.Fatalf("insert period: %v", err)
	}

	goalID, err := repo.Goals.CreateGoal(ctx, scope, goals.GoalInput{
		TeamID: rootID, PeriodID: periodID, Title: "Общая цель",
		Priority: domain.PriorityP1, Weight: 100,
		WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability, OwnerText: "Owner",
	})
	if err != nil {
		t.Fatalf("create goal: %v", err)
	}
	// share the root-owned goal into the child team
	if err := repo.Shares.ReplaceGoalShares(ctx, scope, goalID, []shares.GoalShareInput{{TeamID: childID, Weight: 100}}); err != nil {
		t.Fatalf("share goal: %v", err)
	}

	// A goal owned by a team OUTSIDE root's subtree, shared into the child. When exporting root's
	// tree, no owner block exists for it, so it must render fully (not a bare reference).
	var otherID, outsideGoalID int64
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name, team_type) VALUES ('Секрет', 'team') RETURNING id`).Scan(&otherID); err != nil {
		t.Fatalf("insert other team: %v", err)
	}
	outsideGoalID, err = repo.Goals.CreateGoal(ctx, scope, goals.GoalInput{
		TeamID: otherID, PeriodID: periodID, Title: "Внешняя цель", Description: "детали внешней цели",
		Priority: domain.PriorityP2, Weight: 100, WorkType: domain.WorkTypeDelivery, FocusType: domain.FocusStability,
	})
	if err != nil {
		t.Fatalf("create outside goal: %v", err)
	}
	if err := repo.Shares.ReplaceGoalShares(ctx, scope, outsideGoalID, []shares.GoalShareInput{{TeamID: childID, Weight: 100}}); err != nil {
		t.Fatalf("share outside goal: %v", err)
	}

	svc := service.NewFromStore(repo, grants.NewGrantsCache(repo.Grants), nil, nil)

	// full subtree access: owner block full, child block shows shared reference
	res, err := svc.ExportOKR(ctx, scope, service.ExportParams{
		TeamID: rootID, PeriodID: periodID, Scope: export.ScopeTree,
		Options: export.Options{Format: export.FormatShort}, AllowedTeamIDs: []int64{rootID, childID},
	})
	if err != nil {
		t.Fatalf("ExportOKR tree: %v", err)
	}
	if !strings.Contains(res.Markdown, "## Общая цель\n") {
		t.Fatalf("owner block should render goal fully:\n%s", res.Markdown)
	}
	if !strings.Contains(res.Markdown, "_(общая, владелец: Платформа)_") {
		t.Fatalf("child block should show shared reference:\n%s", res.Markdown)
	}
	if res.Filename != "okr-"+expectedBase(t)+"-u"+itoa(rootID)+"-tree.md" {
		t.Fatalf("unexpected filename: %s", res.Filename)
	}
	if res.Lines == 0 {
		t.Fatalf("expected non-zero line count")
	}

	// restricted scope: child excluded from output
	res2, err := svc.ExportOKR(ctx, scope, service.ExportParams{
		TeamID: rootID, PeriodID: periodID, Scope: export.ScopeTree,
		Options: export.Options{Format: export.FormatShort}, AllowedTeamIDs: []int64{rootID},
	})
	if err != nil {
		t.Fatalf("ExportOKR restricted: %v", err)
	}
	if strings.Contains(res2.Markdown, "# Платформа / Web") {
		t.Fatalf("child block should be excluded when out of scope:\n%s", res2.Markdown)
	}

	// goal scope from the team the goal is SHARED INTO must render fully, not as a reference.
	res3, err := svc.ExportOKR(ctx, scope, service.ExportParams{
		TeamID: childID, PeriodID: periodID, GoalID: goalID, Scope: export.ScopeGoal,
		Options: export.Options{Format: export.FormatShort}, AllowedTeamIDs: []int64{rootID, childID},
	})
	if err != nil {
		t.Fatalf("ExportOKR goal-from-shared: %v", err)
	}
	if strings.Contains(res3.Markdown, "_(общая") {
		t.Fatalf("shared goal in goal scope must render fully, not as reference:\n%s", res3.Markdown)
	}
	if !strings.Contains(res3.Markdown, "## Общая цель\n") {
		t.Fatalf("expected full goal heading in goal scope:\n%s", res3.Markdown)
	}

	// tree scope rooted at the child, with access ONLY to the child (not the root):
	// the heading must not leak the inaccessible parent's name.
	res5, err := svc.ExportOKR(ctx, scope, service.ExportParams{
		TeamID: childID, PeriodID: periodID, Scope: export.ScopeTree,
		Options: export.Options{Format: export.FormatShort}, AllowedTeamIDs: []int64{childID},
	})
	if err != nil {
		t.Fatalf("ExportOKR child-only tree: %v", err)
	}
	if strings.Contains(res5.Markdown, "Платформа") {
		t.Fatalf("inaccessible ancestor name leaked into headings:\n%s", res5.Markdown)
	}
	if !strings.Contains(res5.Markdown, "# Web\n") {
		t.Fatalf("expected child heading without ancestor path:\n%s", res5.Markdown)
	}

	// export root's tree (access to root+child, NOT the outside owner): the outside-owned goal
	// shared into the child must render fully, not collapse to a reference that would drop its body.
	res6, err := svc.ExportOKR(ctx, scope, service.ExportParams{
		TeamID: rootID, PeriodID: periodID, Scope: export.ScopeTree,
		Options: export.Options{Format: export.FormatShort}, AllowedTeamIDs: []int64{rootID, childID},
	})
	if err != nil {
		t.Fatalf("ExportOKR outside-owner: %v", err)
	}
	if strings.Contains(res6.Markdown, "## Внешняя цель _(общая") {
		t.Fatalf("outside-owner goal wrongly collapsed to a reference:\n%s", res6.Markdown)
	}
	if !strings.Contains(res6.Markdown, "## Внешняя цель\n") || !strings.Contains(res6.Markdown, "детали внешней цели") {
		t.Fatalf("outside-owner goal must render fully with its body:\n%s", res6.Markdown)
	}

	// team scope from the shared-into team must also render the shared goal fully.
	res4, err := svc.ExportOKR(ctx, scope, service.ExportParams{
		TeamID: childID, PeriodID: periodID, Scope: export.ScopeTeam,
		Options: export.Options{Format: export.FormatShort}, AllowedTeamIDs: []int64{rootID, childID},
	})
	if err != nil {
		t.Fatalf("ExportOKR team-from-shared: %v", err)
	}
	if strings.Contains(res4.Markdown, "_(общая") || !strings.Contains(res4.Markdown, "## Общая цель\n") {
		t.Fatalf("shared goal in team scope must render fully:\n%s", res4.Markdown)
	}
}

func expectedBase(t *testing.T) string {
	t.Helper()
	// period start_date is CURRENT_DATE - 5 days; the filename quarter is derived from it.
	start := time.Now().UTC().AddDate(0, 0, -5)
	q := (int(start.Month())-1)/3 + 1
	yy := start.Year() % 100
	return "y" + pad(yy) + "q" + itoa(int64(q))
}

func pad(n int) string {
	s := itoa(int64(n))
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	s := string(buf[i:])
	if neg {
		s = "-" + s
	}
	return s
}
