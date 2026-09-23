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

func (f *fakeRepo) ResolveTenantAdmins(context.Context, domain.TenantScope, string, []int64) ([]notificationprefs.Recipient, error) {
	return nil, nil
}

// fakeChannels — внешние каналы доставки пространства: имя -> включён ли по
// умолчанию у сотрудников. Ровно то, что сервис настроек обязан знать, чтобы
// отличить выбор пользователя от значения администратора.
type fakeChannels map[string]bool

func (f fakeChannels) DeliveryChannelDefaults(context.Context, domain.TenantScope) (map[string]bool, error) {
	return map[string]bool(f), nil
}

func TestSetRejectsUnknownType(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo, fakeChannels{"mattermost": true})
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
	svc := notificationprefsvc.New(repo, fakeChannels{"mattermost": true})
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
	svc := notificationprefsvc.New(repo, fakeChannels{"mattermost": true})
	err := svc.Set(context.Background(), domain.TenantScope{TenantID: 1}, 1,
		notificationprefs.Preference{Type: notificationprefs.TypeMyCommentResolved, Enabled: true, Scope: "subtree"})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if repo.saved[0].Scope != "" {
		t.Fatalf("скоуп адресного типа должен обнуляться, got %q", repo.saved[0].Scope)
	}
}

// A hand-crafted PUT can name a channel this build cannot deliver to yet
// (e.g. "telegram", which phase 2 would honour the moment the entitlement lands).
// Set must reject it now, not silently persist it: the DB has no CHECK constraint
// on channels the way it does on type and scope.
func TestSetRejectsUnknownChannel(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo, fakeChannels{"mattermost": true})
	err := svc.Set(context.Background(), domain.TenantScope{TenantID: 1}, 1,
		notificationprefs.Preference{Type: notificationprefs.TypeGoalChanged, Enabled: true, Scope: "own", ChannelOverrides: map[string]bool{"telegram": true}})
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
	svc := notificationprefsvc.New(repo, fakeChannels{"mattermost": true})
	err := svc.Set(context.Background(), domain.TenantScope{TenantID: 1}, 1,
		notificationprefs.Preference{Type: notificationprefs.TypeGoalChanged, Enabled: true, Scope: "own", ChannelOverrides: map[string]bool{"in_app": true, "sms": true}})
	if !errors.Is(err, notificationprefsvc.ErrInvalidChannel) {
		t.Fatalf("got %v, want ErrInvalidChannel", err)
	}
}

