package notification_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/store/notificationprefs"
	"okrs/internal/store/notifications"
	notificationuc "okrs/internal/usecase/notification"
)

// fakeWriter records every CreateBatch call. When failAllTenants is set, EVERY
// call fails, tagged with its own tenant id — deliberately not just one tenant.
// Map iteration order over the per-group map in Handle is randomized, so a single
// failing tenant only catches an early-return bug when that tenant happens to be
// visited first; failing every tenant and asserting every tenant's id appears in
// the joined error makes the assertion true regardless of visit order.
type fakeWriter struct {
	rows           []notifications.InsertInput
	calls          int
	failAllTenants bool
}

func (f *fakeWriter) CreateBatch(_ context.Context, scope domain.TenantScope, ins []notifications.InsertInput) error {
	f.calls++
	if f.failAllTenants {
		return fmt.Errorf("tenant %d failed", scope.TenantID)
	}
	f.rows = append(f.rows, ins...)
	return nil
}

// fakePrefs возвращает одного и того же получателя на каждое событие батча, и
// запоминает scope/targets каждого вызова Resolve — некоторые тесты ниже проверяют
// именно то, что дошло до резолвера, а не только факт вызова.
type fakePrefs struct {
	calls          int
	resolveScopes  []domain.TenantScope
	resolveTargets [][]notificationprefs.Target
}

func (f *fakePrefs) Resolve(_ context.Context, scope domain.TenantScope, _ string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	f.calls++
	f.resolveScopes = append(f.resolveScopes, scope)
	f.resolveTargets = append(f.resolveTargets, append([]notificationprefs.Target(nil), targets...))
	out := make([]notificationprefs.Recipient, 0, len(targets))
	for i := range targets {
		out = append(out, notificationprefs.Recipient{Ord: i, UserID: 42, Channels: []string{"in_app"}})
	}
	return out, nil
}

func (f *fakePrefs) ResolveAddressed(_ context.Context, _ domain.TenantScope, _ string, userIDs []int64) ([]notificationprefs.Recipient, error) {
	f.calls++
	out := make([]notificationprefs.Recipient, 0, len(userIDs))
	for i, id := range userIDs {
		out = append(out, notificationprefs.Recipient{Ord: i, UserID: id, Channels: []string{"in_app"}})
	}
	return out, nil
}

// emptyPrefs resolves every group to nobody, used to check CreateBatch is not
// called with an empty slice.
type emptyPrefs struct{}

func (emptyPrefs) Resolve(context.Context, domain.TenantScope, string, []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	return nil, nil
}

func (emptyPrefs) ResolveAddressed(context.Context, domain.TenantScope, string, []int64) ([]notificationprefs.Recipient, error) {
	return nil, nil
}

// badOrdPrefs always answers with an Ord outside the batch it was given, to prove
// the fan-out skips (and does not panic on) a resolver bug rather than trusting it.
type badOrdPrefs struct{}

func (badOrdPrefs) Resolve(_ context.Context, _ domain.TenantScope, _ string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	return []notificationprefs.Recipient{{Ord: len(targets) + 5, UserID: 1, Channels: []string{"in_app"}}}, nil
}

func (badOrdPrefs) ResolveAddressed(context.Context, domain.TenantScope, string, []int64) ([]notificationprefs.Recipient, error) {
	return nil, nil
}

// ordAwarePrefs gives each recipient a UserID DERIVED FROM its Ord (1000+Ord)
// rather than a constant. A constant UserID (as fakePrefs uses) cannot expose a
// scrambled Ord→event lookup: swapping which event a recipient's row is built
// from is invisible when every recipient carries the same UserID anyway. Deriving
// UserID from Ord ties "this resolved recipient" to "this row's content" so a mixup
// shows up as a UserID/GoalID mismatch.
type ordAwarePrefs struct{}

func (ordAwarePrefs) Resolve(_ context.Context, _ domain.TenantScope, _ string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	out := make([]notificationprefs.Recipient, 0, len(targets))
	for i := range targets {
		out = append(out, notificationprefs.Recipient{Ord: i, UserID: int64(1000 + i), Channels: []string{"in_app"}})
	}
	return out, nil
}

func (ordAwarePrefs) ResolveAddressed(context.Context, domain.TenantScope, string, []int64) ([]notificationprefs.Recipient, error) {
	return nil, nil
}

func teamPtr(v int64) *int64 { return &v }

func meta() event.Meta {
	return event.Meta{
		Scope:      domain.TenantScope{TenantID: 1},
		ActorID:    7,
		TeamID:     teamPtr(3),
		OccurredAt: time.Unix(1_700_000_000, 0),
	}
}

