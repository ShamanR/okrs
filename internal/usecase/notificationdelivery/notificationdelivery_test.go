package notificationdelivery_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/store/users"
	notificationuc "okrs/internal/usecase/notification"
	delivery "okrs/internal/usecase/notificationdelivery"
	"okrs/notifychannel"
)

var scope = domain.TenantScope{TenantID: 1}

type sent struct {
	target notifychannel.Target
	msg    notifychannel.Message
}

// recordingSender принимает сообщения, как это делает настоящий канал: Send
// означает «принято», а не «доставлено».
type recordingSender struct {
	mu       sync.Mutex
	accepted []sent
	err      error
}

func (s *recordingSender) Send(_ context.Context, t notifychannel.Target, m notifychannel.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accepted = append(s.accepted, sent{target: t, msg: m})
	return s.err
}

func (s *recordingSender) SendNow(ctx context.Context, t notifychannel.Target, m notifychannel.Message) error {
	return s.Send(ctx, t, m)
}

func (s *recordingSender) Flush(context.Context) error { return nil }

type fakeChannels struct {
	senders map[string]*recordingSender
	errs    map[string]error
	calls   map[string]int
}

func newChannels(names ...string) *fakeChannels {
	c := &fakeChannels{
		senders: map[string]*recordingSender{},
		errs:    map[string]error{},
		calls:   map[string]int{},
	}
	for _, n := range names {
		c.senders[n] = &recordingSender{}
	}
	return c
}

func (c *fakeChannels) Sender(_ context.Context, _ domain.TenantScope, name string) (notifychannel.Sender, error) {
	c.calls[name]++
	if err, ok := c.errs[name]; ok {
		return nil, err
	}
	s, ok := c.senders[name]
	if !ok {
		return nil, errors.New("unknown channel " + name)
	}
	return s, nil
}

type fakeContacts struct {
	people map[int64]users.Contact
	calls  int
	gotIDs []int64
}

