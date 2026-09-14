package notificationdelivery_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/platform/entitlements"
	notificationsvc "okrs/internal/service/notification"
	notificationchannelsvc "okrs/internal/service/notificationchannel"
	notificationprefsvc "okrs/internal/service/notificationpref"
	usersvc "okrs/internal/service/user"
	"okrs/internal/store"
	"okrs/internal/store/notificationprefs"
	"okrs/internal/store/notifications"
	storetestutil "okrs/internal/store/testutil"
	notificationuc "okrs/internal/usecase/notification"
	delivery "okrs/internal/usecase/notificationdelivery"
	"okrs/notifychannel"
)

// Эти тесты собирают настоящую цепочку целиком — настройки администратора,
// настройки пользователя, резолв получателей по дереву команд, запись строки и
// доставку в канал — на живой базе. Каждый слой покрыт по отдельности; ни один
// из тех тестов не заметит, что звенья перестали быть соединены.

// bufferingSender ведёт себя как настоящий канал: Send принимает сообщение,
// наружу всё уходит одним сообщением при закрытии окна.
type bufferingSender struct {
	mu   sync.Mutex
	buf  map[string][]notifychannel.Message
	sent map[string][]string // email -> тексты отправленных сообщений
}

func newBufferingSender() *bufferingSender {
	return &bufferingSender{buf: map[string][]notifychannel.Message{}, sent: map[string][]string{}}
}

func (s *bufferingSender) Send(_ context.Context, t notifychannel.Target, m notifychannel.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf[t.Email] = append(s.buf[t.Email], m)
	return nil
}

func (s *bufferingSender) SendNow(_ context.Context, t notifychannel.Target, m notifychannel.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent[t.Email] = append(s.sent[t.Email], m.Title+"\n"+m.Body)
	return nil
}

func (s *bufferingSender) Flush(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for email, msgs := range s.buf {
		var b strings.Builder
		for i, m := range msgs {
			if i > 0 {
				b.WriteString("\n---\n")
			}
			b.WriteString(m.Title)
			b.WriteString("\n")
			b.WriteString(m.Body)
		}
		s.sent[email] = append(s.sent[email], b.String())
	}
	s.buf = map[string][]notifychannel.Message{}
	return nil
}

func (s *bufferingSender) delivered(email string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.sent[email]...)
}

// grantAll выдаёт перечисленные каналы пространству — то, что в проде делает
// системный администратор через ключи tenant_settings.
type grantAll []string

func (g grantAll) TenantEntitlements(context.Context, domain.TenantScope) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	for _, name := range g {
		out["notifications."+name] = json.RawMessage(`true`)
	}
	return out, nil
}

// harness — собранная цепочка и всё, что нужно тесту, чтобы ей управлять.
type harness struct {
	ctx       context.Context
	scope     domain.TenantScope
	channels  *notificationchannelsvc.Service
	prefs     *notificationprefsvc.Service
	notifyUC  *notificationuc.UseCase
	sender    *bufferingSender
	pool      *pgxpool.Pool
	store     *store.Store
	leadID    int64
	leadEmail string
	actorID   int64
	teamID    int64
	periodID  int64
	goalID    int64
}