// Ядро находки ревью: матрица проверяется целиком ДО первой записи. Иначе валидная
// первая строка успевала бы примениться, а ответ сообщал, что матрица отвергнута —
// пользователь получал настройки, которых не просил.
func TestSetAllWritesNothingWhenALaterRowIsInvalid(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo, fakeChannels{"mattermost": true})

	err := svc.SetAll(context.Background(), domain.TenantScope{TenantID: 1}, 42, false,
		[]notificationprefs.Preference{
			{Type: notificationprefs.TypeGoalComment, Enabled: true, Scope: notificationprefs.ScopeOwn, ChannelOverrides: map[string]bool{"in_app": true}},
			{Type: "made_up", Enabled: true, Scope: notificationprefs.ScopeOwn, ChannelOverrides: map[string]bool{"in_app": true}},
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
	svc := notificationprefsvc.New(repo, fakeChannels{"mattermost": true})

	err := svc.SetAll(context.Background(), domain.TenantScope{TenantID: 1}, 42, false,
		[]notificationprefs.Preference{
			{Type: notificationprefs.TypeGoalComment, Enabled: true, ChannelOverrides: map[string]bool{"in_app": true}},
			{Type: notificationprefs.TypeKRProgress, Enabled: false, ChannelOverrides: map[string]bool{"in_app": true}},
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

// Пустая карта отклонений — законное состояние: пользователь не высказался ни об
// одном канале, и всё решают значения администратора. Раньше пустой список
// каналов означал «доставлять некуда» и молча чинился на in_app; теперь молча
// чинить нечего — отсутствие выбора это и есть выбор по умолчанию.
func TestNoChoiceIsStoredAsNoChoice(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo, fakeChannels{"mattermost": true})
	err := svc.Set(context.Background(), domain.TenantScope{TenantID: 1}, 1,
		notificationprefs.Preference{Type: notificationprefs.TypeGoalChanged, Enabled: true, Scope: "own"})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if len(repo.saved[0].ChannelOverrides) != 0 {
		t.Fatalf("отсутствие выбора не должно превращаться в явный: %v", repo.saved[0].ChannelOverrides)
	}
}

// Значения по умолчанию пространства: колокольчик всегда есть и всегда включён,
// внешние каналы приходят с признаком, который задал администратор.
func TestDeliveryDefaultsAlwaysCarryTheBell(t *testing.T) {
	svc := notificationprefsvc.New(&fakeRepo{}, fakeChannels{"mattermost": false, "telegram": true})
	got, err := svc.DeliveryDefaults(context.Background(), domain.TenantScope{TenantID: 1})
	if err != nil {
		t.Fatalf("defaults: %v", err)
	}
	want := map[string]bool{"in_app": true, "mattermost": false, "telegram": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("канал %q: got %v, want %v", k, got[k], v)
		}
	}
}

// Сборка без каналов — законная: остаётся один колокольчик.
func TestDeliveryDefaultsWithoutChannels(t *testing.T) {
	svc := notificationprefsvc.New(&fakeRepo{}, nil)
	got, err := svc.DeliveryDefaults(context.Background(), domain.TenantScope{TenantID: 1})
	if err != nil {
		t.Fatalf("defaults: %v", err)
	}
	if len(got) != 1 || !got["in_app"] {
		t.Fatalf("без каналов доставки обязан остаться колокольчик: %v", got)
	}
}

// Каждая присланная ячейка сохраняется как явный выбор — в том числе та, что
// сегодня совпадает со значением администратора. Сохранение матрицы И ЕСТЬ
// выбор: эти переключатели были на экране, и пользователь нажал «Сохранить».
// Иначе смена умолчания позже сдвинула бы ячейку, о которой он уже решил.
func TestEverySubmittedCellIsStoredAsAnExplicitChoice(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo, fakeChannels{"mattermost": true, "telegram": false})

	err := svc.SetAll(context.Background(), domain.TenantScope{TenantID: 1}, 42, false,
		[]notificationprefs.Preference{{
			Type: notificationprefs.TypeGoalChanged, Enabled: true, Scope: "own",
			ChannelOverrides: map[string]bool{
				"in_app":     true,  // совпадает с умолчанием — всё равно выбор
				"mattermost": false, // выключил вопреки умолчанию
				"telegram":   true,  // включил вопреки умолчанию
			},
		}})
	if err != nil {
		t.Fatalf("setAll: %v", err)
	}
	got := repo.saved[0].ChannelOverrides
	want := map[string]bool{"in_app": true, "mattermost": false, "telegram": true}
	if len(got) != len(want) {
		t.Fatalf("сохранено %v, ожидалось %v", got, want)
	}
	for k, v := range want {
		if on, ok := got[k]; !ok || on != v {
			t.Fatalf("ячейка %q: got (%v,%v), want %v", k, on, ok, v)
		}
	}
}

// Ячейка, оставленная в значении по умолчанию, тоже становится явной — и именно
// поэтому переживает последующую смену умолчания администратором.
func TestSavingADefaultValuedCellStillPinsIt(t *testing.T) {
	repo := &fakeRepo{}
	svc := notificationprefsvc.New(repo, fakeChannels{"mattermost": true})
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}

	if err := svc.Set(ctx, scope, 42, notificationprefs.Preference{
		Type: notificationprefs.TypeGoalChanged, Enabled: true, Scope: "own",
		ChannelOverrides: map[string]bool{"mattermost": true}, // = умолчанию
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	stored := repo.saved[0].ChannelOverrides
	if on, ok := stored["mattermost"]; !ok || !on {
		t.Fatalf("совпадение с умолчанием не сохранено как выбор: %v", stored)
	}

	// Администратор выключает канал по умолчанию — закреплённая ячейка не двигается.
	if got := notificationprefsvc.EffectiveChannels(
		map[string]bool{"in_app": true, "mattermost": false}, stored); len(got) != 2 {
		t.Fatalf("явный выбор не пережил смену умолчания: %v", got)
	}
}

// Три состояния ячейки и их разрешение: явное включение, явное выключение и
// «не высказывался» — последнее следует за администратором в обе стороны.
func TestEffectiveChannelsResolvesTheThreeStates(t *testing.T) {
	defaults := map[string]bool{"in_app": true, "mattermost": true, "telegram": false}

	cases := map[string]struct {
		overrides map[string]bool
		want      []string
	}{
		"не высказывался — берём умолчание": {
			overrides: map[string]bool{},
			want:      []string{"in_app", "mattermost"},
		},
		"явно выключил включённый по умолчанию": {
			overrides: map[string]bool{"mattermost": false},
			want:      []string{"in_app"},
		},
		"явно включил выключенный по умолчанию": {
			overrides: map[string]bool{"telegram": true},
			want:      []string{"in_app", "mattermost", "telegram"},
		},
		"отключил всё": {
			overrides: map[string]bool{"in_app": false, "mattermost": false},
			want:      nil,
		},
		"отклонение о канале, которого у пространства нет, ни на что не влияет": {
			overrides: map[string]bool{"sms": true},
			want:      []string{"in_app", "mattermost"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := notificationprefsvc.EffectiveChannels(defaults, tc.overrides)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// Смена значения администратора немедленно отражается на тех, кто не высказывался,
// и не трогает тех, кто высказался. Это правило ретроактивности целиком.
func TestAdminDefaultChangeIsRetroactiveOnlyForTheUndecided(t *testing.T) {
	const ch = "mattermost"
	undecided := map[string]bool{}
	turnedOff := map[string]bool{ch: false}
	turnedOn := map[string]bool{ch: true}

	on := map[string]bool{"in_app": true, ch: true}
	off := map[string]bool{"in_app": true, ch: false}

	has := func(list []string, name string) bool {
		for _, s := range list {
			if s == name {
				return true
			}
		}
		return false
	}

	if !has(notificationprefsvc.EffectiveChannels(on, undecided), ch) {
		t.Error("не высказывавшийся обязан получить канал, включённый администратором")
	}
	if has(notificationprefsvc.EffectiveChannels(off, undecided), ch) {
		t.Error("не высказывавшийся обязан потерять канал, выключенный администратором")
	}
	if has(notificationprefsvc.EffectiveChannels(on, turnedOff), ch) {
		t.Error("явное выключение обязано пережить включение администратором")
	}
	if !has(notificationprefsvc.EffectiveChannels(off, turnedOn), ch) {
		t.Error("явное включение обязано пережить выключение администратором")
	}
}
