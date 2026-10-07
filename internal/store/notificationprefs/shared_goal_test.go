package notificationprefs_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/notificationprefs"
	"okrs/internal/store/testutil"
)

func find(t *testing.T, rs []notificationprefs.Recipient, userID int64) notificationprefs.Recipient {
	t.Helper()
	for _, r := range rs {
		if r.UserID == userID {
			return r
		}
	}
	t.Fatalf("получатель %d не найден среди %+v", userID, rs)
	return notificationprefs.Recipient{}
}

// Общая цель приходит сюда несколькими target'ами с одним и тем же Ord: резолвер
// обязан обойти дерево от каждой команды и вернуть строку на каждую.
func TestSeveralTargetsOfOneEventEachResolveSeparately(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	repo := notificationprefs.NewRepository(pool)

	// Два target'а одного события: корень и лист.
	rs, err := repo.ResolveRecipients(context.Background(), scope, notificationprefs.TypeGoalChanged,
		[]notificationprefs.Target{{TeamID: teams[0], ActorID: 0}, {TeamID: teams[2], ActorID: 0}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !has(rs, leads[0]) || !has(rs, leads[2]) {
		t.Fatalf("каждая команда обязана дать своего лида, got %+v", rs)
	}
	if got := find(t, rs, leads[0]); got.Ord != 0 {
		t.Errorf("лид корня найден через первый target, Ord = %d", got.Ord)
	}
	if got := find(t, rs, leads[2]); got.Ord != 1 {
		t.Errorf("лид листа найден через второй target, Ord = %d", got.Ord)
	}
}

// Получатель обязан знать, ОТ КАКОЙ команды его нашли: именно она попадает в
// запись уведомления и решает, на чью доску ведёт переход.
func TestRecipientCarriesTheTeamTheWalkStartedFrom(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	teams, leads := tree(t, pool)
	scope := domain.TenantScope{TenantID: 1}
	repo := notificationprefs.NewRepository(pool)

	// Лид корня со скоупом «всё поддерево» ловит событие, случившееся в листе.
	if err := repo.Set(context.Background(), scope, leads[0], notificationprefs.Preference{
		Type: notificationprefs.TypeGoalChanged, Enabled: true, Scope: notificationprefs.ScopeSubtree,
	}); err != nil {
		t.Fatalf("настройка: %v", err)
	}
	rs, err := repo.ResolveRecipients(context.Background(), scope, notificationprefs.TypeGoalChanged,
		[]notificationprefs.Target{{TeamID: teams[2], ActorID: 0}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	root := find(t, rs, leads[0])
	if root.TeamID != teams[2] {
		t.Errorf("TeamID = %d, хотя обход начался от листа %d", root.TeamID, teams[2])
	}
	if root.Distance != 2 {
		t.Errorf("Distance = %d, лид корня на два шага выше листа", root.Distance)
	}
	leaf := find(t, rs, leads[2])
	if leaf.Distance != 0 {
		t.Errorf("лид самой команды события: Distance = %d, want 0", leaf.Distance)
	}
}

// Человек, руководящий и командой, и её предком, по одному target'у возвращается
// один раз — и с БЛИЖАЙШЕЙ дистанцией. Две строки downstream схлопнулись бы в одну
// запись с инкрементом coalesce_count и показали бы «×2» на одно изменение.
func TestDuplicateAncestorLeadReportsTheNearestDistance(t *testing.T) {
	pool, cleanup := testutil.SetupDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}
	repo := notificationprefs.NewRepository(pool)

	var leadID int64
	var udid string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (provider_subject_key, provider, subject, display_name)
		VALUES ('lead-both','system','lead-both','Лид обеих') RETURNING id, udid`).Scan(&leadID, &udid); err != nil {
		t.Fatalf("создать лида: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1,1,'user','active')`, leadID); err != nil {
		t.Fatalf("членство: %v", err)
	}
	var parentID, childID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO teams (name, team_type, tenant_id, lead_udid) VALUES ('Родитель','team',1,$1) RETURNING id`,
		udid).Scan(&parentID); err != nil {
		t.Fatalf("родитель: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO teams (name, team_type, parent_id, tenant_id, lead_udid) VALUES ('Потомок','team',$1,1,$2) RETURNING id`,
		parentID, udid).Scan(&childID); err != nil {
		t.Fatalf("потомок: %v", err)
	}
	if err := repo.Set(ctx, scope, leadID, notificationprefs.Preference{
		Type: notificationprefs.TypeGoalChanged, Enabled: true, Scope: notificationprefs.ScopeSubtree,
	}); err != nil {
		t.Fatalf("настройка: %v", err)
	}

	rs, err := repo.ResolveRecipients(ctx, scope, notificationprefs.TypeGoalChanged,
		[]notificationprefs.Target{{TeamID: childID, ActorID: 0}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(rs) != 1 {
		t.Fatalf("лид, найденный двумя путями одного target'а, обязан вернуться один раз, got %+v", rs)
	}
	if rs[0].Distance != 0 {
		t.Errorf("Distance = %d, ожидался ближайший путь (сама команда события)", rs[0].Distance)
	}
}
