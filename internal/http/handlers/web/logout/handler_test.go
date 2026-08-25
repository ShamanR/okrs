package logout

// Выход обязан быть безусловным: даже если сессионной куки нет или удаление
// сессии не удалось, пользователь получает очистку куки и редирект на /login —
// иначе он останется «залогинен» с мёртвым токеном.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authpkg "okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/store/users"
)

// fakeStore реализует хранилище, которое ждёт auth.Manager. Из всего набора
// логауту нужен только DeleteSession; остальное — заглушки ради интерфейса.
type fakeStore struct {
	deleted []string
	delErr  error
}

func (f *fakeStore) UpsertUser(context.Context, users.UpsertUserInput) (*domain.User, error) {
	return nil, nil
}
func (f *fakeStore) GetUser(context.Context, int64) (*domain.User, error) { return nil, nil }
func (f *fakeStore) CreateSession(context.Context, string, int64, string, time.Duration, string, string) (*domain.AuthSession, error) {
	return nil, nil
}
func (f *fakeStore) GetSession(context.Context, string) (*domain.AuthSession, error) {
	return nil, nil
}
func (f *fakeStore) TouchSession(context.Context, string) error { return nil }
func (f *fakeStore) DeleteSession(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return f.delErr
}
func (f *fakeStore) GetSetting(context.Context, string) (json.RawMessage, error) { return nil, nil }
func (f *fakeStore) GetTenantSetting(context.Context, domain.TenantScope, string) (json.RawMessage, error) {
	return nil, nil
}
func (f *fakeStore) AnySystemAdmin(context.Context) (bool, error)      { return false, nil }
func (f *fakeStore) SetSystemAdmin(context.Context, int64, bool) error { return nil }

const cookieName = "okrs_session"

func manager(t *testing.T, st *fakeStore) *authpkg.Manager {
	t.Helper()
	// Имя куки задаётся конфигом; пустое имя браузер отбрасывает, поэтому задаём явно.
	mgr, err := authpkg.NewManager(authpkg.Config{Mode: authpkg.ModeDisabled, SessionCookie: cookieName}, st)
	if err != nil {
		t.Fatalf("auth.NewManager: %v", err)
	}
	return mgr
}

func post(t *testing.T, st *fakeStore, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/logout", nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	New(manager(t, st)).Post(w, r)
	return w
}

func TestRedirectsToLoginWithoutCookie(t *testing.T) {
	st := &fakeStore{}
	w := post(t, st, nil)
	if w.Code != http.StatusFound {
		t.Fatalf("код = %d, want 302", w.Code)
	}
	if got := w.Header().Get("Location"); got != "/login" {
		t.Fatalf("Location = %q, want /login", got)
	}
	if len(st.deleted) != 0 {
		t.Fatalf("без куки удалять нечего, а удалено: %v", st.deleted)
	}
}

func TestDeletesSessionFromCookie(t *testing.T) {
	st := &fakeStore{}
	post(t, st, &http.Cookie{Name: cookieName, Value: "sess-1"})
	if len(st.deleted) != 1 || st.deleted[0] != "sess-1" {
		t.Fatalf("удалены сессии %v, want [sess-1]", st.deleted)
	}
}

// Кука гасится в любом случае — иначе браузер продолжит слать мёртвый токен.
func TestClearsSessionCookie(t *testing.T) {
	w := post(t, &fakeStore{}, &http.Cookie{Name: cookieName, Value: "sess-1"})
	var cleared bool
	for _, c := range w.Result().Cookies() {
		if c.Name == cookieName && c.Value == "" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatalf("кука %q не погашена: %v", cookieName, w.Result().Cookies())
	}
}

// Сбой удаления сессии не должен оставлять пользователя на странице с ошибкой:
// куку всё равно гасим и уводим на /login.
func TestStoreErrorStillLogsOut(t *testing.T) {
	w := post(t, &fakeStore{delErr: errors.New("boom")}, &http.Cookie{Name: cookieName, Value: "sess-1"})
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/login" {
		t.Fatalf("код=%d Location=%q, want 302 /login", w.Code, w.Header().Get("Location"))
	}
}
