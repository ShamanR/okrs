package adminrole

// Обёртка над admincommon.SetMemberRole: пакет отличается только ролью,
// которую назначает, — это и проверяется.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/handlertest"
)

type fakeRoles struct {
	gotRole domain.Role
	gotUser int64
	calls   int
}

func (f *fakeRoles) SetMemberRole(_ context.Context, _ domain.TenantScope, userID int64, role domain.Role) error {
	f.calls++
	f.gotUser, f.gotRole = userID, role
	return nil
}

const uri = "/api/v1/admin/users/7/admin"

func TestRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(&fakeRoles{}).Post, http.MethodPost, uri, "",
		handlertest.URLParam("userID", "7"))
	handlertest.IsError(t, w, http.StatusForbidden)
}

func TestBadUserIDIs400(t *testing.T) {
	w := handlertest.Do(New(&fakeRoles{}).Post, http.MethodPost, uri, "",
		handlertest.Tenant(1), handlertest.URLParam("userID", "не-число"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

// POST выдаёт роль администратора в активном tenant, DELETE — снимает её до
// обычного пользователя. Роль — единственное, чем два метода различаются.
func TestPostGrantsAdminAndDeleteRevokes(t *testing.T) {
	cases := []struct {
		name string
		call func(*fakeRoles) *httptest.ResponseRecorder
		want domain.Role
	}{
		{"Post", func(f *fakeRoles) *httptest.ResponseRecorder {
			return handlertest.Do(New(f).Post, http.MethodPost, uri, "",
				handlertest.Tenant(1), handlertest.URLParam("userID", "7"))
		}, domain.RoleAdmin},
		{"Delete", func(f *fakeRoles) *httptest.ResponseRecorder {
			return handlertest.Do(New(f).Delete, http.MethodDelete, uri, "",
				handlertest.Tenant(1), handlertest.URLParam("userID", "7"))
		}, domain.RoleUser},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeRoles{}
			w := c.call(f)
			handlertest.Status(t, w, http.StatusNoContent)
			if f.calls != 1 {
				t.Fatalf("SetMemberRole вызван %d раз, want 1", f.calls)
			}
			if f.gotRole != c.want {
				t.Fatalf("роль = %q, want %q", f.gotRole, c.want)
			}
			if f.gotUser != 7 {
				t.Fatalf("userID = %d, want 7", f.gotUser)
			}
		})
	}
}
