package notifications_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"okrs/internal/core/domain"
	adminnotifications "okrs/internal/http/handlers/api/v1/admin/settings/notifications"
	"okrs/internal/http/handlers/handlertest"
	notificationchannelsvc "okrs/internal/service/notificationchannel"
	"okrs/notifychannel"
)

type fakeSvc struct {
	states  []notificationchannelsvc.ChannelState
	saved   notificationchannelsvc.SaveInput
	byUser  int64
	saveErr error
}

func (f *fakeSvc) List(context.Context, domain.TenantScope) ([]notificationchannelsvc.ChannelState, error) {
	return f.states, nil
}

func (f *fakeSvc) Save(_ context.Context, _ domain.TenantScope, in notificationchannelsvc.SaveInput, by int64) error {
	f.saved, f.byUser = in, by
	return f.saveErr
}

func state() notificationchannelsvc.ChannelState {
	return notificationchannelsvc.ChannelState{
		Descriptor: notifychannel.Descriptor{
			Name: "mattermost", Title: "Mattermost", SecretField: "token",
			Fields: []notifychannel.Field{
				{Key: "base_url", Label: "Адрес сервера", Required: true, Kind: notifychannel.FieldURL},
				{Key: "token", Label: "Токен бота", Required: true, Kind: notifychannel.FieldSecret},
			},
		},
		Enabled: true, Configured: true,
		Values:     map[string]any{"base_url": "https://mm.example.com"},
		SecretHint: "••••4821",
	}
}

// Форма рисуется по дескриптору, а секрет уходит клиенту только маской.
func TestListDescribesFormAndMasksSecret(t *testing.T) {
	h := adminnotifications.New(&fakeSvc{states: []notificationchannelsvc.ChannelState{state()}})
	rec := handlertest.Do(h.List, http.MethodGet, "/api/v1/admin/settings/notifications", "",
		handlertest.Tenant(1))
	handlertest.Status(t, rec, http.StatusOK)

	var got struct {
		Channels []struct {
			Name       string           `json:"name"`
			Enabled    bool             `json:"enabled"`
			SecretHint string           `json:"secret_hint"`
			Values     map[string]any   `json:"values"`
			Fields     []map[string]any `json:"fields"`
		} `json:"channels"`
	}
	handlertest.DecodeJSON(t, rec, &got)
	if len(got.Channels) != 1 {
		t.Fatalf("каналы: %+v", got.Channels)
	}
	c := got.Channels[0]
	if c.Name != "mattermost" || !c.Enabled || c.SecretHint != "••••4821" {
		t.Fatalf("канал: %+v", c)
	}
	if len(c.Fields) != 2 {
		t.Fatalf("поля формы: %+v", c.Fields)
	}
	if c.Values["token"] != nil {
		t.Fatal("секретное поле попало в values")
	}
}

// Отдельная проверка утечки: даже если реализация начнёт возвращать значения без
// санитайза, тело ответа не должно содержать сам секрет.
func TestListNeverEchoesPlaintextSecret(t *testing.T) {
	const secret = "token-abcdef4821"
	st := state()
	// Секрет, «случайно» попавший в несекретные значения — ровно тот сценарий,
	// от которого защищает санитайз в сервисе и в хендлере.
	st.Values = map[string]any{"base_url": "https://mm.example.com", "token": secret}
	h := adminnotifications.New(&fakeSvc{states: []notificationchannelsvc.ChannelState{st}})

	rec := handlertest.Do(h.List, http.MethodGet, "/api/v1/admin/settings/notifications", "",
		handlertest.Tenant(1))
	handlertest.Status(t, rec, http.StatusOK)
	body := handlertest.Body(rec)
	if strings.Contains(body, secret) {
		t.Fatalf("секрет в теле ответа: %s", body)
	}
}

// PUT доносит до сервиса значения, секрет и автора правки.
func TestSavePassesInputThrough(t *testing.T) {
	svc := &fakeSvc{states: []notificationchannelsvc.ChannelState{state()}}
	h := adminnotifications.New(svc)
	body := `{"enabled":true,"values":{"base_url":"https://mm2"},"secret":"новый-токен"}`
	rec := handlertest.Do(h.Save, http.MethodPut,
		"/api/v1/admin/settings/notifications/mattermost", body,
		handlertest.Tenant(1), handlertest.UserID(42, "udid-42"),
		handlertest.URLParam("channel", "mattermost"))
	handlertest.Status(t, rec, http.StatusNoContent)

	if svc.saved.Channel != "mattermost" || svc.saved.Secret != "новый-токен" {
		t.Fatalf("вход сервиса: %+v", svc.saved)
	}
	if svc.saved.Values["base_url"] != "https://mm2" {
		t.Fatalf("значения: %+v", svc.saved.Values)
	}
	if svc.byUser != 42 {
		t.Fatalf("автор правки: %d", svc.byUser)
	}
}

// Канал, не выданный пространству, — 404, а не 403: подтверждать существование
// канала, которого у пространства нет, значит показывать чужой каталог.
// Дизайн-спека §13.4: недоступные каналы не показываем вовсе.
func TestSaveOfUnavailableChannelIs404(t *testing.T) {
	svc := &fakeSvc{saveErr: notificationchannelsvc.ErrNotAvailable}
	h := adminnotifications.New(svc)
	rec := handlertest.Do(h.Save, http.MethodPut,
		"/api/v1/admin/settings/notifications/telegram", `{"enabled":true}`,
		handlertest.Tenant(1), handlertest.UserID(42, "udid-42"),
		handlertest.URLParam("channel", "telegram"))
	handlertest.Status(t, rec, http.StatusNotFound)
}

