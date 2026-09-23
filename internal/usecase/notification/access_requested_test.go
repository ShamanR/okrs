package notification_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/store/notificationprefs"
	notificationuc "okrs/internal/usecase/notification"
)

// adminPrefs отдаёт двух администраторов (50, 51) на каждое событие, кроме
// самого автора, и запоминает, какая стратегия резолва была вызвана.
type adminPrefs struct {
	adminCalls, treeCalls, addressedCalls int
	actors                                [][]int64
	defaults                              map[string]bool
}

func (p *adminPrefs) Resolve(context.Context, domain.TenantScope, string, []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	p.treeCalls++
	return nil, nil
}

func (p *adminPrefs) ResolveAddressed(context.Context, domain.TenantScope, string, []int64) ([]notificationprefs.Recipient, error) {
	p.addressedCalls++
	return nil, nil
}

func (p *adminPrefs) ResolveTenantAdmins(_ context.Context, _ domain.TenantScope, _ string, actorIDs []int64) ([]notificationprefs.Recipient, error) {
	p.adminCalls++
	p.actors = append(p.actors, append([]int64(nil), actorIDs...))
	var out []notificationprefs.Recipient
	for i, actor := range actorIDs {
		for _, admin := range []int64{50, 51} {
			if admin != actor {
				out = append(out, notificationprefs.Recipient{Ord: i, UserID: admin})
			}
		}
	}
	return out, nil
}

func (p *adminPrefs) DeliveryDefaults(context.Context, domain.TenantScope) (map[string]bool, error) {
	if p.defaults == nil {
		return map[string]bool{notificationprefs.ChannelInApp: true}, nil
	}
	return p.defaults, nil
}

func accessRequested() event.AccessRequested {
	return event.AccessRequested{
		Meta: event.Meta{
			Scope:      domain.TenantScope{TenantID: 1},
			ActorID:    7,
			OccurredAt: time.Unix(1_700_000_000, 0),
		},
		TenantTitle: "Пространство",
	}
}

// Заявка на доступ не имеет команды, но не отбрасывается: получатели — все
// администраторы пространства, найденные одним вызовом на группу.
func TestAccessRequestNotifiesTenantAdmins(t *testing.T) {
	w, p := &fakeWriter{}, &adminPrefs{}
	uc := notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: p})

	if err := uc.Handle(context.Background(), []event.Event{accessRequested()}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if p.adminCalls != 1 || p.treeCalls != 0 || p.addressedCalls != 0 {
		t.Fatalf("резолв: admins=%d tree=%d addressed=%d, want 1/0/0", p.adminCalls, p.treeCalls, p.addressedCalls)
	}
	if len(p.actors) != 1 || len(p.actors[0]) != 1 || p.actors[0][0] != 7 {
		t.Fatalf("в резолвер обязан уйти заявитель как автор события, got %v", p.actors)
	}
	if len(w.rows) != 2 {
		t.Fatalf("строк %d, want 2 — по одной на администратора", len(w.rows))
	}
	for _, r := range w.rows {
		if r.Type != notificationprefs.TypeAccessRequested || r.Kind != string(event.KindAccessRequested) {
			t.Errorf("тип/вид: %q/%q", r.Type, r.Kind)
		}
		if r.EntityTitle != "Пространство" {
			t.Errorf("entity_title: %q, want название пространства", r.EntityTitle)
		}
		if r.ActorUserID != 7 {
			t.Errorf("автор: %d, want заявитель 7", r.ActorUserID)
		}
		if r.TeamID != nil || r.GoalID != nil || r.KRID != nil || r.CommentID != nil {
			t.Errorf("у заявки нет ни команды, ни цели: %+v", r)
		}
		if !strings.Contains(r.CoalesceKey, "access_requested:tenant:1:7:") {
			t.Errorf("ключ схлопывания %q обязан строиться по пространству и заявителю", r.CoalesceKey)
		}
	}
}

// Во внешние каналы уведомление о заявке уходит так же, как любое другое.
func TestAccessRequestIsDelivered(t *testing.T) {
	w, p := &fakeWriter{}, &adminPrefs{
		defaults: map[string]bool{notificationprefs.ChannelInApp: true, "mattermost": true},
	}
	del := &fakeDeliverer{}
	uc := notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: p, Delivery: del})

	if err := uc.Handle(context.Background(), []event.Event{accessRequested()}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(del.items) != 2 {
		t.Fatalf("доставок %d, want 2", len(del.items))
	}
	for _, d := range del.items {
		if d.Kind != string(event.KindAccessRequested) || d.EntityTitle != "Пространство" || d.ActorUserID != 7 {
			t.Errorf("доставка: %+v", d)
		}
		if len(d.Channels) != 1 || d.Channels[0] != "mattermost" {
			t.Errorf("каналы: %v", d.Channels)
		}
		if d.GoalGone {
			t.Error("у заявки нет цели, признак удалённой цели неприменим")
		}
	}
}
