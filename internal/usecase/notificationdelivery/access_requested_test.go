package notificationdelivery_test

import (
	"context"
	"strings"
	"testing"

	"okrs/internal/core/event"
	"okrs/internal/store/users"
	notificationuc "okrs/internal/usecase/notification"
	delivery "okrs/internal/usecase/notificationdelivery"
)

// requestItem — уведомление администратору 1 о заявке пользователя 5.
func requestItem() notificationuc.Delivery {
	return notificationuc.Delivery{
		UserID:      1,
		ActorUserID: 5,
		Channels:    []string{"mattermost"},
		Kind:        string(event.KindAccessRequested),
		EntityTitle: "Маркетинг",
		Count:       1,
		Payload:     map[string]any{},
	}
}

// Заявитель ещё не участник (Removed), но его заявка ожидает решения
// (Requested) — сообщение обязано назвать его по имени.
func withRequester() *fakeContacts {
	c := people()
	c.people[5] = users.Contact{ID: 5, DisplayName: "Ольга", Removed: true, Requested: true}
	return c
}

func TestAccessRequestMessageNamesRequesterTenantAndLinksToQueue(t *testing.T) {
	ch := newChannels("mattermost")
	uc := delivery.New(delivery.Deps{
		Channels: ch, Contacts: withRequester(), BaseURL: "https://okr.example.com",
	})

	if err := uc.Deliver(context.Background(), scope, []notificationuc.Delivery{requestItem()}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	msg := ch.senders["mattermost"].accepted[0].msg
	if !strings.Contains(msg.Body, "Ольга") || !strings.Contains(msg.Body, "Маркетинг") {
		t.Fatalf("сообщение обязано назвать заявителя и пространство: %+v", msg)
	}
	if msg.URL != "https://okr.example.com/admin?section=users&filter=requests" {
		t.Fatalf("ссылка на очередь заявок: %q", msg.URL)
	}
}

func TestAccessRequestMessageWithoutBaseURLHasNoLink(t *testing.T) {
	ch := newChannels("mattermost")
	uc := delivery.New(delivery.Deps{Channels: ch, Contacts: withRequester()})

	if err := uc.Deliver(context.Background(), scope, []notificationuc.Delivery{requestItem()}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	msg := ch.senders["mattermost"].accepted[0].msg
	if msg.URL != "" {
		t.Fatalf("без адреса продукта ссылки нет, got %q", msg.URL)
	}
	if !strings.Contains(msg.Body, "Ольга") {
		t.Fatalf("сообщение уходит и без ссылки: %+v", msg)
	}
}

// Заявка отклонена — заявитель больше не назван по имени.
func TestDeniedRequesterIsNotNamed(t *testing.T) {
	ch := newChannels("mattermost")
	c := people()
	c.people[5] = users.Contact{ID: 5, DisplayName: "Ольга", Removed: true}
	uc := delivery.New(delivery.Deps{Channels: ch, Contacts: c})

	if err := uc.Deliver(context.Background(), scope, []notificationuc.Delivery{requestItem()}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if body := ch.senders["mattermost"].accepted[0].msg.Body; strings.Contains(body, "Ольга") {
		t.Fatalf("бывший участник не называется по имени: %q", body)
	}
}
