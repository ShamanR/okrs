package notification_test

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/store/notificationprefs"
	notificationuc "okrs/internal/usecase/notification"
)

// Этот usecase решает, КОГО уведомить, а не как разговаривать с мессенджером.
// Канал — это notifychannel.Sender, и знание о нём обязано остаться за портом:
// иначе слой, публикующий уведомления, начнёт зависеть от способа их доставки.
func TestUseCaseDoesNotDependOnChannelTypes(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("чтение каталога пакета: %v", err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Clean(name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("разбор %s: %v", name, err)
		}
		checked++
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("разбор импорта в %s: %v", name, err)
			}
			if path == "okrs/notifychannel" || strings.HasPrefix(path, "okrs/notifychannel/") {
				t.Errorf("%s импортирует %q: доставка обязана оставаться за портом", name, path)
			}
		}
	}
	if checked == 0 {
		t.Fatal("не найдено ни одного файла пакета — тест ничего не проверил")
	}
}

// channelPrefs — резолвер с управляемыми каналами: что пространство даёт по
// умолчанию и что получатель об этом сказал.
type channelPrefs struct {
	defaults      map[string]bool
	overrides     map[string]bool
	defaultsCalls int
}

func (p *channelPrefs) Resolve(_ context.Context, _ domain.TenantScope, _ string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	out := make([]notificationprefs.Recipient, 0, len(targets))
	for i := range targets {
		out = append(out, notificationprefs.Recipient{Ord: i, UserID: 42, ChannelOverrides: p.overrides})
	}
	return out, nil
}

func (p *channelPrefs) ResolveAddressed(_ context.Context, _ domain.TenantScope, _ string, userIDs []int64) ([]notificationprefs.Recipient, error) {
	out := make([]notificationprefs.Recipient, 0, len(userIDs))
	for i, id := range userIDs {
		out = append(out, notificationprefs.Recipient{Ord: i, UserID: id, ChannelOverrides: p.overrides})
	}
	return out, nil
}

func (p *channelPrefs) DeliveryDefaults(context.Context, domain.TenantScope) (map[string]bool, error) {
	p.defaultsCalls++
	if p.defaults == nil {
		return map[string]bool{notificationprefs.ChannelInApp: true}, nil
	}
	return p.defaults, nil
}

// fakeDeliverer записывает, что ушло во внешние каналы.
type fakeDeliverer struct {
	calls int
	items []notificationuc.Delivery
	err   error
}

func (f *fakeDeliverer) Deliver(_ context.Context, _ domain.TenantScope, items []notificationuc.Delivery) error {
	f.calls++
	f.items = append(f.items, items...)
	return f.err
}

func newDeliveringUC(prefs *channelPrefs, del *fakeDeliverer) (*notificationuc.UseCase, *fakeWriter) {
	w := &fakeWriter{}
	return notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: prefs, Delivery: del}), w
}

func commentEvent() event.Event {
	return event.CommentAdded{Meta: meta(), GoalID: 7, CommentID: 1, GoalTitle: "Снизить отток", Text: "и вот почему"}
}

