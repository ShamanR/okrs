package preferences_test

import (
	"context"
	"net/http"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/notifications/preferences"
	"okrs/internal/http/handlers/handlertest"
	notificationprefsvc "okrs/internal/service/notificationpref"
	"okrs/internal/store/notificationprefs"
)

// memRepo — хранилище настроек в памяти, чтобы проверить обработчик вместе с
// настоящим сервисом: роль решает, какие типы видны и какие можно сохранить.
type memRepo struct {
	saved []notificationprefs.Preference
}

func (m *memRepo) GetAll(context.Context, domain.TenantScope, int64) ([]notificationprefs.Preference, error) {
	out := make([]notificationprefs.Preference, 0, len(notificationprefs.AllTypes))
	for _, typ := range notificationprefs.AllTypes {
		out = append(out, notificationprefs.Preference{Type: typ, Enabled: notificationprefs.DefaultEnabled(typ)})
	}
	return out, nil
}

func (m *memRepo) Set(_ context.Context, _ domain.TenantScope, _ int64, p notificationprefs.Preference) error {
	m.saved = append(m.saved, p)
	return nil
}

func (m *memRepo) ResolveRecipients(context.Context, domain.TenantScope, string, []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	return nil, nil
}

func (m *memRepo) ResolveAddressed(context.Context, domain.TenantScope, string, []int64) ([]notificationprefs.Recipient, error) {
	return nil, nil
}

func (m *memRepo) ResolveTenantAdmins(context.Context, domain.TenantScope, string, []int64) ([]notificationprefs.Recipient, error) {
	return nil, nil
}

type prefItem struct {
	Type      string `json:"type"`
	Enabled   bool   `json:"enabled"`
	Scope     string `json:"scope"`
	Addressed bool   `json:"addressed"`
	Category  string `json:"category"`
}

func getAs(t *testing.T, role domain.Role) []prefItem {
	t.Helper()
	h := preferences.New(notificationprefsvc.New(&memRepo{}, nil), nil)
	w := handlertest.Do(h.Get, http.MethodGet, "/api/v1/notifications/preferences", "",
		handlertest.Tenant(1), handlertest.UserID(42, "u42"), handlertest.Role(role))
	handlertest.Status(t, w, http.StatusOK)
	var got struct {
		Items []prefItem `json:"items"`
	}
	handlertest.DecodeJSON(t, w, &got)
	return got.Items
}

// Участник видит только категорию «Цели»: заявки на доступ ему не приходят.
func TestGetMemberHasNoSystemCategory(t *testing.T) {
	items := getAs(t, domain.RoleUser)
	if len(items) != 4 {
		t.Fatalf("участнику %d типов, want 4: %+v", len(items), items)
	}
	for _, it := range items {
		if it.Category != notificationprefs.CategoryGoals {
			t.Errorf("%s: категория %q у участника", it.Type, it.Category)
		}
	}
}

// Администратор видит пятый тип — заявку на доступ: в категории «Системные»,
// адресный (без охвата) и выключенный, пока он его не включит.
func TestGetAdminHasAccessRequestInSystemCategory(t *testing.T) {
	items := getAs(t, domain.RoleAdmin)
	if len(items) != 5 {
		t.Fatalf("администратору %d типов, want 5: %+v", len(items), items)
	}
	last := items[len(items)-1]
	if last.Type != notificationprefs.TypeAccessRequested {
		t.Fatalf("системная категория идёт после «Целей», последний тип %q", last.Type)
	}
	if last.Category != notificationprefs.CategorySystem || !last.Addressed || last.Scope != "" || last.Enabled {
		t.Fatalf("заявка на доступ: %+v, want system, addressed, без охвата, выключена", last)
	}
}

// Участник, приславший тип заявки, получает 400, и ничего не сохраняется.
func TestPutMemberWithAccessRequestIs400(t *testing.T) {
	repo := &memRepo{}
	h := preferences.New(notificationprefsvc.New(repo, nil), nil)
	body := `{"items":[
		{"type":"goal_comment","enabled":true,"scope":"own","channels":{"in_app":true}},
		{"type":"access_requested","enabled":true,"channels":{"in_app":true}}]}`

	w := handlertest.Do(h.Put, http.MethodPut, "/api/v1/notifications/preferences", body,
		handlertest.Tenant(1), handlertest.UserID(42, "u42"), handlertest.Role(domain.RoleUser))
	handlertest.Status(t, w, http.StatusBadRequest)
	if len(repo.saved) != 0 {
		t.Fatalf("при отказе ничего не сохраняется, got %+v", repo.saved)
	}

	w = handlertest.Do(h.Put, http.MethodPut, "/api/v1/notifications/preferences", body,
		handlertest.Tenant(1), handlertest.UserID(42, "u42"), handlertest.Role(domain.RoleAdmin))
	handlertest.Status(t, w, http.StatusNoContent)
	if len(repo.saved) != 2 {
		t.Fatalf("администратор сохраняет матрицу целиком, got %d строк", len(repo.saved))
	}
}
