package access

// Базовые проверки гейтов: разбор пути и tenant-scope отрабатывают до обращения
// к сервису, поэтому зависимости здесь нулевые — до них выполнение не доходит.

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

// okSettings принимает любую запись — тесты ниже проверяют аудит, а не хранилище.
type okSettings struct{}

func (okSettings) GetTenant(context.Context, domain.TenantScope, string) (json.RawMessage, error) {
	return nil, nil
}
func (okSettings) SetTenantProduct(context.Context, domain.TenantScope, string, any) error {
	return nil
}

// Эндпоинт закрыт tenant-гейтом: без активного tenant в контексте — 403,
// а не пустой ответ и не паника.
func TestGateGetRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(nil).Get, http.MethodGet, "/api/v1/admin/settings/access", "")
	handlertest.IsError(t, w, http.StatusForbidden)
}

// Эндпоинт закрыт tenant-гейтом: без активного tenant в контексте — 403.
// Тело передаётся валидное: здесь оно разбирается до гейта, и с мусором
// в теле ответ был бы 400 и гейт остался бы непроверенным.
func TestGatePostRequiresTenant(t *testing.T) {
	w := handlertest.Do(New(nil).Post, http.MethodGet, "/api/v1/admin/settings/access", `{}`)
	handlertest.IsError(t, w, http.StatusForbidden)
}

// new_user_policy не проверяется против перечисления, то есть в него попадает
// произвольная строка от клиента. Аудит фиксирует факт изменения, но не само
// значение: под общим ключом редакция по имени ключа его не маскирует.
func TestPolicyValueIsNotLogged(t *testing.T) {
	buf := &bytes.Buffer{}
	const hostile = "xoxb-похоже-на-токен"

	body := strings.NewReader(`{"new_user_policy":"` + hostile + `"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/access", body)
	ctx := auth.WithTenant(r.Context(), &domain.Tenant{ID: 1, Status: domain.TenantActive})
	ctx = logging.WithLogger(ctx, logging.New(logging.Config{Output: buf}))
	w := httptest.NewRecorder()
	New(okSettings{}).Post(w, r.WithContext(ctx))

	if w.Code != http.StatusNoContent {
		t.Fatalf("код = %d (%s)", w.Code, w.Body.String())
	}
	if strings.Contains(buf.String(), hostile) {
		t.Fatalf("непроверенное значение попало в лог: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "new_user_policy") {
		t.Errorf("факт изменения настройки не зафиксирован: %s", buf.String())
	}
}
