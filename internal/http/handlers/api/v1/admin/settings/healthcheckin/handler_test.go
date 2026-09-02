package healthcheckin

// Валидация тела выполняется до tenant-гейта, поэтому тесты гейта шлют
// заведомо валидный конфиг, а тесты валидации — по одному испорченному полю.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/handlertest"
	"okrs/internal/platform/logging"
)

const uri = "/api/v1/admin/settings/health-checkin"

// valid — минимальный конфиг, проходящий все проверки. Тесты валидации портят
// в нём ровно одно поле, чтобы было видно, какая проверка сработала.
const valid = `{"stale_days":7,"cache_ttl_minutes":5,"green_threshold":80,"comment_depth":2,"resolved_comments_limit":10}`

func TestGetRequiresTenant(t *testing.T) {
	handlertest.RequiresTenantScope(t, New(nil, nil).Get, http.MethodGet, uri)
}

// okSettings принимает любую запись — тест проверяет аудит, а не хранилище.
type okSettings struct{}

func (okSettings) GetTenant(context.Context, domain.TenantScope, string) (json.RawMessage, error) {
	return nil, nil
}
func (okSettings) SetTenantProduct(context.Context, domain.TenantScope, string, any) error {
	return nil
}

// Настройки health check-in — такое же административное изменение организации,
// как остальные: без записи оно не попадает в аудит.
func TestHealthCheckInSettingsAreAudited(t *testing.T) {
	buf := &bytes.Buffer{}

	r := httptest.NewRequest(http.MethodPost, uri, strings.NewReader(valid))
	ctx := auth.WithTenant(r.Context(), &domain.Tenant{ID: 1, Status: domain.TenantActive})
	ctx = logging.WithLogger(ctx, logging.New(logging.Config{Output: buf}))
	w := httptest.NewRecorder()
	New(okSettings{}, nil).Post(w, r.WithContext(ctx))

	if w.Code != http.StatusNoContent {
		t.Fatalf("код = %d (%s)", w.Code, w.Body.String())
	}

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("изменение не зафиксировано в аудите: %q", buf.String())
	}
	if rec[logging.KeyEvent] != logging.EventAccessChanged {
		t.Errorf("event = %v, ожидался %s", rec[logging.KeyEvent], logging.EventAccessChanged)
	}
	if rec["setting"] != "health_checkin_config" {
		t.Errorf("setting = %v", rec["setting"])
	}
	// tenant_id и actor_id в записи не проверяются: их проставляет middleware
	// LogContext из контекста запроса, а здесь обработчик вызван напрямую.
	// Связка с middleware покрыта отдельно, в internal/http/middleware.
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