func newUC() (*notificationuc.UseCase, *fakeWriter, *fakePrefs) {
	w, p := &fakeWriter{}, &fakePrefs{}
	return notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: p}), w, p
}

// События, которым не соответствует ни один тип уведомления, до записи не доходят.
// Это фиксирует границу goal_changed из спеки §6.1.
func TestNonNotifyingEventsAreIgnored(t *testing.T) {
	uc, w, _ := newUC()
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalShared{Meta: meta(), GoalID: 1, Title: "Цель"},
		event.GoalLinked{Meta: meta(), ChildGoalID: 1, Title: "Цель"},
		event.KRNoteUpdated{Meta: meta(), GoalID: 1, KRID: 2, KRTitle: "KR"},
		event.StatusChanged{Meta: meta(), TeamTitle: "Команда"},
		event.CommentReopened{Meta: meta(), GoalID: 1, CommentID: 2, GoalTitle: "Цель"},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 0 {
		t.Fatalf("эти события не должны порождать уведомлений, got %d", len(w.rows))
	}
}

// Правка цели и двух её KR одним автором в одном окне даёт ОДИН ключ схлопывания:
// ключ строится по цели, а не по KR (спека §6.1, §7.2).
func TestGoalAndItsKRsShareOneCoalesceKey(t *testing.T) {
	uc, w, _ := newUC()
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: meta(), GoalID: 10, Title: "Цель"},
		event.KRFieldsChanged{Meta: meta(), GoalID: 10, KRID: 20, KRTitle: "KR-1"},
		event.KRDeleted{Meta: meta(), GoalID: 10, KRID: 21, KRTitle: "KR-2"},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 3 {
		t.Fatalf("ожидались три вставки (схлопывает БД, не usecase), got %d", len(w.rows))
	}
	first := w.rows[0].CoalesceKey
	for _, r := range w.rows {
		if r.CoalesceKey != first {
			t.Fatalf("ключи разошлись: %q vs %q — ключ обязан строиться по цели", first, r.CoalesceKey)
		}
	}
}

