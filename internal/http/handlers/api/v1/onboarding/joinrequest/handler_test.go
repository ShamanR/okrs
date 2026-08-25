package joinrequest

// Единственный публичный эндпоинт онбординга: аутентифицированный пользователь
// просится в организацию по slug. Проверяются гейт аутентификации, валидация
// тела и маппинг доменных ошибок в коды ответа.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/handlertest"
	onboardingsvc "okrs/internal/service/onboarding"
	"okrs/internal/store/memberships"
)

type fakeOnboard struct {
	err     error
	gotSlug string
	gotUser int64
}

func (f *fakeOnboard) RequestAccess(_ context.Context, slug string, userID int64) error {
	f.gotSlug, f.gotUser = slug, userID
	return f.err
}
func (f *fakeOnboard) ListAccessRequests(context.Context, domain.TenantScope) ([]memberships.AccessRequest, error) {
	return nil, nil
}
func (f *fakeOnboard) ApproveRequest(context.Context, domain.TenantScope, int64) error { return nil }
func (f *fakeOnboard) DenyRequest(context.Context, domain.TenantScope, int64) error    { return nil }
func (f *fakeOnboard) RemoveMember(context.Context, domain.TenantScope, int64) error   { return nil }

const uri = "/api/v1/onboarding/join-request"

// Заявка подаётся от имени конкретного пользователя, поэтому анонимный запрос — 401.
func TestAnonymousIs401(t *testing.T) {
	w := handlertest.Do(New(&fakeOnboard{}).Post, http.MethodPost, uri, `{"slug":"acme"}`)
	handlertest.IsError(t, w, http.StatusUnauthorized)
}

func TestMalformedBodyIs400(t *testing.T) {
	w := handlertest.Do(New(&fakeOnboard{}).Post, http.MethodPost, uri, `{не json`, handlertest.User("u-1"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

// Без slug непонятно, в какую организацию просится пользователь.
func TestEmptySlugIs400(t *testing.T) {
	w := handlertest.Do(New(&fakeOnboard{}).Post, http.MethodPost, uri, `{"slug":""}`, handlertest.User("u-1"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

func TestSuccessIs204AndPassesSlugAndUser(t *testing.T) {
	f := &fakeOnboard{}
	w := handlertest.Do(New(f).Post, http.MethodPost, uri, `{"slug":"acme"}`, handlertest.UserID(42, "u-1"))
	handlertest.Status(t, w, http.StatusNoContent)
	if f.gotSlug != "acme" {
		t.Fatalf("slug = %q, want acme", f.gotSlug)
	}
	if f.gotUser != 42 {
		t.Fatalf("userID = %d, want 42", f.gotUser)
	}
}

// Доменные ошибки различимы клиентом: несуществующая организация — 404,
// повторная заявка от уже состоящего участника — 409, а не общий 500.
func TestErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"нет такой организации", onboardingsvc.ErrTenantNotFound, http.StatusNotFound},
		{"уже участник", onboardingsvc.ErrAlreadyMember, http.StatusConflict},
		{"прочая ошибка", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := handlertest.Do(New(&fakeOnboard{err: c.err}).Post, http.MethodPost, uri,
				`{"slug":"acme"}`, handlertest.User("u-1"))
			handlertest.IsError(t, w, c.want)
		})
	}
}