func setup(t *testing.T) (*harness, func()) {
	t.Helper()
	pool, cleanup := storetestutil.SetupDB(t)
	ctx := context.Background()
	scope := domain.TenantScope{TenantID: 1}
	st := store.New(pool)

	// Получатель — лид команды с адресом; автор события — отдельный человек,
	// иначе он не получил бы уведомления о собственном действии.
	const leadEmail = "lead@example.com"
	var leadID int64
	var leadUDID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (provider_subject_key, provider, subject, display_name, email)
		VALUES ('lead-delivery', 'system', 'lead-delivery', 'Мария', $1)
		RETURNING id, udid`, leadEmail).Scan(&leadID, &leadUDID); err != nil {
		cleanup()
		t.Fatalf("lead: %v", err)
	}
	var actorID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (provider_subject_key, provider, subject, display_name, email)
		VALUES ('actor-delivery', 'system', 'actor-delivery', 'Пётр', 'petr@example.com')
		RETURNING id`).Scan(&actorID); err != nil {
		cleanup()
		t.Fatalf("actor: %v", err)
	}
	for _, id := range []int64{leadID, actorID} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO memberships (user_id, tenant_id, role, status) VALUES ($1, 1, 'user', 'active')`,
			id); err != nil {
			cleanup()
			t.Fatalf("membership: %v", err)
		}
	}

	var teamID, periodID, goalID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO teams (name, team_type, tenant_id, lead_udid) VALUES ('Команда', 'team', 1, $1) RETURNING id`,
		leadUDID).Scan(&teamID); err != nil {
		cleanup()
		t.Fatalf("team: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO periods (name, start_date, end_date) VALUES ('Q1', '2026-01-01', '2026-03-31') RETURNING id`).
		Scan(&periodID); err != nil {
		cleanup()
		t.Fatalf("period: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO goals (tenant_id, team_id, period_id, title, priority, weight, work_type, focus_type)
		VALUES (1, $1, $2, 'Снизить отток', 'P1', 100, 'Delivery', 'STABILITY') RETURNING id`,
		teamID, periodID).Scan(&goalID); err != nil {
		cleanup()
		t.Fatalf("goal: %v", err)
	}

	sender := newBufferingSender()
	channel := notifychannel.Channel{
		Descriptor: notifychannel.Descriptor{Name: "fake", Title: "Тестовый канал"},
		New: func(notifychannel.Deps) (notifychannel.Sender, error) {
			// Один экземпляр на весь тест: именно это и делает реестр в проде,
			// и именно поэтому накопленное переживает несколько отправок.
			return sender, nil
		},
	}
	channelsSvc, err := notificationchannelsvc.New(
		st.NotificationChannels, nil, []notifychannel.Channel{channel},
		entitlements.UnlimitedEntitlements{}, grantAll{"fake"}, nil)
	if err != nil {
		cleanup()
		t.Fatalf("channels: %v", err)
	}
	prefs := notificationprefsvc.New(st.NotificationPrefs, channelsSvc)
	deliveryUC := delivery.New(delivery.Deps{
		Channels: channelsSvc,
		Contacts: usersvc.New(st.Users),
		BaseURL:  "https://okr.example.com",
	})
	notifyUC := notificationuc.New(notificationuc.Deps{
		Notifications: notificationsvc.New(st.Notifications),
		Prefs:         prefs,
		Delivery:      deliveryUC,
	})

	h := &harness{
		ctx: ctx, scope: scope, channels: channelsSvc, prefs: prefs,
		notifyUC: notifyUC, sender: sender, pool: pool, store: st,
		leadID: leadID, leadEmail: leadEmail, actorID: actorID,
		teamID: teamID, periodID: periodID, goalID: goalID,
	}
	return h, cleanup
}

// enableChannel повторяет то, что делает администратор на своём экране.
func (h *harness) enableChannel(t *testing.T, defaultOn bool) {
	t.Helper()
	if err := h.channels.Save(h.ctx, h.scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true, DefaultOn: defaultOn, Values: map[string]any{},
	}, 1); err != nil {
		t.Fatalf("настройка канала: %v", err)
	}
}

func (h *harness) meta() event.Meta {
	team, period := h.teamID, h.periodID
	return event.Meta{
		Scope: h.scope, ActorID: h.actorID, TeamID: &team, PeriodID: &period,
		OccurredAt: time.Now(),
	}
}

func (h *harness) comment(text string) event.Event {
	return event.CommentAdded{
		Meta: h.meta(), GoalID: h.goalID, CommentID: 1,
		GoalTitle: "Снизить отток", Text: text,
	}
}

func (h *harness) goalChanged() event.Event {
	return event.GoalFieldsChanged{
		Meta: h.meta(), GoalID: h.goalID, Title: "Снизить отток",
		Changed: map[string][2]any{"title": {"Отток", "Снизить отток"}},
	}
}

// 8.1 Администратор настраивает канал и включает его по умолчанию, происходит
// несколько событий — получатель получает ОДНО сообщение со всеми обновлениями.
func TestAdminEnablesChannelAndRecipientGetsOneMessageWithEverything(t *testing.T) {
	h, cleanup := setup(t)
	defer cleanup()
	h.enableChannel(t, true)

	if err := h.notifyUC.Handle(h.ctx, []event.Event{
		h.comment("первое"),
		h.comment("второе"),
		h.goalChanged(),
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	// Окно закрылось.
	if err := h.channels.Flush(h.ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	got := h.sender.delivered(h.leadEmail)
	if len(got) != 1 {
		t.Fatalf("ожидалось одно сообщение на окно, got %d: %v", len(got), got)
	}
	// Сотрудник, который ничего не выбирал, получил канал по решению администратора.
	if !strings.Contains(got[0], "Пётр") {
		t.Errorf("сообщение не называет автора: %q", got[0])
	}
	if !strings.Contains(got[0], "Снизить отток") {
		t.Errorf("сообщение не называет цель: %q", got[0])
	}
	if strings.Count(got[0], "---") < 1 {
		t.Errorf("накопленные обновления не собрались в одно сообщение: %q", got[0])
	}
}

// Канал, который администратор НЕ включил по умолчанию, до не высказывавшегося
// сотрудника не доезжает — но строка уведомления всё равно создаётся.
func TestChannelWithoutDefaultDoesNotReachTheUndecided(t *testing.T) {
	h, cleanup := setup(t)
	defer cleanup()
	h.enableChannel(t, false)

	if err := h.notifyUC.Handle(h.ctx, []event.Event{h.comment("текст")}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if err := h.channels.Flush(h.ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got := h.sender.delivered(h.leadEmail); len(got) != 0 {
		t.Fatalf("канал без умолчания доехал сам: %v", got)
	}
}

// 8.2 Получатель отключает один тип в одном канале — остальные типы продолжают
// приходить.
func TestDisablingOneTypeInOneChannelLeavesTheRestWorking(t *testing.T) {
	h, cleanup := setup(t)
	defer cleanup()
	h.enableChannel(t, true)

	// Выключаем только «изменение цели» и только в этом канале.
	if err := h.prefs.Set(h.ctx, h.scope, h.leadID, notificationprefs.Preference{
		Type: notificationprefs.TypeGoalChanged, Enabled: true, Scope: notificationprefs.ScopeOwn,
		ChannelOverrides: map[string]bool{"fake": false},
	}); err != nil {
		t.Fatalf("настройка пользователя: %v", err)
	}

	if err := h.notifyUC.Handle(h.ctx, []event.Event{h.goalChanged(), h.comment("замечание")}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if err := h.channels.Flush(h.ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	got := h.sender.delivered(h.leadEmail)
	if len(got) != 1 {
		t.Fatalf("ожидалось одно сообщение, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "замечание") {
		t.Errorf("не отключённый тип не доехал: %q", got[0])
	}
	if strings.Contains(got[0], "---") {
		t.Errorf("отключённый тип всё-таки попал в сообщение: %q", got[0])
	}

	// Обе строки в ленте на месте: выбор канала решает только доставку, но не
	// то, создаётся ли запись. Иначе выключенный внешний канал незаметно
	// отбирал бы уведомление и у колокольчика.
	var rows int
	if err := h.pool.QueryRow(h.ctx,
		`SELECT count(*) FROM notifications WHERE tenant_id = 1 AND user_id = $1`,
		h.leadID).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 2 {
		t.Fatalf("строк уведомлений: %d, ожидалось 2 (оба типа)", rows)
	}
}

// 8.3 Канал, подключённый ПОСЛЕ того, как пользователь сохранил настройки,
// доезжает до него без каких-либо его действий. Это и есть причина, по которой
// хранится карта отклонений, а не список включённых каналов.
func TestChannelConnectedLaterStillReachesAnExistingUser(t *testing.T) {
	h, cleanup := setup(t)
	defer cleanup()

	// Пользователь сохраняет настройки, когда внешних каналов ещё нет.
	if err := h.prefs.SetAll(h.ctx, h.scope, h.leadID, []notificationprefs.Preference{
		{Type: notificationprefs.TypeGoalComment, Enabled: true, Scope: notificationprefs.ScopeOwn,
			ChannelOverrides: map[string]bool{notificationprefs.ChannelInApp: true}},
	}); err != nil {
		t.Fatalf("настройки до подключения канала: %v", err)
	}

	// Администратор подключает канал позже.
	h.enableChannel(t, true)

	if err := h.notifyUC.Handle(h.ctx, []event.Event{h.comment("после подключения")}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if err := h.channels.Flush(h.ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	got := h.sender.delivered(h.leadEmail)
	if len(got) != 1 {
		t.Fatalf("канал, подключённый позже, не доехал до сохранившего настройки: %v", got)
	}
}

// Явный выбор пользователя переживает изменение решения администратора.
func TestExplicitUserChoiceSurvivesTheAdminChangingTheirMind(t *testing.T) {
	h, cleanup := setup(t)
	defer cleanup()
	h.enableChannel(t, true)

	// Пользователь отказался от канала.
	if err := h.prefs.Set(h.ctx, h.scope, h.leadID, notificationprefs.Preference{
		Type: notificationprefs.TypeGoalComment, Enabled: true, Scope: notificationprefs.ScopeOwn,
		ChannelOverrides: map[string]bool{"fake": false},
	}); err != nil {
		t.Fatalf("отказ пользователя: %v", err)
	}
	// Администратор ещё раз подтверждает «включён по умолчанию у всех».
	h.enableChannel(t, true)

	if err := h.notifyUC.Handle(h.ctx, []event.Event{h.comment("текст")}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if err := h.channels.Flush(h.ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got := h.sender.delivered(h.leadEmail); len(got) != 0 {
		t.Fatalf("явный отказ пользователя перебит решением администратора: %v", got)
	}
}

// Выключенный администратором канал не доставляет ничего — даже тому, кто явно
// его себе включил. Доступность канала решает пространство, а настройка
// пользователя лишь сужает то, что пространство разрешило.
func TestChannelDisabledByAdminDeliversNothingEvenWhenTheUserWantsIt(t *testing.T) {
	h, cleanup := setup(t)
	defer cleanup()
	h.enableChannel(t, true)

	// Пользователь явно включает канал себе.
	if err := h.prefs.Set(h.ctx, h.scope, h.leadID, notificationprefs.Preference{
		Type: notificationprefs.TypeGoalComment, Enabled: true, Scope: notificationprefs.ScopeOwn,
		ChannelOverrides: map[string]bool{"fake": true},
	}); err != nil {
		t.Fatalf("настройка пользователя: %v", err)
	}
	// Администратор выключает канал целиком.
	if err := h.channels.Save(h.ctx, h.scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: false, DefaultOn: true, Values: map[string]any{},
	}, 1); err != nil {
		t.Fatalf("выключение канала: %v", err)
	}

	if err := h.notifyUC.Handle(h.ctx, []event.Event{h.comment("текст")}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if err := h.channels.Flush(h.ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got := h.sender.delivered(h.leadEmail); len(got) != 0 {
		t.Fatalf("выключенный администратором канал всё-таки доставил: %v", got)
	}

	// Лента при этом работает: выключен канал, а не уведомление.
	items, _, err := h.store.Notifications.List(h.ctx, h.scope, h.leadID, notifications.ListFilter{Limit: 20})
	if err != nil {
		t.Fatalf("лента: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("уведомление обязано остаться в ленте: %d", len(items))
	}
}

// Колокольчик выключен, внешний канал включён: уведомления нет в ленте и в
// счётчике, но во внешний канал оно уходит — строка пишется как журнальная
// запись, из которой и собирается сообщение.
func TestBellOffExternalOnDeliversOutsideButNotInTheFeed(t *testing.T) {
	h, cleanup := setup(t)
	defer cleanup()
	h.enableChannel(t, true)

	if err := h.prefs.Set(h.ctx, h.scope, h.leadID, notificationprefs.Preference{
		Type: notificationprefs.TypeGoalComment, Enabled: true, Scope: notificationprefs.ScopeOwn,
		ChannelOverrides: map[string]bool{notificationprefs.ChannelInApp: false},
	}); err != nil {
		t.Fatalf("настройка пользователя: %v", err)
	}

	if err := h.notifyUC.Handle(h.ctx, []event.Event{h.comment("важное")}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if err := h.channels.Flush(h.ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	got := h.sender.delivered(h.leadEmail)
	if len(got) != 1 || !strings.Contains(got[0], "важное") {
		t.Fatalf("внешний канал не получил уведомление: %v", got)
	}

	items, _, err := h.store.Notifications.List(h.ctx, h.scope, h.leadID, notifications.ListFilter{Limit: 20})
	if err != nil {
		t.Fatalf("лента: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("выключенный колокольчик обязан убрать уведомление из ленты: %d", len(items))
	}
	if n, err := h.store.Notifications.UnreadCount(h.ctx, h.scope, h.leadID); err != nil || n != 0 {
		t.Fatalf("счётчик непрочитанного: got (%d, %v), want (0, nil)", n, err)
	}

	// Строка на месте — именно из неё собралось отправленное сообщение.
	var rows int
	if err := h.pool.QueryRow(h.ctx,
		`SELECT count(*) FROM notifications WHERE tenant_id = 1 AND user_id = $1`,
		h.leadID).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Fatalf("запись уведомления обязана существовать: %d", rows)
	}
}

// Отключённый целиком тип не порождает ни строки, ни доставки: решение «не
// уведомлять» принимается раньше, чем вопрос «куда доставлять».
func TestTypeDisabledEntirelyProducesNeitherRowNorDelivery(t *testing.T) {
	h, cleanup := setup(t)
	defer cleanup()
	h.enableChannel(t, true)

	if err := h.prefs.Set(h.ctx, h.scope, h.leadID, notificationprefs.Preference{
		Type: notificationprefs.TypeGoalComment, Enabled: false, Scope: notificationprefs.ScopeOwn,
	}); err != nil {
		t.Fatalf("настройка пользователя: %v", err)
	}

	if err := h.notifyUC.Handle(h.ctx, []event.Event{h.comment("текст")}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if err := h.channels.Flush(h.ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if got := h.sender.delivered(h.leadEmail); len(got) != 0 {
		t.Fatalf("отключённый тип всё-таки доставлен: %v", got)
	}
	var rows int
	if err := h.pool.QueryRow(h.ctx,
		`SELECT count(*) FROM notifications WHERE tenant_id = 1 AND user_id = $1`,
		h.leadID).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 0 {
		t.Fatalf("отключённый тип не должен порождать строку: %d", rows)
	}
}