// А вот kr_progress схлопывается по KR: два разных KR — два разных уведомления.
func TestProgressCoalescesPerKR(t *testing.T) {
	uc, w, _ := newUC()
	err := uc.Handle(context.Background(), []event.Event{
		event.KRProgressUpdated{Meta: meta(), GoalID: 10, KRID: 20, KRTitle: "A", After: 50},
		event.KRProgressUpdated{Meta: meta(), GoalID: 10, KRID: 21, KRTitle: "B", After: 70},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if w.rows[0].CoalesceKey == w.rows[1].CoalesceKey {
		t.Fatal("прогресс разных KR не должен схлопываться в одно уведомление")
	}
}

// Адресный тип не ходит в резолв по дереву: получатель уже известен из события.
func TestAddressedTypeUsesAuthorFromEvent(t *testing.T) {
	uc, w, _ := newUC()
	err := uc.Handle(context.Background(), []event.Event{
		event.CommentResolved{Meta: meta(), GoalID: 10, CommentID: 5, GoalTitle: "Цель", AuthorUserID: 99},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 1 || w.rows[0].UserID != 99 {
		t.Fatalf("уведомление должно уйти автору таски, got %+v", w.rows)
	}
	if w.rows[0].Type != notificationprefs.TypeMyCommentResolved {
		t.Errorf("тип: got %q", w.rows[0].Type)
	}
}

// Автор, решивший собственную таску, уведомления не получает.
func TestAuthorResolvingOwnCommentGetsNothing(t *testing.T) {
	uc, w, _ := newUC()
	m := meta()
	err := uc.Handle(context.Background(), []event.Event{
		event.CommentResolved{Meta: m, GoalID: 10, CommentID: 5, GoalTitle: "Цель", AuthorUserID: m.ActorID},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 0 {
		t.Fatal("собственное действие не должно порождать уведомление себе")
	}
}

// Резолв вызывается один раз на тип, а не на событие: иначе батч из 50 событий
// даст 50 рекурсивных запросов (правило 9 CLAUDE.md).
func TestResolveCalledOncePerTypeNotPerEvent(t *testing.T) {
	uc, _, p := newUC()
	evs := make([]event.Event, 0, 10)
	for i := 0; i < 10; i++ {
		evs = append(evs, event.GoalFieldsChanged{Meta: meta(), GoalID: int64(i), Title: "Цель"})
	}
	if err := uc.Handle(context.Background(), evs); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("резолв вызван %d раз на 10 событий одного типа, want 1", p.calls)
	}
}

// События без team_id адресовать некому: тихо пропускаем, а не падаем.
func TestEventWithoutTeamIsSkipped(t *testing.T) {
	uc, w, _ := newUC()
	m := meta()
	m.TeamID = nil
	if err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: m, GoalID: 1, Title: "Цель"},
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 0 {
		t.Fatal("событие без команды нельзя заскоупить — уведомлений быть не должно")
	}
}

// Схлопывание — окно в 10 минут: события в соседних окнах получают РАЗНЫЕ ключи.
func TestCoalesceKeyDiffersAcrossTenMinuteBuckets(t *testing.T) {
	uc, w, _ := newUC()
	m1, m2 := meta(), meta()
	m1.OccurredAt = time.Unix(0, 0)
	m2.OccurredAt = time.Unix(int64(notificationuc.CoalesceWindow.Seconds()), 0) // ровно на окно позже
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: m1, GoalID: 10, Title: "Цель"},
		event.GoalFieldsChanged{Meta: m2, GoalID: 10, Title: "Цель"},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 2 {
		t.Fatalf("ожидались две вставки, got %d", len(w.rows))
	}
	if w.rows[0].CoalesceKey == w.rows[1].CoalesceKey {
		t.Fatalf("события в разных 10-минутных окнах должны получать разные ключи, got одинаковый %q", w.rows[0].CoalesceKey)
	}
}

// А внутри одного окна — ОДИН и тот же ключ.
func TestCoalesceKeySameWithinTenMinuteBucket(t *testing.T) {
	uc, w, _ := newUC()
	m1, m2 := meta(), meta()
	m1.OccurredAt = time.Unix(0, 0)
	m2.OccurredAt = time.Unix(int64(notificationuc.CoalesceWindow.Seconds())-1, 0) // всё ещё то же окно
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: m1, GoalID: 10, Title: "Цель"},
		event.GoalFieldsChanged{Meta: m2, GoalID: 10, Title: "Цель"},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 2 {
		t.Fatalf("ожидались две вставки, got %d", len(w.rows))
	}
	if w.rows[0].CoalesceKey != w.rows[1].CoalesceKey {
		t.Fatalf("события в одном 10-минутном окне должны схлопываться по ключу: %q vs %q", w.rows[0].CoalesceKey, w.rows[1].CoalesceKey)
	}
}

// Нулевой OccurredAt (событие без штампа времени) не должен ни падать, ни давать
// произвольный бакет — код обязан подставить time.Now().
func TestZeroOccurredAtFallsBackToNow(t *testing.T) {
	uc, w, _ := newUC()
	m := meta()
	m.OccurredAt = time.Time{}
	before := time.Now()
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: m, GoalID: 10, Title: "Цель"},
	})
	after := time.Now()
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 1 {
		t.Fatalf("ожидалась одна вставка, got %d", len(w.rows))
	}
	parts := strings.Split(w.rows[0].CoalesceKey, ":")
	bucket, perr := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if perr != nil {
		t.Fatalf("не удалось распарсить бакет из ключа %q: %v", w.rows[0].CoalesceKey, perr)
	}
	window := int64(notificationuc.CoalesceWindow.Seconds())
	minBucket := before.Unix() / window
	maxBucket := after.Unix() / window
	if bucket < minBucket || bucket > maxBucket {
		t.Fatalf("бакет %d не в диапазоне [%d,%d] вокруг time.Now() — нулевой OccurredAt обязан подставляться на time.Now()", bucket, minBucket, maxBucket)
	}
}

// Актор — часть ключа схлопывания: правки одного и того же гоала разными людьми
// не должны схлопываться в одно уведомление.
func TestCoalesceKeyIncludesActor(t *testing.T) {
	uc, w, _ := newUC()
	m1, m2 := meta(), meta()
	m2.ActorID = 8
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: m1, GoalID: 10, Title: "Цель"},
		event.GoalFieldsChanged{Meta: m2, GoalID: 10, Title: "Цель"},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 2 {
		t.Fatalf("ожидались две вставки, got %d", len(w.rows))
	}
	if w.rows[0].CoalesceKey == w.rows[1].CoalesceKey {
		t.Fatal("правки одного гоала разными акторами не должны схлопываться — актор обязан быть в ключе")
	}
}

// Тип уведомления — тоже часть ключа: разные типы по одному и тому же гоалу в одном
// окне не должны схлопываться друг с другом.
func TestCoalesceKeyIncludesType(t *testing.T) {
	uc, w, _ := newUC()
	m := meta()
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: m, GoalID: 10, Title: "Цель"},
		event.CommentAdded{Meta: m, GoalID: 10, CommentID: 1, GoalTitle: "Цель", Text: "hi"},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 2 {
		t.Fatalf("ожидались две вставки, got %d", len(w.rows))
	}
	if w.rows[0].CoalesceKey == w.rows[1].CoalesceKey {
		t.Fatal("goal_changed и goal_comment по одному гоалу не должны схлопываться — тип обязан быть в ключе")
	}
}

