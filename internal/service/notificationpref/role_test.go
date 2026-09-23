package notificationpref_test

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/core/domain"
	notificationprefsvc "okrs/internal/service/notificationpref"
	"okrs/internal/store/notificationprefs"
)

// catalogRepo отдаёт весь каталог, как настоящий стор: сохранённые строки как
// есть, для остальных — значения по умолчанию из каталога.
type catalogRepo struct {
	fakeRepo
	stored map[string]notificationprefs.Preference
}

func (c *catalogRepo) GetAll(context.Context, domain.TenantScope, int64) ([]notificationprefs.Preference, error) {
	out := make([]notificationprefs.Preference, 0, len(notificationprefs.AllTypes))
	for _, typ := range notificationprefs.AllTypes {
		if p, ok := c.stored[typ]; ok {
			out = append(out, p)
			continue
		}
		out = append(out, notificationprefs.Preference{Type: typ, Enabled: notificationprefs.DefaultEnabled(typ)})
	}
	return out, nil
}

func types(ps []notificationprefs.Preference) map[string]notificationprefs.Preference {
	out := make(map[string]notificationprefs.Preference, len(ps))
	for _, p := range ps {
		out[p.Type] = p
	}
	return out
}

// Участнику — четыре типа «Целей», администратору — пять, включая заявку на
// доступ, выключенную по умолчанию.
func TestGetAllDependsOnRole(t *testing.T) {
	svc := notificationprefsvc.New(&catalogRepo{}, nil)
	scope := domain.TenantScope{TenantID: 1}

	member, err := svc.GetAll(context.Background(), scope, 42, false)
	if err != nil {
		t.Fatalf("member: %v", err)
	}
	if len(member) != 4 {
		t.Fatalf("участнику %d типов, want 4", len(member))
	}
	if _, ok := types(member)[notificationprefs.TypeAccessRequested]; ok {
		t.Fatal("заявка на доступ не показывается участнику")
	}
	for _, p := range member {
		if !p.Enabled {
			t.Errorf("%s: типы «Целей» по умолчанию включены", p.Type)
		}
	}

	admin, err := svc.GetAll(context.Background(), scope, 42, true)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	if len(admin) != 5 {
		t.Fatalf("администратору %d типов, want 5", len(admin))
	}
	if p, ok := types(admin)[notificationprefs.TypeAccessRequested]; !ok || p.Enabled {
		t.Fatalf("заявка на доступ показана администратору выключенной: %+v", p)
	}
}

// Сохранить настройку заявки может только администратор; отказ не записывает
// ни одной строки матрицы.
func TestSetAllRejectsAdminOnlyTypeForMember(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo, nil)
	matrix := []notificationprefs.Preference{
		{Type: notificationprefs.TypeGoalComment, Enabled: true, Scope: notificationprefs.ScopeOwn},
		{Type: notificationprefs.TypeAccessRequested, Enabled: true},
	}

	err := svc.SetAll(context.Background(), domain.TenantScope{TenantID: 1}, 42, false, matrix)
	if !errors.Is(err, notificationprefsvc.ErrInvalidType) {
		t.Fatalf("err = %v, want ErrInvalidType", err)
	}
	if len(repo.saved) != 0 {
		t.Fatalf("при отказе ничего не пишется, записано: %+v", repo.saved)
	}

	if err := svc.SetAll(context.Background(), domain.TenantScope{TenantID: 1}, 42, true, matrix); err != nil {
		t.Fatalf("администратор: %v", err)
	}
	if len(repo.saved) != 2 {
		t.Fatalf("у администратора записана вся матрица, got %d", len(repo.saved))
	}
	if repo.saved[1].Scope != "" {
		t.Errorf("у заявки на доступ охвата нет, got %q", repo.saved[1].Scope)
	}
}

// Настройка бывшего администратора не стирается: вернув роль, он видит её
// такой, какой оставил.
func TestAdminChoiceSurvivesRoleLoss(t *testing.T) {
	repo := &catalogRepo{stored: map[string]notificationprefs.Preference{
		notificationprefs.TypeAccessRequested: {Type: notificationprefs.TypeAccessRequested, Enabled: true},
	}}
	svc := notificationprefsvc.New(repo, nil)
	scope := domain.TenantScope{TenantID: 1}

	if ps, _ := svc.GetAll(context.Background(), scope, 42, false); len(ps) != 4 {
		t.Fatalf("без роли настройка не показывается, got %d типов", len(ps))
	}
	ps, _ := svc.GetAll(context.Background(), scope, 42, true)
	if p := types(ps)[notificationprefs.TypeAccessRequested]; !p.Enabled {
		t.Fatalf("вернув роль, администратор видит свой прежний выбор: %+v", p)
	}
}
