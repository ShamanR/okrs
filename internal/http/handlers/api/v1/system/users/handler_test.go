package users

// Глобальный список пользователей системного плана: проверяется форма ответа
// и то, что пустой список сериализуется массивом.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/handlertest"
)

type fakeUsers struct {
	list []*domain.User
	err  error
}

func (f *fakeUsers) ListUsers(context.Context) ([]*domain.User, error) { return f.list, f.err }

const uri = "/api/v1/system/users"

func TestReturnsUsers(t *testing.T) {
	f := &fakeUsers{list: []*domain.User{{ID: 1, DisplayName: "Иван", Email: "i@x", IsSystemAdmin: true}}}
	w := handlertest.Do(New(f).Get, http.MethodGet, uri, "")
	handlertest.Status(t, w, http.StatusOK)
	var out []struct {
		ID            int64  `json:"id"`
		DisplayName   string `json:"display_name"`
		IsSystemAdmin bool   `json:"is_system_admin"`
	}
	handlertest.DecodeJSON(t, w, &out)
	if len(out) != 1 || out[0].ID != 1 || out[0].DisplayName != "Иван" || !out[0].IsSystemAdmin {
		t.Fatalf("ответ = %+v", out)
	}
}

func TestEmptyIsArrayNotNull(t *testing.T) {
	w := handlertest.Do(New(&fakeUsers{}).Get, http.MethodGet, uri, "")
	handlertest.Status(t, w, http.StatusOK)
	var out []any
	handlertest.DecodeJSON(t, w, &out)
	if out == nil {
		t.Fatal("список = null, want []")
	}
}

func TestStoreErrorIs500(t *testing.T) {
	w := handlertest.Do(New(&fakeUsers{err: errors.New("boom")}).Get, http.MethodGet, uri, "")
	handlertest.IsError(t, w, http.StatusInternalServerError)
}
