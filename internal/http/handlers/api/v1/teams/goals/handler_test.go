package goals

// Проверки идут в порядке: id команды → доступ к команде → тело. Тесты
// подставляют валидными те части, которые проверяются раньше проверяемой.

import (
	"net/http"
	"testing"

	"okrs/internal/http/handlers/handlertest"
)

const uri = "/api/v1/teams/1/goals"

const valid = `{"period_id":1,"title":"Цель","priority":"P0","weight":100,"work_type":"Delivery","focus_type":"STABILITY"}`

func TestBadTeamIDIs400(t *testing.T) {
	w := handlertest.Do(New(nil, nil).Post, http.MethodPost, uri, valid,
		handlertest.Tenant(1), handlertest.URLParam("teamID", "не-число"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

// Команда вне доступа отвечает 404, а не 403: иначе эндпоинт работал бы оракулом
// существования команд для того, кто их видеть не должен.
func TestInaccessibleTeamIs404(t *testing.T) {
	w := handlertest.Do(New(nil, nil).Post, http.MethodPost, uri, valid,
		handlertest.Tenant(1), handlertest.AllowedTeams([]int64{9}), handlertest.URLParam("teamID", "1"))
	handlertest.IsError(t, w, http.StatusNotFound)
}

func TestMalformedBodyIs400(t *testing.T) {
	w := handlertest.Do(New(nil, nil).Post, http.MethodPost, uri, `{не json`,
		handlertest.Tenant(1), handlertest.URLParam("teamID", "1"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

// Цель без названия не имеет смысла на доске.
func TestEmptyTitleIs400(t *testing.T) {
	w := handlertest.Do(New(nil, nil).Post, http.MethodPost, uri, `{"period_id":1,"title":""}`,
		handlertest.Tenant(1), handlertest.URLParam("teamID", "1"))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

func TestRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(nil, nil).Post, http.MethodPost, uri, valid,
		handlertest.URLParam("teamID", "1"))
	handlertest.IsError(t, w, http.StatusForbidden)
}