// Батч, охватывающий два тенанта, резолвится по каждому тенанту отдельно и с
// правильным scope: перепутать тенанта здесь значит записать строку не туда.
func TestMixedTenantBatchResolvesPerTenant(t *testing.T) {
	uc, w, p := newUC()
	m1, m2 := meta(), meta()
	m2.Scope = domain.TenantScope{TenantID: 2}
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: m1, GoalID: 10, Title: "Цель"},
		event.GoalFieldsChanged{Meta: m2, GoalID: 20, Title: "Цель"},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if p.calls != 2 {
		t.Fatalf("резолв должен быть вызван по разу на тенант, got %d вызовов", p.calls)
	}
	seen := map[int64]bool{}
	for _, s := range p.resolveScopes {
		seen[s.TenantID] = true
	}
	if !seen[1] || !seen[2] {
		t.Fatalf("резолв должен был получить оба тенанта (1 и 2), got scopes %+v", p.resolveScopes)
	}
	if len(w.rows) != 2 {
		t.Fatalf("ожидались две вставки, got %d", len(w.rows))
	}
}

// Резолверу должен уходить настоящий актор события, а не нулевое значение: это
// единственное, что мешает лиду получать уведомления о собственных правках.
func TestResolveReceivesActorFromEvent(t *testing.T) {
	uc, _, p := newUC()
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: meta(), GoalID: 10, Title: "Цель"},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(p.resolveTargets) != 1 || len(p.resolveTargets[0]) != 1 {
		t.Fatalf("ожидался один вызов Resolve с одним target, got %+v", p.resolveTargets)
	}
	got := p.resolveTargets[0][0]
	want := notificationprefs.Target{TeamID: 3, ActorID: 7}
	if got != want {
		t.Fatalf("target = %+v, want %+v", got, want)
	}
}

// Полученные строки обязаны соответствовать СВОЕМУ событию: Ord из Recipient не
// должен перепутать, какая запись какому событию принадлежит. UserID здесь
// специально зависит от Ord (см. ordAwarePrefs) — иначе, если у обоих
// получателей один и тот же UserID, перестановка items[Ord] ничего не меняет
// в наблюдаемом наборе строк (это ровно то, на чём молчала прежняя версия теста).
func TestRowsMapBackToTheirOwnEvent(t *testing.T) {
	w := &fakeWriter{}
	uc := notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: ordAwarePrefs{}})
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: meta(), GoalID: 10, Title: "Цель А"}, // Ord 0 → UserID 1000
		event.GoalFieldsChanged{Meta: meta(), GoalID: 11, Title: "Цель Б"}, // Ord 1 → UserID 1001
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 2 {
		t.Fatalf("ожидались две вставки, got %d", len(w.rows))
	}
	byUser := map[int64]*notifications.InsertInput{}
	for i := range w.rows {
		byUser[w.rows[i].UserID] = &w.rows[i]
	}
	r0, r1 := byUser[1000], byUser[1001]
	if r0 == nil || r1 == nil {
		t.Fatalf("ожидались строки для UserID 1000 и 1001, got %+v", w.rows)
	}
	if r0.GoalID == nil || *r0.GoalID != 10 || r0.EntityTitle != "Цель А" {
		t.Fatalf("получатель Ord 0 (UserID 1000) должен получить строку первого события, got %+v", r0)
	}
	if r1.GoalID == nil || *r1.GoalID != 11 || r1.EntityTitle != "Цель Б" {
		t.Fatalf("получатель Ord 1 (UserID 1001) должен получить строку второго события, got %+v", r1)
	}
}

// notifyType обязан покрывать все события goal_changed, включая создание, копию,
// перенос, смену владельца и создание KR — каждое из них выживало как отдельный
// мутант при их удалении из switch по отдельности.
func TestNotifyTypeCoversAllGoalChangedEvents(t *testing.T) {
	uc, w, _ := newUC()
	m := meta()
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalCreated{Meta: m, GoalID: 1, Title: "Цель"},
		event.GoalCopied{Meta: m, GoalID: 2, Title: "Цель"},
		event.GoalMoved{Meta: m, GoalID: 3, Title: "Цель"},
		event.GoalOwnerChanged{Meta: m, GoalID: 4, Title: "Цель"},
		event.KRCreated{Meta: m, GoalID: 5, KRID: 50, KRTitle: "KR"},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 5 {
		t.Fatalf("ожидались пять вставок (все — goal_changed), got %d", len(w.rows))
	}
	for _, r := range w.rows {
		if r.Type != notificationprefs.TypeGoalChanged {
			t.Errorf("тип = %q, want %q", r.Type, notificationprefs.TypeGoalChanged)
		}
	}
}

