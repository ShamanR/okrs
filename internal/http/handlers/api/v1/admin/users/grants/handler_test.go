package grants

// Разбор пути идёт до tenant-гейта, поэтому в тестах гейта параметры валидные.

import (
	"net/http"
	"testing"

	"okrs/internal/http/handlers/handlertest"
)

const uri = "/api/v1/admin/users/1/grants"

func TestBadUserIDIs400(t *testing.T) {
	for _, m := range []struct {
		name string
		h    http.HandlerFunc
	}{
		{"Get", New(nil).Get}, {"Post", New(nil).Post}, {"Delete", New(nil).Delete},
	} {
		t.Run(m.name, func(t *testing.T) {
			w := handlertest.Do(m.h, http.MethodGet, uri, `{"team_id":2}`,
				handlertest.Tenant(1), handlertest.URLParam("userID", "не-число"), handlertest.URLParam("teamID", "2"))
			handlertest.IsError(t, w, http.StatusBadRequest)
		})
	}
}

func TestDeleteBadTeamIDIs400(t *testing.T) {
	w := handlertest.Do(New(nil).Delete, http.MethodDelete, uri+"/x", "",
		handlertest.Tenant(1), handlertest.URLParam("userID", "1"), handlertest.URLParam("teamID", "не-число"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

// Выдача доступа без указания команды бессмысленна и должна отвергаться, а не
// создавать пустой грант.
func TestPostRequiresTeamID(t *testing.T) {
	w := handlertest.Do(New(nil).Post, http.MethodPost, uri, `{}`,
		handlertest.Tenant(1), handlertest.URLParam("userID", "1"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

func TestRequiresTenant(t *testing.T) {
	cases := []struct {
		name string
		h    http.HandlerFunc
		body string
	}{
		{"Get", New(nil).Get, ""},
		{"Post", New(nil).Post, `{"team_id":2}`},
		{"Delete", New(nil).Delete, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := handlertest.Do(c.h, http.MethodGet, uri, c.body,
				handlertest.URLParam("userID", "1"), handlertest.URLParam("teamID", "2"))
			handlertest.IsError(t, w, http.StatusForbidden)
		})
	}
}
