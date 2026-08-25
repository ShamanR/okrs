package healthcheckin

// Валидация тела выполняется до tenant-гейта, поэтому тесты гейта шлют
// заведомо валидный конфиг, а тесты валидации — по одному испорченному полю.

import (
	"net/http"
	"testing"

	"okrs/internal/http/handlers/handlertest"
)

const uri = "/api/v1/admin/settings/health-checkin"

// valid — минимальный конфиг, проходящий все проверки. Тесты валидации портят
// в нём ровно одно поле, чтобы было видно, какая проверка сработала.
const valid = `{"stale_days":7,"cache_ttl_minutes":5,"green_threshold":80,"comment_depth":2,"resolved_comments_limit":10}`

func TestGetRequiresTenant(t *testing.T) {
	handlertest.RequiresTenantScope(t, New(nil, nil).Get, http.MethodGet, uri)
}

func TestPostRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(nil, nil).Post, http.MethodPost, uri, valid)
	handlertest.IsError(t, w, http.StatusForbidden)
}

func TestPostRejectsMalformedBody(t *testing.T) {
	w := handlertest.Do(New(nil, nil).Post, http.MethodPost, uri, `{не json`, handlertest.Tenant(1))
	handlertest.IsError(t, w, http.StatusBadRequest)
}

// Каждое поле конфига имеет смысл только в своём диапазоне: ноль или отрицательное
// значение сломало бы расчёт здоровья молча, поэтому отвергается на входе.
func TestPostValidatesEachField(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"stale_days=0", `{"stale_days":0,"cache_ttl_minutes":5,"green_threshold":80,"comment_depth":2,"resolved_comments_limit":10}`},
		{"cache_ttl_minutes=0", `{"stale_days":7,"cache_ttl_minutes":0,"green_threshold":80,"comment_depth":2,"resolved_comments_limit":10}`},
		{"green_threshold=0", `{"stale_days":7,"cache_ttl_minutes":5,"green_threshold":0,"comment_depth":2,"resolved_comments_limit":10}`},
		{"green_threshold=101", `{"stale_days":7,"cache_ttl_minutes":5,"green_threshold":101,"comment_depth":2,"resolved_comments_limit":10}`},
		{"comment_depth<0", `{"stale_days":7,"cache_ttl_minutes":5,"green_threshold":80,"comment_depth":-1,"resolved_comments_limit":10}`},
		{"resolved_comments_limit=0", `{"stale_days":7,"cache_ttl_minutes":5,"green_threshold":80,"comment_depth":2,"resolved_comments_limit":0}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := handlertest.Do(New(nil, nil).Post, http.MethodPost, uri, c.body, handlertest.Tenant(1))
			handlertest.IsError(t, w, http.StatusBadRequest)
		})
	}
}