// CommentAdded и ReplyAdded оба должны давать goal_comment — ReplyAdded ничем не
// покрывался раньше.
func TestCommentAndReplyNotifyGoalComment(t *testing.T) {
	uc, w, _ := newUC()
	m := meta()
	err := uc.Handle(context.Background(), []event.Event{
		event.CommentAdded{Meta: m, GoalID: 1, CommentID: 10, GoalTitle: "Цель", Text: "hi"},
		event.ReplyAdded{Meta: m, GoalID: 1, CommentID: 10, ParentCommentID: 10, GoalTitle: "Цель", Text: "reply"},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 2 {
		t.Fatalf("ожидались две вставки (обе — goal_comment), got %d", len(w.rows))
	}
	for _, r := range w.rows {
		if r.Type != notificationprefs.TypeGoalComment {
			t.Errorf("тип = %q, want %q", r.Type, notificationprefs.TypeGoalComment)
		}
	}
}

// Один упавший тенант не должен стоить остальным их строк: сравни с
// service/activity/journal.go, где та же гарантия для батча, охватывающего
// несколько тенантов.
//
// Обе группы (оба тенанта) намеренно падают, а не только одна: Handle обходит map
// групп, порядок обхода которой рандомизирован в Go, так что тест с ОДНИМ упавшим
// тенантом ловит баг "return err на первой ошибке" только когда упавший тенант
// обходится первым — то есть не в 100% прогонов. Когда падают ОБА, объединённая
// ошибка обязана упоминать оба id независимо от порядка обхода: под багом она
// содержит только id тенанта, обработанного первым.
func TestAllTenantFailuresAreJoinedRegardlessOfOrder(t *testing.T) {
	w := &fakeWriter{failAllTenants: true}
	p := &fakePrefs{}
	uc := notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: p})
	m1, m2 := meta(), meta()
	m2.Scope = domain.TenantScope{TenantID: 2}
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: m1, GoalID: 10, Title: "Цель"},
		event.GoalFieldsChanged{Meta: m2, GoalID: 20, Title: "Цель"},
	})
	if err == nil {
		t.Fatal("ожидалась объединённая ошибка от обоих тенантов")
	}
	if !strings.Contains(err.Error(), "tenant 1 failed") || !strings.Contains(err.Error(), "tenant 2 failed") {
		t.Fatalf("ошибка обязана упоминать оба тенанта независимо от порядка обхода map, got %q", err.Error())
	}
	if len(w.rows) != 0 {
		t.Fatalf("оба тенанта упали — строк быть не должно, got %+v", w.rows)
	}
}

// CoalesceWindow сама ВЕЛИЧИНА окна задана спекой (§7.2 — ровно 10 минут), а не
// только поведение делителя. Тесты на бакеты выше берут офсеты из самой константы,
// поэтому они движутся вместе с ней и не ловят смену её значения; этот тест
// закрепляет значение напрямую, литералом.
func TestCoalesceWindowIsTenMinutes(t *testing.T) {
	if notificationuc.CoalesceWindow != 10*time.Minute {
		t.Fatalf("CoalesceWindow = %v, want 10m per spec §7.2", notificationuc.CoalesceWindow)
	}
}

// Ord вне диапазона — баг резолвера, а не повод паниковать на весь батч (тем более
// что задача 10 подписывает этот хендлер асинхронно, и шина глотает паники).
func TestOutOfRangeOrdIsSkippedNotPanicked(t *testing.T) {
	w := &fakeWriter{}
	uc := notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: badOrdPrefs{}})
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: meta(), GoalID: 10, Title: "Цель"},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 0 {
		t.Fatalf("получатель с некорректным Ord должен быть пропущен, а не превращён в строку, got %d", len(w.rows))
	}
}

// Группа, резолвящаяся в пустой список получателей, не должна дёргать CreateBatch
// вовсе — с пустым списком это было бы бессмысленным вызовом.
func TestEmptyRecipientGroupDoesNotCallCreateBatch(t *testing.T) {
	w := &fakeWriter{}
	uc := notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: emptyPrefs{}})
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: meta(), GoalID: 10, Title: "Цель"},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if w.calls != 0 {
		t.Fatalf("CreateBatch не должен вызываться для пустой группы получателей, got %d вызовов", w.calls)
	}
}
