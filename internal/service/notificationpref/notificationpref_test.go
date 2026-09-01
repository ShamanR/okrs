package notificationpref_test

import (
	"context"
	"errors"
	"testing"

	"okrs/internal/core/domain"
	notificationprefsvc "okrs/internal/service/notificationpref"
	"okrs/internal/store/notificationprefs"
)

// fakeRepo фиксирует, что дошло до стора: валидация обязана отсекать мусор
// ДО записи, а не полагаться на CHECK-ограничение в БД.
type fakeRepo struct {
	saved []notificationprefs.Preference
}

func (f *fakeRepo) GetAll(context.Context, domain.TenantScope, int64) ([]notificationprefs.Preference, error) {
	return nil, nil
}

func (f *fakeRepo) Set(_ context.Context, _ domain.TenantScope, _ int64, p notificationprefs.Preference) error {
	f.saved = append(f.saved, p)
	return nil
}

func (f *fakeRepo) ResolveRecipients(context.Context, domain.TenantScope, string, []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	return nil, nil
}

func (f *fakeRepo) ResolveAddressed(context.Context, domain.TenantScope, string, []int64) ([]notificationprefs.Recipient, error) {
	return nil, nil
}

func TestSetRejectsUnknownType(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo)
	err := svc.Set(context.Background(), domain.TenantScope{TenantID: 1}, 1,
		notificationprefs.Preference{Type: "made_up", Enabled: true, Scope: "own"})
	if !errors.Is(err, notificationprefsvc.ErrInvalidType) {
		t.Fatalf("got %v, want ErrInvalidType", err)
	}
	if len(repo.saved) != 0 {
		t.Error("невалидный тип не должен доходить до стора")
	}
}

func TestSetRejectsUnknownScope(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo)
	err := svc.Set(context.Background(), domain.TenantScope{TenantID: 1}, 1,
		notificationprefs.Preference{Type: notificationprefs.TypeGoalChanged, Enabled: true, Scope: "everything"})
	if !errors.Is(err, notificationprefsvc.ErrInvalidScope) {
		t.Fatalf("got %v, want ErrInvalidScope", err)
	}
}

// У адресного типа скоуп неприменим: даже если клиент его прислал, он
// затирается, иначе в БД появится строка, противоречащая CHECK-ограничению.
func TestSetClearsScopeForAddressedType(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo)
	err := svc.Set(context.Background(), domain.TenantScope{TenantID: 1}, 1,
		notificationprefs.Preference{Type: notificationprefs.TypeMyCommentResolved, Enabled: true, Scope: "subtree"})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if repo.saved[0].Scope != "" {
		t.Fatalf("скоуп адресного типа должен обнуляться, got %q", repo.saved[0].Scope)
	}
}

// Пустой список каналов означал бы «уведомление некуда доставить»: тихо
// починить осмысленнее, чем сохранить бесполезную настройку.
func TestSetDefaultsEmptyChannelsToInApp(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo)
	err := svc.Set(context.Background(), domain.TenantScope{TenantID: 1}, 1,
		notificationprefs.Preference{Type: notificationprefs.TypeGoalChanged, Enabled: true, Scope: "own"})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if len(repo.saved[0].Channels) != 1 || repo.saved[0].Channels[0] != "in_app" {
		t.Fatalf("got %v, want [in_app]", repo.saved[0].Channels)
	}
}

// A hand-crafted PUT can name a channel this build cannot deliver to yet
// (e.g. "telegram", which phase 2 would honour the moment the entitlement lands).
// Set must reject it now, not silently persist it: the DB has no CHECK constraint
// on channels the way it does on type and scope.
func TestSetRejectsUnknownChannel(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo)
	err := svc.Set(context.Background(), domain.TenantScope{TenantID: 1}, 1,
		notificationprefs.Preference{Type: notificationprefs.TypeGoalChanged, Enabled: true, Scope: "own", Channels: []string{"telegram"}})
	if !errors.Is(err, notificationprefsvc.ErrInvalidChannel) {
		t.Fatalf("got %v, want ErrInvalidChannel", err)
	}
	if len(repo.saved) != 0 {
		t.Error("невалидный канал не должен доходить до стора")
	}
}

// A payload naming a real channel alongside an unknown one must still be rejected
// wholesale, not partially applied.
func TestSetRejectsMixOfKnownAndUnknownChannel(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo)
	err := svc.Set(context.Background(), domain.TenantScope{TenantID: 1}, 1,
		notificationprefs.Preference{Type: notificationprefs.TypeGoalChanged, Enabled: true, Scope: "own", Channels: []string{"in_app", "sms"}})
	if !errors.Is(err, notificationprefsvc.ErrInvalidChannel) {
		t.Fatalf("got %v, want ErrInvalidChannel", err)
	}
}

// Ядро находки ревью: матрица проверяется целиком ДО первой записи. Иначе валидная
// первая строка успевала бы примениться, а ответ сообщал, что матрица отвергнута —
// пользователь получал настройки, которых не просил.
func TestSetAllWritesNothingWhenALaterRowIsInvalid(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo)

	err := svc.SetAll(context.Background(), domain.TenantScope{TenantID: 1}, 42,
		[]notificationprefs.Preference{
			{Type: notificationprefs.TypeGoalComment, Enabled: true, Scope: notificationprefs.ScopeOwn, Channels: []string{"in_app"}},
			{Type: "made_up", Enabled: true, Scope: notificationprefs.ScopeOwn, Channels: []string{"in_app"}},
		})
	if !errors.Is(err, notificationprefsvc.ErrInvalidType) {
		t.Fatalf("err = %v, want ErrInvalidType", err)
	}
	if len(repo.saved) != 0 {
		t.Fatalf("до отказа не должно быть записано ничего, записано: %+v", repo.saved)
	}
}

// Валидная матрица записывается целиком, с подставленными значениями по умолчанию.
func TestSetAllWritesEveryRow(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo)

	err := svc.SetAll(context.Background(), domain.TenantScope{TenantID: 1}, 42,
		[]notificationprefs.Preference{
			{Type: notificationprefs.TypeGoalComment, Enabled: true, Channels: []string{"in_app"}},
			{Type: notificationprefs.TypeKRProgress, Enabled: false, Channels: []string{"in_app"}},
		})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(repo.saved) != 2 {
		t.Fatalf("записано строк: %d, ожидалось 2", len(repo.saved))
	}
	// Пустой scope у не-адресного типа обязан замениться значением по умолчанию —
	// нормализация не должна теряться при переходе на пакетную запись.
	if repo.saved[0].Scope != notificationprefs.ScopeOwn {
		t.Errorf("scope по умолчанию не подставлен: %q", repo.saved[0].Scope)
	}
}
