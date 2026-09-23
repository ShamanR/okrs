package notificationdelivery_test

import (
	"context"
	"strings"
	"testing"

	"okrs/internal/core/event"
	onboardingsvc "okrs/internal/service/onboarding"
	settingssvc "okrs/internal/service/settings"
	"okrs/internal/store/grants"
	"okrs/internal/store/memberships"
	"okrs/internal/store/notificationprefs"
	"okrs/internal/store/notifications"
	"okrs/internal/store/settings"
	"okrs/internal/store/tenantsettings"
	notificationuc "okrs/internal/usecase/notification"
)

// syncBus передаёт опубликованное событие прямо в fan-out уведомлений — так
// тест видит результат без асинхронной шины.
type syncBus struct {
	t  *testing.T
	uc *notificationuc.UseCase
}

func (b syncBus) Publish(ctx context.Context, ev event.Event) {
	if err := b.uc.Handle(ctx, []event.Event{ev}); err != nil {
		b.t.Errorf("fan-out: %v", err)
	}
}

// Сквозной путь заявки на доступ: сервис онбординга → событие → администратор
// пространства в колокольчике и во внешнем канале. Системный тип выключен по
// умолчанию, поэтому до явного включения администратором не приходит ничего.
func TestAccessRequestReachesTheAdminOnlyAfterOptIn(t *testing.T) {
	h, cleanup := setup(t)
	defer cleanup()
	h.enableChannel(t, true)

	// Лид из харнеса становится администратором пространства.
	if _, err := h.pool.Exec(h.ctx,
		`UPDATE memberships SET role = 'admin' WHERE user_id = $1 AND tenant_id = 1`, h.leadID); err != nil {
		t.Fatalf("admin: %v", err)
	}

	st := h.store
	sysRepo := settings.NewSettingsRepository(h.pool)
	tsRepo := tenantsettings.NewTenantSettingsRepository(h.pool)
	onboarding := onboardingsvc.New(
		st.Invitations, st.Memberships, memberships.NewMembershipCache(st.Memberships), st.Tenants,
		settingssvc.New(tenantsettings.NewTenantSettingsCache(tsRepo), tsRepo,
			settings.NewSystemSettingsCache(sysRepo), sysRepo),
		grants.NewGrantsCache(grants.NewGrantRepository(h.pool)),
		syncBus{t: t, uc: h.notifyUC},
	)

	newUser := func(key, name string) int64 {
		t.Helper()
		var id int64
		if err := h.pool.QueryRow(h.ctx, `
			INSERT INTO users (provider_subject_key, provider, subject, display_name, email)
			VALUES ($1, 'google', $1, $2, $1 || '@example.com') RETURNING id`, key, name).Scan(&id); err != nil {
			t.Fatalf("user %s: %v", key, err)
		}
		return id
	}
	feed := func() []notifications.Notification {
		t.Helper()
		items, _, err := notifications.NewRepository(h.pool).List(h.ctx, h.scope, h.leadID,
			notifications.ListFilter{Limit: 50})
		if err != nil {
			t.Fatalf("feed: %v", err)
		}
		var out []notifications.Notification
		for _, n := range items {
			if n.Type == notificationprefs.TypeAccessRequested {
				out = append(out, n)
			}
		}
		return out
	}

	// 1. Тип не включён — заявка никого не уведомляет.
	early := newUser("early-requester", "Ранний")
	if err := onboarding.RequestAccess(h.ctx, "default", early); err != nil {
		t.Fatalf("early request: %v", err)
	}
	if got := feed(); len(got) != 0 {
		t.Fatalf("до включения типа уведомлений быть не должно, got %+v", got)
	}

	// 2. Администратор включает тип — следующая заявка до него доходит.
	if err := h.prefs.SetAll(h.ctx, h.scope, h.leadID, true, []notificationprefs.Preference{
		{Type: notificationprefs.TypeAccessRequested, Enabled: true},
	}); err != nil {
		t.Fatalf("opt in: %v", err)
	}
	requester := newUser("requester", "Ольга")
	if err := onboarding.RequestAccess(h.ctx, "default", requester); err != nil {
		t.Fatalf("request: %v", err)
	}
	got := feed()
	if len(got) != 1 {
		t.Fatalf("уведомлений о заявке %d, want 1: %+v", len(got), got)
	}
	if got[0].ActorUserID != requester || got[0].ActorRemoved || got[0].ActorDisplayName != "Ольга" {
		t.Fatalf("заявитель обязан быть назван по имени: %+v", got[0])
	}

	// 3. Повторная отправка той же заявки нового уведомления не даёт.
	if err := onboarding.RequestAccess(h.ctx, "default", requester); err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if again := feed(); len(again) != 1 || again[0].CoalesceCount != 1 {
		t.Fatalf("повторная заявка не уведомляет: %+v", again)
	}

	// Во внешний канал ушло одно сообщение, называющее заявителя и пространство.
	// Ссылку тестовый канал не записывает; её формат проверяет
	// TestAccessRequestMessageNamesRequesterTenantAndLinksToQueue.
	if err := h.channels.Close(h.ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	msgs := h.sender.delivered(h.leadEmail)
	if len(msgs) != 1 {
		t.Fatalf("сообщений администратору %d, want 1: %v", len(msgs), msgs)
	}
	for _, want := range []string{"Заявка на доступ", "Ольга", "Default"} {
		if !strings.Contains(msgs[0], want) {
			t.Errorf("сообщение не содержит %q: %s", want, msgs[0])
		}
	}
}