func (f *fakeContacts) ContactsByIDs(_ context.Context, ids []int64) (map[int64]users.Contact, error) {
	f.calls++
	f.gotIDs = append(f.gotIDs, ids...)
	out := map[int64]users.Contact{}
	for _, id := range ids {
		if c, ok := f.people[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

func people() *fakeContacts {
	return &fakeContacts{people: map[int64]users.Contact{
		1: {ID: 1, DisplayName: "Пётр", Email: "petr@example.com"},
		2: {ID: 2, DisplayName: "Мария", Email: "maria@example.com"},
		3: {ID: 3, DisplayName: "Иван", Email: ""},
	}}
}

func goalPtr(v int64) *int64 { return &v }

func item(userID, actorID int64, channels ...string) notificationuc.Delivery {
	return notificationuc.Delivery{
		UserID:      userID,
		ActorUserID: actorID,
		Channels:    channels,
		Kind:        string(event.KindCommentAdded),
		EntityTitle: "Снизить отток",
		Count:       1,
		Payload:     map[string]any{"text": "и вот почему"},
		GoalID:      goalPtr(7),
		TeamID:      goalPtr(3),
		PeriodID:    goalPtr(9),
		CommentID:   goalPtr(11),
	}
}

// Сообщение уходит в каждый канал получателя, адресуется его почтой и несёт тот
// же текст, которым уведомление показывается в ленте.
func TestDeliversToEveryChannelOfTheRecipient(t *testing.T) {
	ch := newChannels("mattermost", "telegram")
	uc := delivery.New(delivery.Deps{
		Channels: ch, Contacts: people(), BaseURL: "https://okr.example.com",
	})

	err := uc.Deliver(context.Background(), scope,
		[]notificationuc.Delivery{item(1, 2, "mattermost", "telegram")})
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}

	for _, name := range []string{"mattermost", "telegram"} {
		s := ch.senders[name]
		if len(s.accepted) != 1 {
			t.Fatalf("канал %s принял %d сообщений, ожидалось 1", name, len(s.accepted))
		}
		got := s.accepted[0]
		if got.target.Email != "petr@example.com" {
			t.Fatalf("канал %s адресован не получателем: %+v", name, got.target)
		}
		if got.msg.Title == "" {
			t.Fatalf("канал %s получил сообщение без заголовка", name)
		}
		if !strings.Contains(got.msg.Title, "Мария") {
			t.Fatalf("сообщение не называет автора события: %q", got.msg.Title)
		}
		if !strings.Contains(got.msg.URL, "goal=7") {
			t.Fatalf("ссылка не ведёт к цели: %q", got.msg.URL)
		}
	}
}

// Адреса и имена резолвятся один раз на пачку. Ровно то же правило, по которому
// пакетным сделан резолв получателей: доставка одного батча не должна давать
// запрос на каждое сообщение.
func TestContactsResolvedOncePerBatch(t *testing.T) {
	ch := newChannels("mattermost")
	contacts := people()
	uc := delivery.New(delivery.Deps{Channels: ch, Contacts: contacts})

	items := []notificationuc.Delivery{
		item(1, 2, "mattermost"),
		item(2, 1, "mattermost"),
		item(1, 2, "mattermost"),
	}
	if err := uc.Deliver(context.Background(), scope, items); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if contacts.calls != 1 {
		t.Fatalf("адреса резолвились %d раз на пачку, ожидался 1", contacts.calls)
	}
	// Один вызов покрывает и тех, кого называют, и тех, кому адресуют, без дублей.
	seen := map[int64]int{}
	for _, id := range contacts.gotIDs {
		seen[id]++
	}
	if len(seen) != 2 || seen[1] != 1 || seen[2] != 1 {
		t.Fatalf("идентификаторы запрошены неэкономно: %v", contacts.gotIDs)
	}
	if ch.calls["mattermost"] != 1 {
		t.Fatalf("отправитель канала запрашивался %d раз на пачку, ожидался 1", ch.calls["mattermost"])
	}
}

// Получателю без адреса отправлять некуда. Это не повод уронить всю пачку:
// остальные обязаны получить своё.
func TestRecipientWithoutAnAddressIsSkippedWithoutStoppingTheBatch(t *testing.T) {
	ch := newChannels("mattermost")
	uc := delivery.New(delivery.Deps{Channels: ch, Contacts: people()})

	err := uc.Deliver(context.Background(), scope, []notificationuc.Delivery{
		item(3, 2, "mattermost"), // без почты
		item(1, 2, "mattermost"),
	})
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	s := ch.senders["mattermost"]
	if len(s.accepted) != 1 {
		t.Fatalf("принято %d сообщений, ожидалось одно (второй получатель без адреса)", len(s.accepted))
	}
	if s.accepted[0].target.Email != "petr@example.com" {
		t.Fatalf("ушло не тому: %+v", s.accepted[0].target)
	}
}

// Автор, которого больше нет, не оставляет сообщение без подписи.
func TestUnknownActorFallsBackToANeutralName(t *testing.T) {
	ch := newChannels("mattermost")
	uc := delivery.New(delivery.Deps{Channels: ch, Contacts: people()})

	if err := uc.Deliver(context.Background(), scope,
		[]notificationuc.Delivery{item(1, 999, "mattermost")}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	msg := ch.senders["mattermost"].accepted[0].msg
	if strings.TrimSpace(msg.Title) == "" {
		t.Fatal("сообщение осталось без заголовка")
	}
	if !strings.Contains(msg.Title, "Бывший участник") {
		t.Fatalf("неизвестный автор обязан получить нейтральную подпись: %q", msg.Title)
	}
}

// Непригодный канал не уносит с собой остальные: ошибка возвращается, но
// сообщения в исправный канал уходят.
func TestBrokenChannelDoesNotStopTheOthers(t *testing.T) {
	ch := newChannels("mattermost")
	ch.errs["telegram"] = errors.New("notificationchannel: channel not configured")
	uc := delivery.New(delivery.Deps{Channels: ch, Contacts: people()})

	err := uc.Deliver(context.Background(), scope,
		[]notificationuc.Delivery{item(1, 2, "telegram", "mattermost")})
	if err == nil {
		t.Fatal("ошибка непригодного канала обязана дойти до вызывающего")
	}
	if len(ch.senders["mattermost"].accepted) != 1 {
		t.Fatal("исправный канал остался без сообщения")
	}
	if ch.calls["telegram"] != 1 {
		t.Fatalf("непригодный канал переспрашивался %d раз, ожидался 1", ch.calls["telegram"])
	}
}

// Пустая пачка не должна ходить ни в базу, ни в каналы.
func TestEmptyBatchDoesNothing(t *testing.T) {
	ch := newChannels("mattermost")
	contacts := people()
	uc := delivery.New(delivery.Deps{Channels: ch, Contacts: contacts})

	if err := uc.Deliver(context.Background(), scope, nil); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if contacts.calls != 0 || len(ch.calls) != 0 {
		t.Fatalf("пустая пачка сходила наружу: contacts=%d channels=%v", contacts.calls, ch.calls)
	}
}

// Ссылка в сообщении внешнего канала обязана быть абсолютной: получатель читает
// его в мессенджере, где «/?team=13&goal=72» либо остаётся нежимаемым текстом,
// либо разрешается относительно хоста самого мессенджера.
func TestDeliveredLinkIsAbsolute(t *testing.T) {
	ch := newChannels("mattermost")
	uc := delivery.New(delivery.Deps{
		Channels: ch, Contacts: people(), BaseURL: "https://okr.example.com",
	})

	if err := uc.Deliver(context.Background(), scope,
		[]notificationuc.Delivery{item(1, 2, "mattermost")}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	got := ch.senders["mattermost"].accepted[0].msg.URL
	if !strings.HasPrefix(got, "https://okr.example.com/?") {
		t.Fatalf("ссылка не абсолютная: %q", got)
	}
	if !strings.Contains(got, "goal=7") || !strings.Contains(got, "comment=11") {
		t.Fatalf("ссылка потеряла адресацию: %q", got)
	}
}

// Завершающий слэш в настройке не должен давать двойной слэш в ссылке.
func TestTrailingSlashInBaseIsNormalised(t *testing.T) {
	ch := newChannels("mattermost")
	uc := delivery.New(delivery.Deps{
		Channels: ch, Contacts: people(), BaseURL: "https://okr.example.com/",
	})
	if err := uc.Deliver(context.Background(), scope,
		[]notificationuc.Delivery{item(1, 2, "mattermost")}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	got := ch.senders["mattermost"].accepted[0].msg.URL
	if strings.Contains(got, "com//") {
		t.Fatalf("двойной слэш в ссылке: %q", got)
	}
}

// Без настроенного адреса ссылка не отправляется вовсе: относительная в
// мессенджере не работает, а указывающая на хост мессенджера — обманывает.
// Само сообщение при этом уходит.
func TestWithoutBaseURLTheLinkIsDroppedNotSentRelative(t *testing.T) {
	ch := newChannels("mattermost")
	uc := delivery.New(delivery.Deps{Channels: ch, Contacts: people()})

	if err := uc.Deliver(context.Background(), scope,
		[]notificationuc.Delivery{item(1, 2, "mattermost")}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	msg := ch.senders["mattermost"].accepted[0].msg
	if msg.URL != "" {
		t.Fatalf("без базового адреса ссылка обязана отсутствовать, got %q", msg.URL)
	}
	if msg.Title == "" {
		t.Fatal("сообщение обязано уйти и без ссылки")
	}
}

// Уведомление без цели ссылки не несёт и без базового адреса ничего не ломает.
func TestNotificationWithoutGoalHasNoLink(t *testing.T) {
	ch := newChannels("mattermost")
	uc := delivery.New(delivery.Deps{
		Channels: ch, Contacts: people(), BaseURL: "https://okr.example.com",
	})
	it := item(1, 2, "mattermost")
	it.GoalID = nil
	if err := uc.Deliver(context.Background(), scope, []notificationuc.Delivery{it}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if got := ch.senders["mattermost"].accepted[0].msg.URL; got != "" {
		t.Fatalf("ссылка появилась без цели: %q", got)
	}
}