// Неизвестный канал отвечает так же, как недоступный: различие в ответах дало бы
// перебором полный список каналов продукта.
func TestSaveOfUnknownChannelIs404(t *testing.T) {
	h := adminnotifications.New(&fakeSvc{saveErr: notificationchannelsvc.ErrUnknownChannel})
	rec := handlertest.Do(h.Save, http.MethodPut,
		"/api/v1/admin/settings/notifications/nope", `{"enabled":true}`,
		handlertest.Tenant(1), handlertest.UserID(42, "udid-42"),
		handlertest.URLParam("channel", "nope"))
	handlertest.Status(t, rec, http.StatusNotFound)
}

// Совпадение статуса — недостаточная проверка неразличимости: разный текст при
// одном и том же 404 — такой же оракул для перебора каталога каналов, как и
// разный статус. Тела обоих ответов обязаны совпадать байт в байт.
func TestSaveOfUnavailableAndUnknownChannelReturnIdenticalBodies(t *testing.T) {
	unavailable := adminnotifications.New(&fakeSvc{saveErr: notificationchannelsvc.ErrNotAvailable})
	recUnavailable := handlertest.Do(unavailable.Save, http.MethodPut,
		"/api/v1/admin/settings/notifications/telegram", `{"enabled":true}`,
		handlertest.Tenant(1), handlertest.UserID(42, "udid-42"),
		handlertest.URLParam("channel", "telegram"))
	handlertest.Status(t, recUnavailable, http.StatusNotFound)
	bodyUnavailable := handlertest.Body(recUnavailable)

	unknown := adminnotifications.New(&fakeSvc{saveErr: notificationchannelsvc.ErrUnknownChannel})
	recUnknown := handlertest.Do(unknown.Save, http.MethodPut,
		"/api/v1/admin/settings/notifications/nope", `{"enabled":true}`,
		handlertest.Tenant(1), handlertest.UserID(42, "udid-42"),
		handlertest.URLParam("channel", "nope"))
	handlertest.Status(t, recUnknown, http.StatusNotFound)
	bodyUnknown := handlertest.Body(recUnknown)

	if bodyUnavailable != bodyUnknown {
		t.Fatalf("тела ответов различаются: недоступный=%q неизвестный=%q", bodyUnavailable, bodyUnknown)
	}
}

// Отсутствие ключа шифрования — ошибка развёртывания, и администратор должен
// прочитать её как таковую, а не как «что-то пошло не так».
func TestSaveWithoutSecretKeyExplainsItself(t *testing.T) {
	h := adminnotifications.New(&fakeSvc{saveErr: notificationchannelsvc.ErrNoSecretKey})
	rec := handlertest.Do(h.Save, http.MethodPut,
		"/api/v1/admin/settings/notifications/mattermost", `{"enabled":true,"secret":"x"}`,
		handlertest.Tenant(1), handlertest.UserID(42, "udid-42"),
		handlertest.URLParam("channel", "mattermost"))
	handlertest.Status(t, rec, http.StatusServiceUnavailable)
	body := handlertest.Body(rec)
	if !strings.Contains(body, "NOTIFICATIONS_SECRET_KEY") {
		t.Fatalf("сообщение не называет причину: %s", body)
	}
}

// Включение канала без секрета (ни нового, ни ранее сохранённого) — 422, а не 500:
// это ошибка заполнения формы администратором, а не сбой сервера.
func TestSaveWithoutRequiredSecretIsUnprocessable(t *testing.T) {
	h := adminnotifications.New(&fakeSvc{saveErr: notificationchannelsvc.ErrSecretRequired})
	rec := handlertest.Do(h.Save, http.MethodPut,
		"/api/v1/admin/settings/notifications/mattermost", `{"enabled":true}`,
		handlertest.Tenant(1), handlertest.UserID(42, "udid-42"),
		handlertest.URLParam("channel", "mattermost"))
	handlertest.Status(t, rec, http.StatusUnprocessableEntity)
	body := handlertest.Body(rec)
	if !strings.Contains(body, "секрет") {
		t.Fatalf("сообщение не объясняет причину: %s", body)
	}
}

// Без активного пространства настройки недоступны.
func TestRequiresTenantScope(t *testing.T) {
	h := adminnotifications.New(&fakeSvc{states: []notificationchannelsvc.ChannelState{state()}})
	handlertest.RequiresTenantScope(t, h.List, http.MethodGet, "/api/v1/admin/settings/notifications")
}

// Незаполненное обязательное поле — 422 с названием поля: форма генерится из
// дескриптора, поэтому «заполните обязательные поля» ничего не сообщает — какие
// именно поля есть у канала, знает только сервер.
func TestEmptyRequiredFieldIsUnprocessableAndNamesTheField(t *testing.T) {
	h := adminnotifications.New(&fakeSvc{saveErr: &notificationchannelsvc.FieldRequiredError{
		Channel: "mattermost", Key: "base_url", Label: "Адрес сервера",
	}})
	rec := handlertest.Do(h.Save, http.MethodPut,
		"/api/v1/admin/settings/notifications/mattermost", `{"enabled":true,"values":{}}`,
		handlertest.Tenant(1), handlertest.URLParam("channel", "mattermost"))
	handlertest.Status(t, rec, http.StatusUnprocessableEntity)
	if !strings.Contains(handlertest.Body(rec), "Адрес сервера") {
		t.Fatalf("ответ не называет поле: %s", handlertest.Body(rec))
	}
}
