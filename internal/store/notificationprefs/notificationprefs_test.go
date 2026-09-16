package notificationprefs_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/notificationprefs"
	"okrs/internal/store/testutil"

	"github.com/jackc/pgx/v5/pgxpool"
)

// tree строит дерево команд корень → середина → лист, у каждой свой лид,
// и делает всех лидов активными участниками тенанта 1.
// Возвращает id команд и id пользователей-лидов.
func tree(t *testing.T, pool *pgxpool.Pool) (teamIDs [3]int64, leadIDs [3]int64) {
	t.Helper()
	ctx := context.Background()
	names := []string{"Корень", "Середина", "Лист"}
	var parent *int64
	for i, name := range names {
		var udid string
		err := pool.QueryRow(ctx, `
			INSERT INTO users (provider_subject_key, provider, subject, display_name)
			VALUES ($1,'system',$1,$2) RETURNING id, udid`,
			"lead-"+name, "Лид "+name).Scan(&leadIDs[i], &udid)
		if err != nil {
			t.Fatalf("создать лида %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1,1,'user','active')`,
			leadIDs[i]); err != nil {
			t.Fatalf("членство %s: %v", name, err)
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO teams (name, team_type, parent_id, tenant_id, lead_udid)
			VALUES ($1,'team',$2,1,$3) RETURNING id`,
			name, parent, udid).Scan(&teamIDs[i]); err != nil {
			t.Fatalf("создать команду %s: %v", name, err)
		}
		p := teamIDs[i]
		parent = &p
	}
	return teamIDs, leadIDs
}

func has(rs []notificationprefs.Recipient, userID int64) bool {
	for _, r := range rs {
		if r.UserID == userID {
			return true
		}
	}
	return false
}

// Дефолт (строки настроек нет) — scope=own: событие в листе уведомляет только
// лида листа, ни середину, ни корень.
func TestResolveDefaultScopeIsOwn(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}

	rs, err := repo.ResolveRecipients(context.Background(), scope, "goal_changed",
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 1}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !has(rs, leads[2]) {
		t.Error("лид собственной команды обязан получить уведомление")
	}
	if has(rs, leads[1]) || has(rs, leads[0]) {
		t.Error("при scope=own предки не уведомляются")
	}
}

// own_and_children: лид середины получает событие из листа (дистанция 1),
// лид корня — нет (дистанция 2).
func TestResolveOwnAndChildren(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	for _, id := range []int64{leads[0], leads[1]} {
		if err := repo.Set(ctx, scope, id, notificationprefs.Preference{
			Type: "goal_changed", Enabled: true, Scope: "own_and_children", ChannelOverrides: map[string]bool{"in_app": true},
		}); err != nil {
			t.Fatalf("set: %v", err)
		}
	}

	rs, err := repo.ResolveRecipients(ctx, scope, "goal_changed",
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 1}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !has(rs, leads[1]) {
		t.Error("лид на дистанции 1 обязан получить уведомление")
	}
	if has(rs, leads[0]) {
		t.Error("лид на дистанции 2 не должен получать при own_and_children")
	}
}

// subtree: событие из листа доходит до корня на любой глубине.
func TestResolveSubtree(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	if err := repo.Set(ctx, scope, leads[0], notificationprefs.Preference{
		Type: "goal_changed", Enabled: true, Scope: "subtree", ChannelOverrides: map[string]bool{"in_app": true},
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	rs, err := repo.ResolveRecipients(ctx, scope, "goal_changed",
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 1}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !has(rs, leads[0]) {
		t.Error("при subtree корень обязан получить событие из листа")
	}
}

// Актор не уведомляется о собственном действии — и исключается ПОШТУЧНО:
// лид, оказавшийся автором одного события в батче, должен получить остальные.
func TestActorExcludedPerEventNotPerBatch(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}

	// Два события: в первом актор — лид листа, во втором — посторонний (id 1).
	rs, err := repo.ResolveRecipients(context.Background(), scope, "goal_changed",
		[]notificationprefs.Target{
			{TeamID: teams[2], ActorID: leads[2]},
			{TeamID: teams[2], ActorID: 1},
		})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	for _, r := range rs {
		if r.Ord == 0 && r.UserID == leads[2] {
			t.Error("актор получил уведомление о собственном действии")
		}
	}
	found := false
	for _, r := range rs {
		if r.Ord == 1 && r.UserID == leads[2] {
			found = true
		}
	}
	if !found {
		t.Error("лид не получил уведомление о ЧУЖОМ действии в том же батче")
	}
}

// Выключенный тип не приносит уведомлений вообще.
func TestDisabledTypeYieldsNoRecipients(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	if err := repo.Set(ctx, scope, leads[2], notificationprefs.Preference{
		Type: "goal_changed", Enabled: false, Scope: "own", ChannelOverrides: map[string]bool{"in_app": true},
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	rs, err := repo.ResolveRecipients(ctx, scope, "goal_changed",
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 1}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if has(rs, leads[2]) {
		t.Error("выключенный тип не должен давать получателей")
	}
}

// Лид, исключённый из пространства, уведомления не получает, хотя
// teams.lead_udid у команды остался заполненным.
func TestInactiveMemberExcluded(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `DELETE FROM memberships WHERE user_id = $1 AND tenant_id = 1`, leads[2]); err != nil {
		t.Fatalf("удалить членство: %v", err)
	}
	rs, err := repo.ResolveRecipients(ctx, scope, "goal_changed",
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 1}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if has(rs, leads[2]) {
		t.Error("бывший участник пространства не должен получать уведомления")
	}
}

// Soft-deleted команда в середине цепочки обрывает подъём: предки за ней
// не считаются частью поддерева.
func TestSoftDeletedTeamBreaksChain(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	if err := repo.Set(ctx, scope, leads[0], notificationprefs.Preference{
		Type: "goal_changed", Enabled: true, Scope: "subtree", ChannelOverrides: map[string]bool{"in_app": true},
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE teams SET deleted_at = now() WHERE id = $1`, teams[1]); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	rs, err := repo.ResolveRecipients(ctx, scope, "goal_changed",
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 1}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if has(rs, leads[0]) {
		t.Error("удалённая команда в цепочке должна обрывать подъём к предкам")
	}
}

// GetAll подставляет дефолты для типов, у которых строки нет: все четыре типа
// должны вернуться всегда, иначе экран настроек покажет пустоту новому пользователю.
func TestGetAllReturnsDefaultsForMissingRows(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	scope := domain.TenantScope{TenantID: 1}

	prefs, err := repo.GetAll(context.Background(), scope, 1)
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if len(prefs) != 4 {
		t.Fatalf("ожидались все 4 типа, got %d", len(prefs))
	}
	for _, p := range prefs {
		if !p.Enabled {
			t.Errorf("%s: дефолт должен быть включён", p.Type)
		}
		if p.Type == "my_comment_resolved" {
			if p.Scope != "" {
				t.Errorf("у адресного типа скоуп неприменим, got %q", p.Scope)
			}
			continue
		}
		if p.Scope != "own" {
			t.Errorf("%s: дефолтный скоуп own, got %q", p.Type, p.Scope)
		}
	}
}

// A stored row must be returned as stored, not silently replaced by the default.
// TestGetAllReturnsDefaultsForMissingRows only exercises the missing-row path; this
// pins the "use the stored row" branch of GetAll.
func TestGetAllReturnsStoredRowOverridesDefault(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	if err := repo.Set(ctx, scope, 1, notificationprefs.Preference{
		Type: "goal_changed", Enabled: false, Scope: "subtree", ChannelOverrides: map[string]bool{"in_app": true},
	}); err != nil {
		t.Fatalf("set: %v", err)
	}

	prefs, err := repo.GetAll(ctx, scope, 1)
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	var got *notificationprefs.Preference
	for i := range prefs {
		if prefs[i].Type == "goal_changed" {
			got = &prefs[i]
		}
	}
	if got == nil {
		t.Fatalf("goal_changed missing from result")
	}
	if got.Enabled {
		t.Error("stored row is disabled, GetAll must not report it enabled")
	}
	if got.Scope != "subtree" {
		t.Errorf("stored scope is subtree, got %q", got.Scope)
	}
}

// A member with status "requested" (not yet admitted) must be excluded, not just an
// absent membership row. Deleting the row (as TestInactiveMemberExcluded does) only
// exercises the join's existence, not the status predicate.
func TestRequestedMemberExcluded(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE memberships SET status = 'requested' WHERE user_id = $1 AND tenant_id = 1`,
		leads[2]); err != nil {
		t.Fatalf("demote membership: %v", err)
	}
	rs, err := repo.ResolveRecipients(ctx, scope, "goal_changed",
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 1}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if has(rs, leads[2]) {
		t.Error("a requested (not active) member must not receive notifications")
	}
}

// An event in a team that is already soft-deleted must resolve no recipients: the
// seed term's deleted_at guard, not just the recursive term's, must hold.
func TestResolveEventTeamAlreadyDeletedYieldsNoRecipients(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE teams SET deleted_at = now() WHERE id = $1`, teams[2]); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	rs, err := repo.ResolveRecipients(ctx, scope, "goal_changed",
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 1}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if has(rs, leads[2]) {
		t.Error("event's own team is soft-deleted, seed term must exclude it")
	}
	if len(rs) != 0 {
		t.Errorf("expected no recipients, got %d", len(rs))
	}
}

// One person leading both a team and its parent unit must be resolved exactly once
// for a single event, not once per ancestor path. Without SELECT DISTINCT this lead
// would appear twice with the identical (Ord, UserID), and the fan-out's ON CONFLICT
// would bump coalesce_count instead of discarding the duplicate.
func TestDuplicateAncestorLeadReturnedOnce(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	// Make the middle team's lead the same person as the root's lead, so leads[0]
	// is reached via two different ancestor paths of the same leaf event.
	if _, err := pool.Exec(ctx, `
		UPDATE teams SET lead_udid = (SELECT udid FROM users WHERE id = $1)
		 WHERE id = $2`, leads[0], teams[1]); err != nil {
		t.Fatalf("reassign middle lead: %v", err)
	}
	if err := repo.Set(ctx, scope, leads[0], notificationprefs.Preference{
		Type: "goal_changed", Enabled: true, Scope: "subtree", ChannelOverrides: map[string]bool{"in_app": true},
	}); err != nil {
		t.Fatalf("set: %v", err)
	}

	rs, err := repo.ResolveRecipients(ctx, scope, "goal_changed",
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 1}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	count := 0
	for _, r := range rs {
		if r.UserID == leads[0] {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one row for the dual-role lead, got %d", count)
	}
}

// ResolveAddressed has no test coverage yet: pin the ord-1 mapping across a
// multi-element batch and the enabled-preference filter.
func TestResolveAddressedOrdMappingAndDisabledFilter(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	_, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	if err := repo.Set(ctx, scope, leads[1], notificationprefs.Preference{
		Type: "my_comment_resolved", Enabled: false, ChannelOverrides: map[string]bool{"in_app": true},
	}); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Three-element batch: leads[0] (default enabled), leads[1] (disabled),
	// leads[2] (default enabled) at ordinals 0, 1, 2.
	rs, err := repo.ResolveAddressed(ctx, scope, "my_comment_resolved",
		[]int64{leads[0], leads[1], leads[2]})
	if err != nil {
		t.Fatalf("resolve addressed: %v", err)
	}

	byOrd := make(map[int]int64)
	for _, r := range rs {
		byOrd[r.Ord] = r.UserID
	}
	if byOrd[0] != leads[0] {
		t.Errorf("ord 0: want %d, got %d", leads[0], byOrd[0])
	}
	if _, ok := byOrd[1]; ok {
		t.Error("leads[1] has the type disabled and must be filtered out")
	}
	if byOrd[2] != leads[2] {
		t.Errorf("ord 2: want %d, got %d", leads[2], byOrd[2])
	}
	if len(rs) != 2 {
		t.Errorf("expected 2 recipients (leads[1] filtered), got %d", len(rs))
	}
}

// Выбор по каналу, которого не было на экране, обязан пережить сохранение.
//
// Администратор временно выключает канал — его колонка исчезает из матрицы, и
// PUT про этот канал ничего не несёт. Полная замена карты стёрла бы ответ
// пользователя, и повторное включение канала вернуло бы ему умолчание
// администратора — то есть доставку, от которой он явно отказался.
func TestSetMergesAndKeepsChoicesTheFormCouldNotShow(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	// Пользователь однажды отказался от mattermost и включил telegram.
	if err := repo.Set(ctx, scope, 1, notificationprefs.Preference{
		Type: "goal_changed", Enabled: true, Scope: "own",
		ChannelOverrides: map[string]bool{"in_app": true, "mattermost": false, "telegram": true},
	}); err != nil {
		t.Fatalf("первое сохранение: %v", err)
	}

	// Администратор выключил mattermost — его колонки в матрице больше нет,
	// и следующее сохранение про него молчит.
	if err := repo.Set(ctx, scope, 1, notificationprefs.Preference{
		Type: "goal_changed", Enabled: true, Scope: "subtree",
		ChannelOverrides: map[string]bool{"in_app": false, "telegram": true},
	}); err != nil {
		t.Fatalf("второе сохранение: %v", err)
	}

	all, err := repo.GetAll(ctx, scope, 1)
	if err != nil {
		t.Fatalf("getAll: %v", err)
	}
	var got map[string]bool
	var gotScope string
	for _, p := range all {
		if p.Type == "goal_changed" {
			got, gotScope = p.ChannelOverrides, p.Scope
		}
	}
	if on, ok := got["mattermost"]; !ok || on {
		t.Fatalf("выбор по скрытому каналу потерян: %v", got)
	}
	// Видимые каналы перезаписаны присланным: слияние не удерживает устаревший ответ.
	if got["in_app"] {
		t.Fatalf("присланное значение видимого канала не победило: %v", got)
	}
	if !got["telegram"] {
		t.Fatalf("telegram потерян: %v", got)
	}
	// Остальные поля строки перезаписываются как прежде.
	if gotScope != "subtree" {
		t.Fatalf("scope не обновился: %q", gotScope)
	}
}

// Повторное сохранение той же матрицы не меняет итогового состояния.
func TestSetIsIdempotent(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	repo := notificationprefs.NewRepository(pool)
	scope := domain.TenantScope{TenantID: 1}
	ctx := context.Background()

	p := notificationprefs.Preference{
		Type: "kr_progress", Enabled: true, Scope: "own",
		ChannelOverrides: map[string]bool{"in_app": true, "mattermost": false},
	}
	for i := 0; i < 2; i++ {
		if err := repo.Set(ctx, scope, 1, p); err != nil {
			t.Fatalf("сохранение %d: %v", i, err)
		}
	}
	all, _ := repo.GetAll(ctx, scope, 1)
	for _, got := range all {
		if got.Type != "kr_progress" {
			continue
		}
		if len(got.ChannelOverrides) != 2 || got.ChannelOverrides["in_app"] != true ||
			got.ChannelOverrides["mattermost"] != false {
			t.Fatalf("повторное сохранение изменило состояние: %v", got.ChannelOverrides)
		}
	}
}