// Уведомление уходит во все внешние каналы, включённые у получателя, и только
// в них. Колокольчик в список доставки не попадает — он и есть записанная строка.
func TestDeliveryGoesToTheRecipientsExternalChannels(t *testing.T) {
	prefs := &channelPrefs{
		defaults:  map[string]bool{"in_app": true, "mattermost": true, "telegram": false},
		overrides: map[string]bool{},
	}
	del := &fakeDeliverer{}
	uc, w := newDeliveringUC(prefs, del)

	if err := uc.Handle(context.Background(), []event.Event{commentEvent()}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 1 {
		t.Fatalf("строка уведомления не записана: %d", len(w.rows))
	}
	if len(del.items) != 1 {
		t.Fatalf("доставок: %d, ожидалась одна", len(del.items))
	}
	got := del.items[0]
	if len(got.Channels) != 1 || got.Channels[0] != "mattermost" {
		t.Fatalf("каналы доставки: %v, ожидался только mattermost", got.Channels)
	}
	if got.UserID != 42 || got.ActorUserID == 0 {
		t.Fatalf("доставка не описывает ни получателя, ни автора: %+v", got)
	}
	if got.EntityTitle == "" || got.Kind == "" {
		t.Fatalf("доставке нечего рендерить: %+v", got)
	}
}

// Отключённый получателем канал в доставку не попадает, а канал, включённый им
// вопреки умолчанию, — попадает.
func TestRecipientChoiceDecidesTheChannels(t *testing.T) {
	cases := map[string]struct {
		overrides map[string]bool
		want      []string
	}{
		"выключил включённый по умолчанию": {
			overrides: map[string]bool{"mattermost": false},
			want:      nil,
		},
		"включил выключенный по умолчанию": {
			overrides: map[string]bool{"telegram": true},
			want:      []string{"mattermost", "telegram"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			prefs := &channelPrefs{
				defaults:  map[string]bool{"in_app": true, "mattermost": true, "telegram": false},
				overrides: tc.overrides,
			}
			del := &fakeDeliverer{}
			uc, w := newDeliveringUC(prefs, del)

			if err := uc.Handle(context.Background(), []event.Event{commentEvent()}); err != nil {
				t.Fatalf("handle: %v", err)
			}
			// Строка пишется всегда — она источник и для ленты, и для дайджеста.
			if len(w.rows) != 1 {
				t.Fatalf("строка уведомления не записана: %d", len(w.rows))
			}
			if len(tc.want) == 0 {
				if del.calls != 0 {
					t.Fatalf("доставка вызвана, хотя внешних каналов нет: %+v", del.items)
				}
				return
			}
			if len(del.items) != 1 {
				t.Fatalf("доставок: %d", len(del.items))
			}
			got := del.items[0].Channels
			if len(got) != len(tc.want) {
				t.Fatalf("каналы: %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("каналы: %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// Значения по умолчанию читаются один раз на группу, а не на получателя: ответ
// самого получателя уже приехал в Recipient, а умолчание внутри одного батча
// измениться не может.
func TestDeliveryDefaultsReadOncePerGroup(t *testing.T) {
	prefs := &channelPrefs{
		defaults:  map[string]bool{"in_app": true, "mattermost": true},
		overrides: map[string]bool{},
	}
	uc, _ := newDeliveringUC(prefs, &fakeDeliverer{})

	evs := []event.Event{commentEvent(), commentEvent(), commentEvent()}
	if err := uc.Handle(context.Background(), evs); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if prefs.defaultsCalls != 1 {
		t.Fatalf("умолчания прочитаны %d раз на одну группу, ожидался 1", prefs.defaultsCalls)
	}
}

// Запись уведомления — предпосылка доставки: если строку записать не удалось,
// рассказывать во внешний канал не о чем.
func TestFailedWriteSkipsDelivery(t *testing.T) {
	prefs := &channelPrefs{
		defaults:  map[string]bool{"in_app": true, "mattermost": true},
		overrides: map[string]bool{},
	}
	del := &fakeDeliverer{}
	w := &fakeWriter{failAllTenants: true}
	uc := notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: prefs, Delivery: del})

	if err := uc.Handle(context.Background(), []event.Event{commentEvent()}); err == nil {
		t.Fatal("ожидалась ошибка записи")
	}
	if del.calls != 0 {
		t.Fatalf("доставка вызвана при несостоявшейся записи: %+v", del.items)
	}
}

// Неудачная доставка не отменяет и не меняет запись уведомления: лента получателя
// от недоступности внешнего сервиса не страдает.
func TestFailedDeliveryLeavesTheNotificationIntact(t *testing.T) {
	prefs := &channelPrefs{
		defaults:  map[string]bool{"in_app": true, "mattermost": true},
		overrides: map[string]bool{},
	}
	del := &fakeDeliverer{err: errors.New("mattermost: posts: status 500")}
	uc, w := newDeliveringUC(prefs, del)

	err := uc.Handle(context.Background(), []event.Event{commentEvent()})
	if err == nil {
		t.Fatal("ошибка доставки обязана дойти до вызывающего, чтобы попасть в лог шины")
	}
	if len(w.rows) != 1 {
		t.Fatalf("строка уведомления пострадала от отказа доставки: %d", len(w.rows))
	}
	if w.rows[0].UserID != 42 {
		t.Fatalf("строка изменилась: %+v", w.rows[0])
	}
}

// Сборка без каналов доставки ведёт себя ровно как прежде: строка пишется,
// умолчания даже не запрашиваются.
func TestBuildWithoutDeliveryIsUnchanged(t *testing.T) {
	prefs := &channelPrefs{overrides: map[string]bool{}}
	w := &fakeWriter{}
	uc := notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: prefs})

	if err := uc.Handle(context.Background(), []event.Event{commentEvent()}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 1 {
		t.Fatalf("строка уведомления не записана: %d", len(w.rows))
	}
	if prefs.defaultsCalls != 0 {
		t.Fatalf("без доставки умолчания каналов читать незачем: %d", prefs.defaultsCalls)
	}
}
